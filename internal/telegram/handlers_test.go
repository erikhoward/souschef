package telegram

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// stubEnricher returns fixed metadata, or an error, with no network.
type stubEnricher struct {
	meta ideas.Metadata
	err  error
}

func (s stubEnricher) Enrich(context.Context, string) (ideas.Metadata, error) {
	return s.meta, s.err
}
func (s stubEnricher) Model() string { return "stub-model" }

type stubTranscriber struct {
	text string
	err  error
}

func (s stubTranscriber) Transcribe(context.Context, string) (string, error) {
	return s.text, s.err
}

func okMetadata() ideas.Metadata {
	return ideas.Metadata{
		Title: "Crispy chili eggs", Difficulty: "easy", DurationClass: "quick",
		Treatment: "elevated", ContentType: "recipe", Cuisine: "Chinese-inspired",
		VisualPotential: "high", Seasonality: "all_year", ProductionEffort: "light",
	}
}

// newTestBot wires a Bot against the fake API and a real store, so the tests
// exercise genuine domain behaviour rather than a mock of it.
func newBotWithFake(t *testing.T, e Enricher, tr Transcriber) (*Bot, *fakeAPI, *ideas.Service) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	api := newFakeAPI()
	svc := ideas.NewService(st)

	bot, err := New(Deps{
		Client:      api,
		Ideas:       svc,
		Enricher:    e,
		Transcriber: tr,
		ChatID:      42,
		AudioDir:    t.TempDir(),
		WebBaseURL:  "http://127.0.0.1:8420",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bot, api, svc
}

func textUpdate(body string) Update {
	return Update{
		UpdateID: 1,
		Message:  &Message{MessageID: 7, Chat: Chat{ID: 42}, Text: body},
	}
}

// --- search: the feature this whole redesign exists for ---

func TestHandleSearchReturnsTappableResultsAndNeverShowsAnID(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{meta: okMetadata()}, stubTranscriber{})
	ctx := context.Background()

	created, err := svc.Create(ctx, "sheet pan shawarma with lemony feta", ideas.SourceWeb, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "cabbage soup, humble", ideas.SourceWeb, ""); err != nil {
		t.Fatal(err)
	}

	if err := handleSearch(bot, textUpdate("/s shawarma"), "shawarma"); err != nil {
		t.Fatalf("handleSearch: %v", err)
	}

	sent := api.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("want 1 message, got %d", len(sent))
	}
	if sent[0].Markup == nil || len(sent[0].Markup.InlineKeyboard) != 1 {
		t.Fatalf("want exactly one result button, got %+v", sent[0].Markup)
	}

	btn := sent[0].Markup.InlineKeyboard[0][0]
	if btn.CallbackData != "open:"+created.ID {
		t.Errorf("callback_data = %q, want open:%s", btn.CallbackData, created.ID)
	}
	// The defect that made the previous iteration unusable: an id the human
	// had to read and copy.
	if strings.Contains(btn.Text, created.ID) {
		t.Errorf("button label leaks the id: %q", btn.Text)
	}
	if strings.Contains(sent[0].Text, created.ID) {
		t.Errorf("message body leaks the id: %q", sent[0].Text)
	}
}

func TestHandleSearchWithNoQueryAsksForOne(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	if err := handleSearch(bot, textUpdate("/s"), ""); err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	sent := api.sentMessages()
	if len(sent) != 1 || !strings.Contains(strings.ToLower(sent[0].Text), "search for") {
		t.Errorf("want a prompt for a query, got %+v", sent)
	}
	if sent[0].Markup != nil {
		t.Error("a prompt should not carry a results keyboard")
	}
}

func TestHandleSearchWithNoMatchesSaysSo(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{}, stubTranscriber{})
	if _, err := svc.Create(context.Background(), "cabbage soup", ideas.SourceWeb, ""); err != nil {
		t.Fatal(err)
	}

	if err := handleSearch(bot, textUpdate("/s zzzznothing"), "zzzznothing"); err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	sent := api.sentMessages()
	if len(sent) != 1 || !strings.Contains(sent[0].Text, "Nothing matches") {
		t.Errorf("want a no-matches reply, got %+v", sent)
	}
}

