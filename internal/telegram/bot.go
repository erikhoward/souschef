package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

type Enricher interface {
	Enrich(ctx context.Context, rawText string) (ideas.Metadata, error)
}

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

type Deps struct {
	Client      *Client
	Ideas       *ideas.Service
	Enricher    Enricher
	Transcriber Transcriber
	ChatID      int64
	AudioDir    string
	WebBaseURL  string
}

type Bot struct {
	Deps
	offset int64
}

func New(deps Deps) (*Bot, error) {
	if err := ValidateRegistry(Commands); err != nil {
		return nil, err
	}
	return &Bot{Deps: deps}, nil
}

const longPollSeconds = 30

// Run polls for updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	// Publish the menu, but never let a sync failure stop the bot: a stale
	// menu costs discoverability, whereas refusing to run costs capture.
	changed, err := SyncCommands(ctx, b.Client, b.ChatID, Commands)
	switch {
	case err != nil:
		log.Printf("telegram: command menu not synced: %v", err)
	case changed:
		log.Printf("telegram: published %d commands", len(Commands))
	default:
		log.Printf("telegram: command menu already current")
	}

	log.Printf("telegram: listening (chat %d)", b.ChatID)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.Client.GetUpdates(ctx, b.offset, longPollSeconds)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("telegram: getUpdates: %v", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			b.offset = update.UpdateID + 1
			if err := b.handle(ctx, update); err != nil {
				log.Printf("telegram: handle update %d: %v", update.UpdateID, err)
			}
		}
	}
}

// authorised drops anything from a chat that is not the configured one.
func (b *Bot) authorised(update Update) bool {
	switch {
	case update.Message != nil:
		return update.Message.Chat.ID == b.ChatID
	case update.CallbackQuery != nil && update.CallbackQuery.Message != nil:
		return update.CallbackQuery.Message.Chat.ID == b.ChatID
	default:
		return false
	}
}

func (b *Bot) handle(ctx context.Context, update Update) error {
	if !b.authorised(update) {
		log.Printf("telegram: dropped update from an unauthorised chat")
		return nil
	}

	if update.CallbackQuery != nil {
		return b.handleCallback(ctx, update)
	}
	if update.Message == nil {
		return nil
	}
	if update.Message.Voice != nil {
		return b.handleVoice(ctx, update)
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	if name, args := parseCommand(text); name != "" {
		cmd, ok := Lookup(name)
		if !ok {
			// An unknown command answers with help rather than being
			// silently swallowed.
			_, err := b.Client.SendMessage(ctx, b.ChatID,
				"I don't know /"+escapeHTML(name)+".\n\n"+HelpText(Commands), nil)
			return err
		}
		return cmd.Handler(b, update, args)
	}

	return b.capture(ctx, text, ideas.SourceTelegramText,
		fmt.Sprint(update.Message.MessageID))
}

// parseCommand splits "/name args" into its parts. It tolerates the
// "/name@botname" form Telegram uses in groups.
func parseCommand(text string) (name, args string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	body := strings.TrimPrefix(text, "/")

	head, rest, _ := strings.Cut(body, " ")
	if head == "" {
		return "", ""
	}
	if at := strings.IndexByte(head, '@'); at >= 0 {
		head = head[:at]
	}
	return head, strings.TrimSpace(rest)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// capture saves an idea, replies immediately, and enriches in the background,
// editing the same message in place when the result arrives.
func (b *Bot) capture(ctx context.Context, rawText string, source ideas.Source, ref string) error {
	idea, err := b.Ideas.Create(ctx, rawText, source, ref)
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID, "Could not save that: "+escapeHTML(err.Error()), nil)
		return sendErr
	}

	sent, err := b.Client.SendMessage(ctx, b.ChatID,
		"✓ Saved. <i>Reading it now…</i>", nil)
	if err != nil {
		return err
	}

	go b.enrichAndEdit(idea.ID, idea.RawText, sent.MessageID)
	return nil
}

