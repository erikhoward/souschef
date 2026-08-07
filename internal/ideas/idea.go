// Package ideas holds the domain model and the rules that govern it. It knows
// nothing about SQL, HTTP, or Telegram.
package ideas

import "time"

type Stage string

const (
	StageIdea            Stage = "idea"
	StageBriefReady      Stage = "brief_ready"
	StageRecipeReview    Stage = "recipe_review"
	StageScriptReady     Stage = "script_ready"
	StageProductionReady Stage = "production_ready"
)

type Source string

const (
	SourceWeb           Source = "web"
	SourceTelegramText  Source = "telegram_text"
	SourceTelegramVoice Source = "telegram_voice"
)

type EnrichmentStatus string

const (
	EnrichPending EnrichmentStatus = "pending"
	EnrichOK      EnrichmentStatus = "ok"
	EnrichFailed  EnrichmentStatus = "failed"
)

// Metadata is everything Claude infers about an idea. Every field is
// optional: an idea is fully usable before enrichment lands, and must be.
type Metadata struct {
	Title             string   `json:"title"`
	Difficulty        string   `json:"difficulty"`     // easy | moderate | insane
	DurationClass     string   `json:"duration_class"` // quick | average | multi_day
	Treatment         string   `json:"treatment"`      // elevated | non_elevated
	ContentType       string   `json:"content_type"`   // recipe | vlog
	Cuisine           string   `json:"cuisine"`
	PrimaryIngredient string   `json:"primary_ingredient"`
	Equipment         []string `json:"equipment"`
	VisualPotential   string   `json:"visual_potential"`  // low | medium | high
	Seasonality       string   `json:"seasonality"`       // spring | summer | fall | winter | all_year
	ProductionEffort  string   `json:"production_effort"` // light | average | heavy
	Tags              []string `json:"tags"`
}

// Enrichment records what happened on the Claude call. Error holds the
// provider's message verbatim so a dead key reads as "401
// authentication_error" on the row instead of as a spinner that never stops.
type Enrichment struct {
	Status     EnrichmentStatus `json:"status"`
	Error      string           `json:"error,omitempty"`
	Model      string           `json:"model,omitempty"`
	EnrichedAt *time.Time       `json:"enriched_at,omitempty"`
}

type Note struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Idea struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	RawText   string `json:"raw_text"`
	Source    Source `json:"source"`
	SourceRef string `json:"source_ref,omitempty"`
	Stage     Stage  `json:"stage"`

	// ArchivedAt is separate from Stage on purpose. Archiving must not
	// destroy how far an idea got through the pipeline.
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
	MergedIntoID *string    `json:"merged_into_id,omitempty"`

	Metadata Metadata `json:"metadata"`

	// FieldOverrides names metadata fields a human has corrected.
	// Re-enrichment must not overwrite anything listed here.
	FieldOverrides []string `json:"field_overrides"`

	Enrichment Enrichment `json:"enrichment"`

	Notes     []Note   `json:"notes"`
	LinkedIDs []string `json:"linked_ids"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (i Idea) IsArchived() bool { return i.ArchivedAt != nil }
func (i Idea) IsMerged() bool   { return i.MergedIntoID != nil }

// HasOverride reports whether a human has corrected the named field.
func (i Idea) HasOverride(field string) bool {
	for _, f := range i.FieldOverrides {
		if f == field {
			return true
		}
	}
	return false
}

// ArchivedScope selects which archived state a listing includes.
type ArchivedScope string

const (
	ArchivedExclude ArchivedScope = "false" // default
	ArchivedOnly    ArchivedScope = "true"
	ArchivedAll     ArchivedScope = "all"
)

type ListFilter struct {
	Query      string // when set, FTS rank wins and Sort is ignored
	Stage      string
	Difficulty string
	Duration   string
	Treatment  string
	Archived   ArchivedScope
	Sort       string // created_at | updated_at | title | difficulty | duration
	Order      string // asc | desc
	Limit      int
}
