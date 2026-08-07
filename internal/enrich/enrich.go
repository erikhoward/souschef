// Package enrich turns raw capture text into structured metadata using Claude.
//
// It never touches the database. Enrich is a function from text to metadata,
// which makes the expensive, nondeterministic, key-requiring part of the
// system replayable from fixtures in tests.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/erikhoward/souschef/internal/ideas"
)

// taxonomy lists the permitted values per enum field. It is the single source
// of truth for both the JSON schema we send and the validation we apply on
// the way back — a model can return well-formed JSON with a value we never
// asked for, and storing it would corrupt every filter downstream.
var taxonomy = map[string][]string{
	"difficulty":        {"easy", "moderate", "insane"},
	"duration_class":    {"quick", "average", "multi_day"},
	"treatment":         {"elevated", "non_elevated"},
	"content_type":      {"recipe", "vlog"},
	"visual_potential":  {"low", "medium", "high"},
	"seasonality":       {"spring", "summer", "fall", "winter", "all_year"},
	"production_effort": {"light", "average", "heavy"},
}

func enumProp(field string) map[string]any {
	return map[string]any{"type": "string", "enum": taxonomy[field]}
}

// MetadataSchema is sent as output_config.format. Note the deprecated
// top-level output_format parameter is not used anywhere.
var MetadataSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":              map[string]any{"type": "string"},
		"difficulty":         enumProp("difficulty"),
		"duration_class":     enumProp("duration_class"),
		"treatment":          enumProp("treatment"),
		"content_type":       enumProp("content_type"),
		"cuisine":            map[string]any{"type": "string"},
		"primary_ingredient": map[string]any{"type": "string"},
		"equipment":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"visual_potential":   enumProp("visual_potential"),
		"seasonality":        enumProp("seasonality"),
		"production_effort":  enumProp("production_effort"),
		"tags":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required": []string{
		"title", "difficulty", "duration_class", "treatment", "content_type",
		"visual_potential", "seasonality", "production_effort",
	},
	"additionalProperties": false,
}

func allowed(field, value string) bool {
	for _, v := range taxonomy[field] {
		if v == value {
			return true
		}
	}
	return false
}

// ParseResponse decodes and validates a model response. It is pure — every
// taxonomy test runs against recorded fixtures with no network and no key.
func ParseResponse(b []byte) (ideas.Metadata, error) {
	var m ideas.Metadata
	if err := json.Unmarshal(b, &m); err != nil {
		return ideas.Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}

	if strings.TrimSpace(m.Title) == "" {
		return ideas.Metadata{}, errors.New("metadata: title must not be empty")
	}

	for field, value := range map[string]string{
		"difficulty":        m.Difficulty,
		"duration_class":    m.DurationClass,
		"treatment":         m.Treatment,
		"content_type":      m.ContentType,
		"visual_potential":  m.VisualPotential,
		"seasonality":       m.Seasonality,
		"production_effort": m.ProductionEffort,
	} {
		if !allowed(field, value) {
			return ideas.Metadata{}, fmt.Errorf(
				"metadata: %s = %q is outside the taxonomy (allowed: %s)",
				field, value, strings.Join(taxonomy[field], ", "))
		}
	}

	if m.Equipment == nil {
		m.Equipment = []string{}
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	return m, nil
}

// Failure is a classified enrichment error, ready to be written to the row.
type Failure struct {
	Message   string
	Retryable bool
}

// statusCarrier is satisfied by the SDK's error type and by the test stub.
type statusCarrier interface {
	Error() string
	StatusCode() int
}

type requestIDCarrier interface{ RequestID() string }

// Classify turns any error into a message for the idea row and a retry
// decision. Nothing is swallowed: a transport failure with no status still
// produces text a human can act on.
func Classify(err error) Failure {
	if err == nil {
		return Failure{}
	}

	var sc statusCarrier
	if errors.As(err, &sc) {
		msg := fmt.Sprintf("%d %s", sc.StatusCode(), sc.Error())

		var rc requestIDCarrier
		if errors.As(err, &rc) && rc.RequestID() != "" {
			msg += fmt.Sprintf(" (request_id=%s)", rc.RequestID())
		}

		switch code := sc.StatusCode(); {
		case code == 401 || code == 403:
			// The key is wrong or unauthorized. Retrying cannot help, and
			// retrying quietly is how this became invisible last time.
			return Failure{Message: msg, Retryable: false}
		case code == 400 || code == 404 || code == 413:
			return Failure{Message: msg, Retryable: false}
		case code == 429 || code >= 500:
			return Failure{Message: msg, Retryable: true}
		default:
			return Failure{Message: msg, Retryable: false}
		}
	}

	return Failure{Message: err.Error(), Retryable: true}
}

type Enricher struct {
	client anthropic.Client
	model  string
	effort string
}

// New builds an Enricher. The client resolves credentials itself: an
// ANTHROPIC_API_KEY, an ANTHROPIC_AUTH_TOKEN, or an `ant auth login` profile.
func New(model, effort string) *Enricher {
	return &Enricher{client: anthropic.NewClient(), model: model, effort: effort}
}

const (
	enrichTimeout = 60 * time.Second
	maxAttempts   = 3
)

// Enrich classifies rawText. On a retryable failure it backs off and tries
// again up to maxAttempts; on a terminal failure it returns immediately.
func (e *Enricher) Enrich(ctx context.Context, rawText string) (ideas.Metadata, error) {
	var last error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, enrichTimeout)
		raw, err := e.call(attemptCtx, rawText)
		cancel()

		if err == nil {
			return ParseResponse(raw)
		}
		last = err

		if f := Classify(err); !f.Retryable || attempt == maxAttempts {
			return ideas.Metadata{}, err
		}

		select {
		case <-ctx.Done():
			return ideas.Metadata{}, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return ideas.Metadata{}, last
}

// call issues one request and returns the raw JSON body of the response.
func (e *Enricher) call(ctx context.Context, rawText string) ([]byte, error) {
	msg, err := e.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(e.model),
		MaxTokens: 2048,

		// The taxonomy prompt is byte-identical every call, so it is the
		// cache target. Effort is low: this is a short, scoped classification.
		System: []anthropic.TextBlockParam{{
			Text:         SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},

		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},

		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(e.effort),
			Format: anthropic.JSONOutputFormatParam{Schema: MetadataSchema},
		},

		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(rawText)),
		},
	})
	if err != nil {
		return nil, err
	}

	// A refusal arrives as HTTP 200 with an empty content array, so this must
	// be checked before indexing into Content.
	if msg.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("model declined to classify this idea")
	}

	for _, block := range msg.Content {
		if block.Type == "text" {
			return []byte(block.Text), nil
		}
	}
	return nil, errors.New("response contained no text block")
}