// enrichAndEdit classifies the idea and rewrites the original message.
// Editing in place, rather than sending a second message, keeps the chat at
// one message per idea — which matters when the chat is the phone-side view
// of the backlog.
func (b *Bot) enrichAndEdit(ideaID, rawText string, messageID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var updated ideas.Idea

	meta, err := b.Enricher.Enrich(ctx, rawText)
	if err != nil {
		updated, err = b.Ideas.RecordEnrichmentFailure(ctx, ideaID, err.Error())
	} else {
		updated, err = b.Ideas.ApplyEnrichment(ctx, ideaID, meta, "")
	}
	if err != nil {
		log.Printf("telegram: record enrichment for %s: %v", ideaID, err)
		// The idea itself is already saved safely — only its enrichment
		// status write failed — but the message is stuck at "Reading it
		// now…" with no way for the user to know something went wrong.
		// Leaving it there forever is exactly the silent-hang failure mode
		// this project has already been burned by once; edit the message
		// with a legible error instead of returning quietly.
		if editErr := b.Client.EditMessageText(ctx, b.ChatID, messageID,
			"⚠️ Could not read it: "+escapeHTML(err.Error()), nil); editErr != nil {
			log.Printf("telegram: edit message %d after enrichment error: %v", messageID, editErr)
		}
		return
	}

	text, markup := RenderIdeaCard(updated, b.WebBaseURL)
	if err := b.Client.EditMessageText(ctx, b.ChatID, messageID, text, markup); err != nil {
		log.Printf("telegram: edit message %d: %v", messageID, err)
	}
}

func (b *Bot) handleVoice(ctx context.Context, update Update) error {
	sent, err := b.Client.SendMessage(ctx, b.ChatID, "🎙 <i>Transcribing…</i>", nil)
	if err != nil {
		return err
	}

	path, err := b.Client.DownloadFile(ctx, update.Message.Voice.FileID, b.AudioDir)
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not download that voice note: "+escapeHTML(err.Error()), nil)
	}

	text, err := b.Transcriber.Transcribe(ctx, path)
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not transcribe that: "+escapeHTML(err.Error()), nil)
	}

	idea, err := b.Ideas.Create(ctx, text, ideas.SourceTelegramVoice,
		fmt.Sprint(update.Message.MessageID))
	if err != nil {
		return b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
			"Could not save that: "+escapeHTML(err.Error()), nil)
	}

	if err := b.Client.EditMessageText(ctx, b.ChatID, sent.MessageID,
		"✓ Saved: <i>"+escapeHTML(text)+"</i>\n\n<i>Reading it now…</i>", nil); err != nil {
		return err
	}

	go b.enrichAndEdit(idea.ID, idea.RawText, sent.MessageID)
	return nil
}

var difficultyLabels = map[string]string{"easy": "Easy", "moderate": "Moderate", "insane": "Insane"}
var durationLabels = map[string]string{"quick": "Quick", "average": "Average", "multi_day": "Multi-day"}
var treatmentLabels = map[string]string{"elevated": "Elevated", "non_elevated": "Everyday"}

// RenderIdeaCard builds the message body and keyboard for one idea. The id
// appears only inside callback_data and the deep link, never in visible text.
func RenderIdeaCard(idea ideas.Idea, webBaseURL string) (string, *InlineKeyboardMarkup) {
	var b strings.Builder
	b.WriteString("<b>" + escapeHTML(idea.Title) + "</b>\n")

	buttons := []InlineKeyboardButton{}

	switch idea.Enrichment.Status {
	case ideas.EnrichPending:
		b.WriteString("<i>Reading it now…</i>")

	case ideas.EnrichFailed:
		b.WriteString("⚠️ Could not read it: " + escapeHTML(idea.Enrichment.Error))
		buttons = append(buttons, InlineKeyboardButton{
			Text: "Retry", CallbackData: "retry:" + idea.ID,
		})

	default:
		parts := []string{}
		for _, v := range []string{
			difficultyLabels[idea.Metadata.Difficulty],
			durationLabels[idea.Metadata.DurationClass],
			treatmentLabels[idea.Metadata.Treatment],
			idea.Metadata.Cuisine,
		} {
			if v != "" {
				parts = append(parts, escapeHTML(v))
			}
		}
		b.WriteString(strings.Join(parts, " · "))
	}

	if webBaseURL != "" {
		buttons = append(buttons, InlineKeyboardButton{
			Text: "Open", URL: webBaseURL + "/ideas/" + idea.ID,
		})
	}

	if len(buttons) == 0 {
		return b.String(), nil
	}
	return b.String(), &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{buttons}}
}
