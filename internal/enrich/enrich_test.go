package enrich

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// newAPIError builds a real *anthropic.Error the way the SDK does: StatusCode
// and RequestID are struct fields, not methods. (*anthropic.Error).Error()
// dereferences Request and Response to build its message, so both must be
// populated with minimal real values or Error() panics.
func newAPIError(status int, requestID string) *anthropic.Error {
	req := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", nil)
	resp := &http.Response{StatusCode: status}
	return &anthropic.Error{
		StatusCode: status,
		Request:    req,
		Response:   resp,
		RequestID:  requestID,
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestParseResponseValid(t *testing.T) {
	got, err := ParseResponse(fixture(t, "valid.json"))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if got.Title != "Crispy chili eggs with scallion oil" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Difficulty != "easy" || got.DurationClass != "quick" {
		t.Errorf("difficulty/duration = %q/%q", got.Difficulty, got.DurationClass)
	}
	if len(got.Equipment) != 2 || got.Equipment[0] != "wok" {
		t.Errorf("Equipment = %v", got.Equipment)
	}
	if len(got.Tags) != 3 {
		t.Errorf("Tags = %v, want 3", got.Tags)
	}
}

func TestParseResponseAcceptsEmptyOptionalFields(t *testing.T) {
	got, err := ParseResponse(fixture(t, "partial.json"))
	if err != nil {
		t.Fatalf("a response with empty optional fields must parse: %v", err)
	}
	if got.Cuisine != "" || got.PrimaryIngredient != "" {
		t.Error("empty strings should stay empty, not be defaulted")
	}
	if got.Difficulty != "moderate" {
		t.Errorf("Difficulty = %q", got.Difficulty)
	}
}

// A model can return well-formed JSON containing a value outside our
// taxonomy. Storing "trivial" as a difficulty would silently corrupt every
// filter and sort downstream, so it must be rejected here.
func TestParseResponseRejectsValueOutsideTaxonomy(t *testing.T) {
	_, err := ParseResponse(fixture(t, "bad_enum.json"))
	if err == nil {
		t.Fatal("expected rejection of difficulty=trivial")
	}
	if !strings.Contains(err.Error(), "difficulty") {
		t.Errorf("error should name the offending field, got: %v", err)
	}
}

func TestParseResponseRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseResponse(fixture(t, "malformed.json")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseResponseRejectsEmptyTitle(t *testing.T) {
	_, err := ParseResponse([]byte(`{"title":"","difficulty":"easy","duration_class":"quick",
		"treatment":"elevated","content_type":"recipe","visual_potential":"high",
		"seasonality":"all_year","production_effort":"light"}`))
	if err == nil {
		t.Fatal("title is the one field we require; empty must be rejected")
	}
}

// TestClassifyRealAPIError401NotRetryable exercises the concrete SDK error
// type end to end, not a hand-shaped stub. StatusCode and RequestID are
// fields on *anthropic.Error, not methods — a duck-typed interface expecting
// methods never matches, so this must use the real type or it proves nothing.
func TestClassifyRealAPIError401NotRetryable(t *testing.T) {
	got := Classify(newAPIError(401, "req_real_123"))

	if got.Retryable {
		t.Error("a real SDK 401 must not be retried — the key will not fix itself")
	}
	for _, want := range []string{"401", "req_real_123"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message %q should contain %q", got.Message, want)
		}
	}
}

func TestClassifyAuthFailureIsNotRetryable(t *testing.T) {
	got := Classify(newAPIError(401, "req_abc"))

	if got.Retryable {
		t.Error("a 401 must not be retried — the key will not fix itself")
	}
	for _, want := range []string{"401", "req_abc"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message %q should contain %q", got.Message, want)
		}
	}
}

func TestClassifyRateLimitIsRetryable(t *testing.T) {
	got := Classify(newAPIError(429, ""))
	if !got.Retryable {
		t.Error("429 should be retryable")
	}
	if !strings.Contains(got.Message, "429") {
		t.Errorf("message must carry the status code, got %q", got.Message)
	}
}

func TestClassifyServerErrorIsRetryable(t *testing.T) {
	if !Classify(newAPIError(529, "")).Retryable {
		t.Error("529 should be retryable")
	}
	if !Classify(newAPIError(500, "")).Retryable {
		t.Error("500 should be retryable")
	}
}

func TestClassifyNonAPIErrorStillProducesMessage(t *testing.T) {
	got := Classify(errors.New("dial tcp: lookup api.anthropic.com: no such host"))
	if got.Message == "" {
		t.Fatal("a transport error must still produce a message for the row")
	}
	if !got.Retryable {
		t.Error("transport errors should be retryable")
	}
}

func TestClassifyNilIsNotAFailure(t *testing.T) {
	if got := Classify(nil); got.Message != "" {
		t.Errorf("Classify(nil) should be zero, got %+v", got)
	}
}

func TestSystemPromptCoversEveryTaxonomyValue(t *testing.T) {
	// If a value exists in the schema, the prompt must define it — otherwise
	// the model is guessing at what we mean.
	for _, v := range []string{
		"easy", "moderate", "insane",
		"quick", "average", "multi_day",
		"elevated", "non_elevated",
		"recipe", "vlog",
		"light", "heavy",
	} {
		if !strings.Contains(SystemPrompt, v) {
			t.Errorf("SystemPrompt does not mention taxonomy value %q", v)
		}
	}
}

func TestLiveEnrichment(t *testing.T) {
	if os.Getenv("SOUSCHEF_LIVE_TEST") == "" {
		t.Skip("set SOUSCHEF_LIVE_TEST=1 to exercise the real API")
	}
	e := New("claude-sonnet-5", "low")
	got, err := e.Enrich(context.Background(),
		"sheet pan shawarma with a lemony feta situation, weeknight thing")
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	t.Logf("%+v", got)
	if got.Title == "" {
		t.Error("live call returned an empty title")
	}
}
