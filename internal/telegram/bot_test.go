package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func enrichedIdea() ideas.Idea {
	at := time.Now()
	return ideas.Idea{
		ID:      "0191f0c2-1234-7890-abcd-ef0123456789",
		Title:   "Crispy chili eggs with scallion oil",
		RawText: "chili crisp eggs, scallion oil, fast",
		Stage:   ideas.StageIdea,
		Metadata: ideas.Metadata{
			Difficulty: "easy", DurationClass: "quick",
			Treatment: "elevated", Cuisine: "Chinese-inspired",
		},
		Enrichment: ideas.Enrichment{Status: ideas.EnrichOK, EnrichedAt: &at},
	}
}

func TestRenderIdeaCardShowsMetadata(t *testing.T) {
	text, markup := RenderIdeaCard(enrichedIdea(), "http://localhost:8420")

	for _, want := range []string{"Crispy chili eggs", "Easy", "Quick", "Elevated", "Chinese-inspired"} {
		if !strings.Contains(text, want) {
			t.Errorf("card omits %q:\n%s", want, text)
		}
	}
	if markup == nil || len(markup.InlineKeyboard) == 0 {
		t.Fatal("an enriched card must carry an Open button")
	}
}

// No ID is ever shown, copied, or pasted. That was the defect in the previous
// iteration and it must not reappear.
func TestRenderIdeaCardNeverShowsRawID(t *testing.T) {
	idea := enrichedIdea()
	text, _ := RenderIdeaCard(idea, "http://localhost:8420")

	if strings.Contains(text, idea.ID) {
		t.Errorf("card leaks the raw id into visible text:\n%s", text)
	}
}

func TestRenderIdeaCardDeepLinksToTheWebApp(t *testing.T) {
	idea := enrichedIdea()
	_, markup := RenderIdeaCard(idea, "http://localhost:8420")

	var found string
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.URL != "" {
				found = button.URL
			}
		}
	}
	want := "http://localhost:8420/ideas/" + idea.ID
	if found != want {
		t.Errorf("deep link = %q, want %q", found, want)
	}
}

func TestRenderIdeaCardPendingSaysReading(t *testing.T) {
	idea := enrichedIdea()
	idea.Enrichment = ideas.Enrichment{Status: ideas.EnrichPending}

	text, _ := RenderIdeaCard(idea, "http://localhost:8420")
	if !strings.Contains(strings.ToLower(text), "reading") {
		t.Errorf("a pending card should say it is still reading:\n%s", text)
	}
}

func TestRenderIdeaCardFailedShowsErrorAndRetry(t *testing.T) {
	idea := enrichedIdea()
	idea.Enrichment = ideas.Enrichment{
		Status: ideas.EnrichFailed,
		Error:  "401 authentication_error: invalid x-api-key",
	}

	text, markup := RenderIdeaCard(idea, "http://localhost:8420")
	if !strings.Contains(text, "401") {
		t.Errorf("a failed card must show the message:\n%s", text)
	}

	var hasRetry bool
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.Contains(strings.ToLower(button.Text), "retry") {
				hasRetry = true
			}
		}
	}
	if !hasRetry {
		t.Error("a failed card must offer Retry")
	}
}

func TestParseCommandSplitsNameAndArgs(t *testing.T) {
	cases := []struct{ in, name, args string }{
		{"/s shawarma", "s", "shawarma"},
		{"/s  lots   of  words ", "s", "lots   of  words"},
		{"/recent", "recent", ""},
		{"/help", "help", ""},
		{"/s@souschef_bot shawarma", "s", "shawarma"}, // group-style mention
		{"not a command", "", ""},
		{"", "", ""},
	}

	for _, c := range cases {
		name, args := parseCommand(c.in)
		if name != c.name || args != c.args {
			t.Errorf("parseCommand(%q) = (%q, %q), want (%q, %q)", c.in, name, args, c.name, c.args)
		}
	}
}

func TestEscapeHTMLPreventsMarkupInjection(t *testing.T) {
	// An idea title is user-supplied and lands in an HTML-parsed message.
	got := escapeHTML(`<b>bold</b> & "quoted"`)
	for _, forbidden := range []string{"<b>", "</b>"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("escapeHTML left %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("escapeHTML did not escape &: %q", got)
	}
}
