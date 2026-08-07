package telegram

import (
	"context"
	"fmt"
	"sync"
)

// sentMessage records one SendMessage call.
type sentMessage struct {
	Text   string
	Markup *InlineKeyboardMarkup
}

// editedMessage records one EditMessageText call.
type editedMessage struct {
	MessageID int64
	Text      string
	Markup    *InlineKeyboardMarkup
}

// fakeAPI is an in-memory stand-in for the Telegram Bot API.
//
// It records what the bot tried to send so tests can assert on the actual
// user-visible behaviour — the message text, the inline keyboard, whether an
// existing message was edited rather than a second one sent — without a token
// or a network. Every method is safe for concurrent use because enrichment
// runs in a goroutine and writes back through EditMessageText.
type fakeAPI struct {
	mu sync.Mutex

	sent    []sentMessage
	edited  []editedMessage
	answers []string

	menu      []BotCommand
	setCalls  int
	getErr    error
	sendErr   error
	editErr   error
	downloads map[string]string // fileID -> path returned
	dlErr     error

	nextMessageID int64
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{downloads: map[string]string{}, nextMessageID: 100}
}

func (f *fakeAPI) GetUpdates(context.Context, int64, int) ([]Update, error) { return nil, nil }

func (f *fakeAPI) SendMessage(_ context.Context, _ int64, text string,
	markup *InlineKeyboardMarkup) (Message, error) {

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return Message{}, f.sendErr
	}
	f.sent = append(f.sent, sentMessage{Text: text, Markup: markup})
	f.nextMessageID++
	return Message{MessageID: f.nextMessageID}, nil
}

func (f *fakeAPI) EditMessageText(_ context.Context, _, messageID int64, text string,
	markup *InlineKeyboardMarkup) error {

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.editErr != nil {
		return f.editErr
	}
	f.edited = append(f.edited, editedMessage{MessageID: messageID, Text: text, Markup: markup})
	return nil
}

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, _, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, text)
	return nil
}

func (f *fakeAPI) GetMyCommands(context.Context, int64) ([]BotCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.menu, nil
}

func (f *fakeAPI) SetMyCommands(_ context.Context, _ int64, commands []BotCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls++
	f.menu = commands
	return nil
}

func (f *fakeAPI) DownloadFile(_ context.Context, fileID, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dlErr != nil {
		return "", f.dlErr
	}
	p, ok := f.downloads[fileID]
	if !ok {
		return "", fmt.Errorf("fake: no download registered for %s", fileID)
	}
	return p, nil
}

// --- accessors, so tests never touch the mutex-guarded fields directly ---

func (f *fakeAPI) sentMessages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.sent...)
}

func (f *fakeAPI) editedMessages() []editedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]editedMessage(nil), f.edited...)
}

func (f *fakeAPI) callbackAnswers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answers...)
}

func (f *fakeAPI) setMyCommandsCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setCalls
}