func TestHandleSearchEscapesTheQuery(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	if err := handleSearch(bot, textUpdate("/s x"), "<script>alert(1)</script>"); err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	body := api.sentMessages()[0].Text
	if strings.Contains(body, "<script>") {
		t.Errorf("query must be HTML-escaped before entering an HTML-parsed message: %q", body)
	}
}

func TestHandleRecentOnEmptyBacklogIsHelpful(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	if err := handleRecent(bot, textUpdate("/recent"), ""); err != nil {
		t.Fatalf("handleRecent: %v", err)
	}
	sent := api.sentMessages()
	if len(sent) != 1 || !strings.Contains(strings.ToLower(sent[0].Text), "nothing captured") {
		t.Errorf("want a friendly empty-state, got %+v", sent)
	}
}

func TestHandleRecentCapsResults(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{}, stubTranscriber{})
	ctx := context.Background()

	for i := 0; i < 14; i++ {
		if _, err := svc.Create(ctx, "idea number "+string(rune('a'+i)), ideas.SourceWeb, ""); err != nil {
			t.Fatal(err)
		}
	}

	if err := handleRecent(bot, textUpdate("/recent"), ""); err != nil {
		t.Fatalf("handleRecent: %v", err)
	}
	rows := api.sentMessages()[0].Markup.InlineKeyboard
	if len(rows) != 10 {
		t.Errorf("want 10 buttons, got %d", len(rows))
	}
}

// --- callbacks ---

func callbackUpdate(data string) Update {
	return Update{
		UpdateID: 2,
		CallbackQuery: &CallbackQuery{
			ID:      "cb1",
			Data:    data,
			Message: &Message{MessageID: 55, Chat: Chat{ID: 42}},
		},
	}
}

func TestHandleCallbackOpenSendsTheCard(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{}, stubTranscriber{})
	ctx := context.Background()

	created, _ := svc.Create(ctx, "crispy chili eggs", ideas.SourceWeb, "")

	if err := bot.handleCallback(ctx, callbackUpdate("open:"+created.ID)); err != nil {
		t.Fatalf("handleCallback: %v", err)
	}
	if len(api.callbackAnswers()) != 1 {
		t.Error("every callback must be answered, or Telegram shows a spinner forever")
	}
	sent := api.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("want the idea card, got %d messages", len(sent))
	}
	if !strings.Contains(sent[0].Text, created.Title) {
		t.Errorf("card should carry the title, got %q", sent[0].Text)
	}
	if strings.Contains(sent[0].Text, created.ID) {
		t.Errorf("card body leaks the id: %q", sent[0].Text)
	}
}

func TestHandleCallbackForDeletedIdeaSaysSo(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	err := bot.handleCallback(context.Background(),
		callbackUpdate("open:0191f0c2-1234-7890-abcd-ef0123456789"))
	if err != nil {
		t.Fatalf("a missing idea must not error the handler: %v", err)
	}
	answers := api.callbackAnswers()
	if len(answers) != 1 || !strings.Contains(answers[0], "gone") {
		t.Errorf("want a 'that idea is gone' answer, got %v", answers)
	}
	if len(api.sentMessages()) != 0 {
		t.Error("no card should be sent for an idea that does not exist")
	}
}

func TestHandleCallbackUnknownActionIsAnsweredNotIgnored(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	for _, data := range []string{"bogus:123", "no-colon-at-all", ""} {
		if err := bot.handleCallback(context.Background(), callbackUpdate(data)); err != nil {
			t.Errorf("handleCallback(%q): %v", data, err)
		}
	}
	if len(api.callbackAnswers()) != 3 {
		t.Errorf("every callback must be answered; got %d answers", len(api.callbackAnswers()))
	}
}

func TestHandleCallbackRetryReenriches(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{meta: okMetadata()}, stubTranscriber{})
	ctx := context.Background()

	created, _ := svc.Create(ctx, "chili eggs", ideas.SourceWeb, "")
	if _, err := svc.RecordEnrichmentFailure(ctx, created.ID, "401 authentication_error"); err != nil {
		t.Fatal(err)
	}

	if err := bot.handleCallback(ctx, callbackUpdate("retry:"+created.ID)); err != nil {
		t.Fatalf("handleCallback: %v", err)
	}
	bot.Drain(ctx) // retry enriches in the background

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enrichment.Status != ideas.EnrichOK {
		t.Errorf("Status = %q, want ok after a successful retry", got.Enrichment.Status)
	}
	if len(api.editedMessages()) == 0 {
		t.Error("retry should rewrite the original card in place")
	}
}

