// Package telegram implements the bot.
//
// There is no SDK here on purpose: we need seven Bot API methods, all plain
// JSON over HTTPS. A small client avoids inheriting a dependency's release
// cadence and keeps the long-poll timeout under our control.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

type Client struct {
	token string
	// baseURL is apiBase in production. It exists as a field, rather than the
	// const being used directly, so package-internal tests can point the
	// client at an httptest server or an unreachable host — without it there
	// is no way to exercise the transport-failure path at all, which is
	// precisely the path that used to leak the token.
	baseURL string
	http    *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: apiBase,
		// Longer than the long-poll timeout below, so getUpdates is never cut
		// off by the transport.
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

// redactedError carries a scrubbed message while keeping the original error
// reachable through Unwrap, so errors.Is/As still work on whatever the
// transport returned — bot.Run's cancellation check depends on that.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redact scrubs the bot token out of an error before it can escape the client.
//
// Every URL this client builds embeds the token (.../bot<TOKEN>/<method>), and
// on a transport failure http.Client.Do returns a *url.Error whose Error()
// prints that URL verbatim. Those errors reach the log on every network blip —
// a closed laptop lid is enough — and, worse, handleVoice sends err.Error()
// into the chat as message text, persisting the token into the chat history.
// A leaked bot token is full control of the bot, so no raw error is allowed
// out of call() or DownloadFile.
//
// The API-level path (a 401 rendering as "getMyCommands failed: 401
// Unauthorized") never contained the URL and is unaffected; passing it through
// here too is simply cheaper than reasoning about which paths are safe.
func (c *Client) redact(err error) error {
	if err == nil || c.token == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, c.token) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(msg, c.token, "<redacted>"), err: err}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

// call issues one Bot API request. Every error it returns passes through
// redact: the request URL embeds the token, so a raw transport error is a
// credential leak.
func (c *Client) call(ctx context.Context, method string, payload, out any) error {
	return c.redact(c.doCall(ctx, method, payload, out))
}

func (c *Client) doCall(ctx context.Context, method string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", method, err)
		}
		body = bytes.NewReader(buf)
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	var envelope apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if !envelope.OK {
		return fmt.Errorf("%s failed: %d %s", method, envelope.ErrorCode, envelope.Description)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// --- wire types (only the fields we use) ---

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
	Voice     *Voice `json:"voice"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// --- methods ---

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	var out []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message", "callback_query"},
	}, &out)
	return out, err
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string,
	markup *InlineKeyboardMarkup) (Message, error) {

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}

	var out Message
	err := c.call(ctx, "sendMessage", payload, &out)
	return out, err
}

func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string,
	markup *InlineKeyboardMarkup) error {

	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return c.call(ctx, "editMessageText", payload, nil)
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery",
		map[string]any{"callback_query_id": id, "text": text}, nil)
}

func (c *Client) GetMyCommands(ctx context.Context, chatID int64) ([]BotCommand, error) {
	var out []BotCommand
	err := c.call(ctx, "getMyCommands", map[string]any{
		"scope": map[string]any{"type": "chat", "chat_id": chatID},
	}, &out)
	return out, err
}

// SetMyCommands publishes the menu, scoped to the one chat allowed to use the
// bot rather than to everyone who finds it.
func (c *Client) SetMyCommands(ctx context.Context, chatID int64, commands []BotCommand) error {
	return c.call(ctx, "setMyCommands", map[string]any{
		"commands": commands,
		"scope":    map[string]any{"type": "chat", "chat_id": chatID},
	}, nil)
}

// GetFile resolves a file_id to its remote file_path.
func (c *Client) GetFile(ctx context.Context, fileID string) (string, error) {
	var meta struct {
		FilePath string `json:"file_path"`
	}
	if err := c.call(ctx, "getFile", map[string]any{"file_id": fileID}, &meta); err != nil {
		return "", err
	}
	return meta.FilePath, nil
}

// DownloadFile resolves a file_id and writes the bytes into dir, returning the
// local path.
func (c *Client) DownloadFile(ctx context.Context, fileID, dir string) (string, error) {
	path, err := c.doDownloadFile(ctx, fileID, dir)
	return path, c.redact(err)
}

func (c *Client) doDownloadFile(ctx context.Context, fileID, dir string) (string, error) {
	filePath, err := c.GetFile(ctx, fileID)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", fileID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", fileID, resp.StatusCode)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// filepath.Base guards against a traversal via the server-supplied path.
	dest := filepath.Join(dir, fileID+filepath.Ext(filepath.Base(filePath)))

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}
	return dest, nil
}
