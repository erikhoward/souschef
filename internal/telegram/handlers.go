package telegram

import (
	"context"
	"strings"

	"github.com/erikhoward/souschef/internal/ideas"
)

const maxResults = 5

// resultsKeyboard turns a result set into tappable buttons. This is the whole
// point of the redesign: the previous iteration made you copy and paste ids.
func resultsKeyboard(results []ideas.Idea) *InlineKeyboardMarkup {
	rows := make([][]InlineKeyboardButton, 0, len(results))
	for _, idea := range results {
		label := idea.Title
		if len(label) > 48 {
			label = label[:47] + "…"
		}
		rows = append(rows, []InlineKeyboardButton{{
			Text:         label,
			CallbackData: "open:" + idea.ID,
		}})
	}
	if len(rows) == 0 {
		return nil
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func handleSearch(b *Bot, update Update, args string) error {
	ctx := context.Background()

	query := strings.TrimSpace(args)
	if query == "" {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Give me something to search for — for example: <code>/s shawarma</code>", nil)
		return err
	}

	results, err := b.Ideas.List(ctx, ideas.ListFilter{Query: query, Limit: maxResults})
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID,
			"Search failed: "+escapeHTML(err.Error()), nil)
		return sendErr
	}
	if len(results) == 0 {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Nothing matches “"+escapeHTML(query)+"”.", nil)
		return err
	}

	_, err = b.Client.SendMessage(ctx,
		b.ChatID,
		"Tap one to open it:",
		resultsKeyboard(results))
	return err
}

func handleRecent(b *Bot, update Update, _ string) error {
	ctx := context.Background()

	results, err := b.Ideas.List(ctx, ideas.ListFilter{Sort: "created_at", Order: "desc", Limit: 10})
	if err != nil {
		_, sendErr := b.Client.SendMessage(ctx, b.ChatID,
			"Could not load your backlog: "+escapeHTML(err.Error()), nil)
		return sendErr
	}
	if len(results) == 0 {
		_, err := b.Client.SendMessage(ctx, b.ChatID,
			"Nothing captured yet. Send me anything and I'll save it.", nil)
		return err
	}

	_, err = b.Client.SendMessage(ctx, b.ChatID, "Your ten newest:", resultsKeyboard(results))
	return err
}

func handleHelp(b *Bot, update Update, _ string) error {
	_, err := b.Client.SendMessage(context.Background(), b.ChatID, HelpText(Commands), nil)
	return err
}

// handleCallback services the inline keyboard. Callbacks are not commands:
// they arrive as callback_data, never as typed text, and Telegram has no menu
// concept for them — which is why they are not in the registry.
func (b *Bot) handleCallback(ctx context.Context, update Update) error {
	query := update.CallbackQuery
	action, id, found := strings.Cut(query.Data, ":")
	if !found {
		return b.Client.AnswerCallbackQuery(ctx, query.ID, "")
	}

	switch action {
	case "open":
		idea, err := b.Ideas.Get(ctx, id)
		if err != nil {
			return b.Client.AnswerCallbackQuery(ctx, query.ID, "That idea is gone.")
		}
		if err := b.Client.AnswerCallbackQuery(ctx, query.ID, ""); err != nil {
			return err
		}
		text, markup := RenderIdeaCard(idea, b.WebBaseURL)
		_, err = b.Client.SendMessage(ctx, b.ChatID, text, markup)
		return err

	case "retry":
		idea, err := b.Ideas.Get(ctx, id)
		if err != nil {
			return b.Client.AnswerCallbackQuery(ctx, query.ID, "That idea is gone.")
		}
		if err := b.Client.AnswerCallbackQuery(ctx, query.ID, "Retrying…"); err != nil {
			return err
		}
		if _, err := b.Ideas.MarkPending(ctx, id); err != nil {
			return err
		}
		go b.enrichAndEdit(idea.ID, idea.RawText, query.Message.MessageID)
		return nil

	default:
		return b.Client.AnswerCallbackQuery(ctx, query.ID, "")
	}
}