// --- authorisation ---

func TestUpdatesFromOtherChatsAreDropped(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{meta: okMetadata()}, stubTranscriber{})
	ctx := context.Background()

	intruderText := Update{Message: &Message{MessageID: 1, Chat: Chat{ID: 999}, Text: "steal this"}}
	intruderCB := Update{CallbackQuery: &CallbackQuery{
		ID: "x", Data: "open:whatever", Message: &Message{MessageID: 1, Chat: Chat{ID: 999}},
	}}

	for _, u := range []Update{intruderText, intruderCB} {
		if err := bot.handle(ctx, u); err != nil {
			t.Fatalf("dropping an unauthorised update must not error: %v", err)
		}
	}

	if n := len(api.sentMessages()) + len(api.editedMessages()) + len(api.callbackAnswers()); n != 0 {
		t.Errorf("an unauthorised chat must get no response at all, got %d interactions", n)
	}
	got, err := svc.List(ctx, ideas.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("an unauthorised message must not create an idea, got %d", len(got))
	}
}

// --- capture ---

func TestTextCaptureRepliesOnceThenEditsThatMessage(t *testing.T) {
	bot, api, svc := newBotWithFake(t, stubEnricher{meta: okMetadata()}, stubTranscriber{})
	ctx := context.Background()

	if err := bot.handle(ctx, textUpdate("crispy chili eggs with scallion oil")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	bot.Drain(ctx)

	sent := api.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("capture must send exactly one message, got %d", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Saved") {
		t.Errorf("first reply should confirm the save, got %q", sent[0].Text)
	}

	// One message per idea: enrichment edits the same message rather than
	// sending a second, so the chat stays usable as a backlog view.
	edits := api.editedMessages()
	if len(edits) == 0 {
		t.Fatal("enrichment should have edited the original message")
	}
	if edits[0].MessageID != int64(101) {
		t.Errorf("edited message id = %d, want the id returned by SendMessage", edits[0].MessageID)
	}
	if !strings.Contains(edits[0].Text, "Crispy chili eggs") {
		t.Errorf("edited card should show the enriched title, got %q", edits[0].Text)
	}

	got, _ := svc.List(ctx, ideas.ListFilter{})
	if len(got) != 1 || got[0].Enrichment.Status != ideas.EnrichOK {
		t.Errorf("idea should be saved and enriched, got %+v", got)
	}
}

func TestFailedEnrichmentShowsTheErrorAndOffersRetry(t *testing.T) {
	bot, api, _ := newBotWithFake(t,
		stubEnricher{err: errors.New("401 authentication_error: invalid x-api-key")},
		stubTranscriber{})
	ctx := context.Background()

	if err := bot.handle(ctx, textUpdate("this will fail to enrich")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	bot.Drain(ctx)

	edits := api.editedMessages()
	if len(edits) == 0 {
		t.Fatal("a failed enrichment must still rewrite the message")
	}
	last := edits[len(edits)-1]
	if !strings.Contains(last.Text, "401") {
		t.Errorf("the card must carry the provider's message, got %q", last.Text)
	}

	var hasRetry bool
	if last.Markup != nil {
		for _, row := range last.Markup.InlineKeyboard {
			for _, b := range row {
				if strings.Contains(strings.ToLower(b.Text), "retry") {
					hasRetry = true
				}
			}
		}
	}
	if !hasRetry {
		t.Error("a failed card must offer Retry — otherwise the chat is a dead end")
	}
}

func TestUnknownCommandRepliesWithHelp(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})

	if err := bot.handle(context.Background(), textUpdate("/definitelynotacommand")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	sent := api.sentMessages()
	if len(sent) != 1 {
		t.Fatalf("want one reply, got %d", len(sent))
	}
	for _, c := range Commands {
		if !strings.Contains(sent[0].Text, "/"+c.Name) {
			t.Errorf("unknown-command reply should list /%s", c.Name)
		}
	}
}

func TestVoiceCaptureTranscribesAndSaves(t *testing.T) {
	bot, api, svc := newBotWithFake(t,
		stubEnricher{meta: okMetadata()},
		stubTranscriber{text: "crispy chili eggs with scallion oil"})
	ctx := context.Background()

	api.downloads["voice-file-1"] = filepath.Join(t.TempDir(), "note.oga")

	update := Update{
		UpdateID: 3,
		Message: &Message{
			MessageID: 9, Chat: Chat{ID: 42},
			Voice: &Voice{FileID: "voice-file-1", Duration: 3},
		},
	}
	if err := bot.handle(ctx, update); err != nil {
		t.Fatalf("handle: %v", err)
	}
	bot.Drain(ctx)

	got, _ := svc.List(ctx, ideas.ListFilter{})
	if len(got) != 1 {
		t.Fatalf("a voice note should create one idea, got %d", len(got))
	}
	if got[0].Source != ideas.SourceTelegramVoice {
		t.Errorf("Source = %q, want telegram_voice", got[0].Source)
	}
	if got[0].RawText != "crispy chili eggs with scallion oil" {
		t.Errorf("RawText should be the transcript, got %q", got[0].RawText)
	}
	if len(api.sentMessages()) != 1 {
		t.Errorf("voice capture should send one message and edit it, got %d sends",
			len(api.sentMessages()))
	}
}

func TestVoiceTranscriptionFailureIsLegible(t *testing.T) {
	bot, api, svc := newBotWithFake(t,
		stubEnricher{},
		stubTranscriber{err: errors.New("ffmpeg could not decode note.oga")})
	ctx := context.Background()

	api.downloads["voice-file-1"] = filepath.Join(t.TempDir(), "note.oga")

	update := Update{
		Message: &Message{
			MessageID: 9, Chat: Chat{ID: 42},
			Voice: &Voice{FileID: "voice-file-1"},
		},
	}
	if err := bot.handle(ctx, update); err != nil {
		t.Fatalf("handle: %v", err)
	}

	edits := api.editedMessages()
	if len(edits) == 0 {
		t.Fatal("a transcription failure must rewrite the message")
	}
	if !strings.Contains(edits[len(edits)-1].Text, "could not decode") {
		t.Errorf("the reason must reach the user, got %q", edits[len(edits)-1].Text)
	}
	if got, _ := svc.List(ctx, ideas.ListFilter{}); len(got) != 0 {
		t.Error("a failed transcription should not create an idea")
	}
}

// --- menu sync ---

func TestSyncCommandsPublishesWhenMenuDiffers(t *testing.T) {
	api := newFakeAPI() // starts with an empty menu

	changed, err := SyncCommands(context.Background(), api, 42, Commands)
	if err != nil {
		t.Fatalf("SyncCommands: %v", err)
	}
	if !changed {
		t.Error("an empty remote menu must be published")
	}
	if api.setMyCommandsCalls() != 1 {
		t.Errorf("setMyCommands calls = %d, want 1", api.setMyCommandsCalls())
	}
}

func TestSyncCommandsIsANoOpWhenMenuMatches(t *testing.T) {
	api := newFakeAPI()
	api.menu = MenuFrom(Commands)

	changed, err := SyncCommands(context.Background(), api, 42, Commands)
	if err != nil {
		t.Fatalf("SyncCommands: %v", err)
	}
	if changed {
		t.Error("an identical menu must not be republished")
	}
	if api.setMyCommandsCalls() != 0 {
		t.Errorf("a no-op restart must not write to a rate-limited API; calls = %d",
			api.setMyCommandsCalls())
	}
}

func TestSyncCommandsSurfacesAReadFailure(t *testing.T) {
	api := newFakeAPI()
	api.getErr = errors.New("401 Unauthorized")

	if _, err := SyncCommands(context.Background(), api, 42, Commands); err == nil {
		t.Fatal("a failed read must be reported to the caller")
	}
}

// Run's sync failure must not stop the bot: a stale menu costs
// discoverability, refusing to run costs capture entirely.
func TestRunSurvivesASyncFailure(t *testing.T) {
	bot, api, _ := newBotWithFake(t, stubEnricher{}, stubTranscriber{})
	api.getErr = errors.New("401 Unauthorized")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := bot.Run(ctx); err != nil {
		t.Fatalf("Run must not fail because the menu could not sync: %v", err)
	}
}
