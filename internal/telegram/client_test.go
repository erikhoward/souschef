package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// secretToken has the shape of a real BotFather token so a substring match is
// meaningful. A leaked bot token is full control of the bot.
const secretToken = "123456:AAHsuperSECRETbotTOKENvalue"

// deadClient points at a closed port, so every call fails in the transport
// rather than at the API. That is the path that leaks: http.Client.Do returns
// a *url.Error whose Error() prints the full request URL, and every URL this
// client builds embeds the token as .../bot<TOKEN>/<method>.
func deadClient() *Client {
	c := NewClient(secretToken)
	// Port 1 is reserved and never listening: a refused connection, instantly,
	// with no DNS lookup and no network access.
	c.baseURL = "http://127.0.0.1:1"
	c.http = &http.Client{Timeout: 5 * time.Second}
	return c
}

// TestTransportErrorNeverLeaksTheToken covers the four places the reviewer
// traced a raw transport error to: bot.go's getUpdates log line (fires on any
// network blip — a closed laptop lid is enough), the two handle/menu log
// lines, and worst, the voice path that sends err.Error() into the chat as a
// message, persisting the token into the chat history.
func TestTransportErrorNeverLeaksTheToken(t *testing.T) {
	c := deadClient()
	ctx := context.Background()

	calls := map[string]func() error{
		"GetUpdates": func() error {
			_, err := c.GetUpdates(ctx, 0, 1)
			return err
		},
		"SendMessage": func() error {
			_, err := c.SendMessage(ctx, 1, "hello", nil)
			return err
		},
		"EditMessageText": func() error {
			return c.EditMessageText(ctx, 1, 2, "hello", nil)
		},
		"AnswerCallbackQuery": func() error {
			return c.AnswerCallbackQuery(ctx, "cb", "")
		},
		"GetMyCommands": func() error {
			_, err := c.GetMyCommands(ctx, 1)
			return err
		},
		"SetMyCommands": func() error {
			return c.SetMyCommands(ctx, 1, MenuFrom(Commands))
		},
		"GetFile": func() error {
			_, err := c.GetFile(ctx, "file-1")
			return err
		},
		"DownloadFile": func() error {
			_, err := c.DownloadFile(ctx, "file-1", t.TempDir())
			return err
		},
	}

	for name, call := range calls {
		err := call()
		if err == nil {
			t.Fatalf("%s: expected a transport failure against a closed port", name)
		}
		if strings.Contains(err.Error(), secretToken) {
			t.Errorf("%s leaks the bot token into its error message:\n  %v", name, err)
		}
	}
}

// failFileLeg lets getFile succeed against a real test server while the
// subsequent file download fails in the transport, so DownloadFile's own
// http.Do call (a second URL that also embeds the token) is exercised
// directly rather than only via GetFile.
type failFileLeg struct{ base http.RoundTripper }

func (t failFileLeg) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.Contains(r.URL.Path, "/file/") {
		return nil, errors.New("simulated transport failure")
	}
	return t.base.RoundTrip(r)
}

func TestDownloadFileTransportErrorNeverLeaksTheToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"file_path": "voice/file_1.oga"},
		})
	}))
	defer srv.Close()

	c := NewClient(secretToken)
	c.baseURL = srv.URL
	c.http = &http.Client{Transport: failFileLeg{base: http.DefaultTransport}}

	_, err := c.DownloadFile(context.Background(), "file-1", t.TempDir())
	if err == nil {
		t.Fatal("expected the download leg to fail")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Errorf("DownloadFile leaks the bot token on the download leg:\n  %v", err)
	}
}

// The API-level error path was already clean and must stay that way: a 401
// renders as "getMyCommands failed: 401 Unauthorized" with no URL in it.
func TestAPIErrorStaysLegibleAndTokenFree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"ok": false, "error_code": 401, "description": "Unauthorized",
		})
	}))
	defer srv.Close()

	c := NewClient(secretToken)
	c.baseURL = srv.URL

	_, err := c.GetMyCommands(context.Background(), 1)
	if err == nil {
		t.Fatal("expected a 401 to surface as an error")
	}
	if strings.Contains(err.Error(), secretToken) {
		t.Errorf("API error leaks the token: %v", err)
	}
	// Redaction must not cost legibility — the operator still needs to know
	// which method failed and why.
	for _, want := range []string{"getMyCommands", "401", "Unauthorized"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("API error dropped %q, leaving an unactionable message: %v", want, err)
		}
	}
}

// Redaction must not break error inspection: bot.Run distinguishes a real
// failure from a cancelled context, and future callers may use errors.Is.
func TestRedactionPreservesTheErrorChain(t *testing.T) {
	c := deadClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.GetUpdates(ctx, 0, 1)
	if err == nil {
		t.Fatal("expected a cancelled context to fail the call")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; redaction must preserve the chain: %v", err)
	}
}
