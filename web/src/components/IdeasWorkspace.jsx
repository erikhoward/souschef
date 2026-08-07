import { useEffect, useState } from 'react';

import { thumbnailProps } from '../lib/thumbnail.js';
import { Icon } from './Icon.jsx';

const STATUS_LABELS = {
  idea: 'Backlog',
  brief_ready: 'Brief ready',
  recipe_review: 'In recipe review',
  script_ready: 'Script ready',
  production_ready: 'Ready to produce',
};

const PIPELINE_STEPS = [
  ['idea', 'Idea'],
  ['brief_ready', 'Brief'],
  ['recipe_review', 'Recipe'],
  ['script_ready', 'Script'],
  ['production_ready', 'Produce'],
];

const filters = [
  ['', 'All'],
  ['idea', 'Backlog'],
  ['brief_ready', 'Brief'],
  ['recipe_review', 'Recipe'],
  ['script_ready', 'Script'],
];

const labelize = (value) => (value ? value.replaceAll('_', ' ') : '—');

const DURATION_LABELS = { quick: '20 min', average: '45 min', multi_day: 'Multi-day' };

// A pending idea normally resolves within a few seconds. Past this, the
// enrichment worker may have died mid-run (process killed, or shutdown
// drained past its grace period) with no automatic re-drive on restart.
// Once an idea has been pending longer than this, the row grows a Retry
// affordance instead of leaving it stuck with none. The inspector offers
// Retry for a pending idea immediately, since opening it is already a
// deliberate action.
const PENDING_STALE_MS = 45_000;

function FoodThumbnail({ idea }) {
  const { letter, style, label } = thumbnailProps(idea);
  return (
    <span className="food-thumbnail" style={style} role="img" aria-label={label}>
      {letter}
    </span>
  );
}

function MetaIcon({ icon, children, tone = '' }) {
  return <span className={`meta-value ${tone}`}><Icon name={icon} size={14} />{children}</span>;
}

function PipelineStepper({ stage }) {
  const index = Math.max(0, PIPELINE_STEPS.findIndex(([id]) => id === stage));
  return (
    <ol className="pipeline-stepper" aria-label="Idea workflow stage">
      {PIPELINE_STEPS.map(([id, label], stepIndex) => (
        <li className={stepIndex <= index ? 'is-complete' : ''} key={id}>
          <span className={stepIndex === index ? 'is-current' : ''} />
          <small>{label}</small>
        </li>
      ))}
    </ol>
  );
}

// The row is a div[role=button], not a <button>. The Retry control it can
// contain is itself an interactive control, and nesting a <button> inside a
// <button> is invalid HTML that leaves the inner control unreachable by
// keyboard in some browsers. A div with an explicit role and key handling
// gives the same activation behaviour without that trap.
function IdeaRow({ idea, isSelected, onSelect, onRetry, now }) {
  const { metadata, enrichment } = idea;
  const isStalePending = enrichment.status === 'pending'
    && now - new Date(idea.created_at).getTime() > PENDING_STALE_MS;

  return (
    <div
      className={`idea-row ${isSelected ? 'is-selected' : ''} ${enrichment.status === 'failed' ? 'is-failed' : ''}`}
      role="button"
      tabIndex={0}
      onClick={() => onSelect(idea.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onSelect(idea.id);
        }
      }}
    >
      <span className="row-title-group">
        <span className="selection-mark"><Icon name={isSelected ? 'check' : 'lightbulb'} size={15} /></span>
        <span><strong>{idea.title}</strong><small>{idea.raw_text}</small></span>
      </span>
      <FoodThumbnail idea={idea} />
      <span className="row-meta">
        {enrichment.status === 'pending' && (
          <span className={`meta-pending ${isStalePending ? 'is-stale' : ''}`}>
            Reading…
            {isStalePending && (
              <button
                type="button"
                className="text-button"
                onClick={(event) => { event.stopPropagation(); onRetry(idea.id); }}
              >
                Retry
              </button>
            )}
          </span>
        )}

        {enrichment.status === 'failed' && (
          <span className="meta-failed">
            <Icon name="x" size={13} />
            <span className="meta-failed-text" title={enrichment.error}>{enrichment.error}</span>
            <button
              type="button"
              className="text-button"
              onClick={(event) => { event.stopPropagation(); onRetry(idea.id); }}
            >
              Retry
            </button>
          </span>
        )}

        {enrichment.status === 'ok' && (
          <>
            <MetaIcon icon="lightbulb" tone={metadata.difficulty}>{labelize(metadata.difficulty)}</MetaIcon>
            <MetaIcon icon="clock">{DURATION_LABELS[metadata.duration_class] ?? labelize(metadata.duration_class)}</MetaIcon>
            <MetaIcon icon="leaf">{labelize(metadata.treatment)}</MetaIcon>
            <MetaIcon icon="globe">{metadata.cuisine || '—'}</MetaIcon>
            <MetaIcon icon="egg">{metadata.primary_ingredient || '—'}</MetaIcon>
          </>
        )}
      </span>
      <span className="content-type">
        <Icon name={metadata.content_type === 'vlog' ? 'video' : 'book'} size={19} />
        {metadata.content_type === 'vlog' ? 'Vlog' : 'Video + Recipe'}
      </span>
      <span className="status-cell">
        <i className={`status-dot ${idea.archived_at ? 'archived' : idea.stage}`} />
        {idea.archived_at ? 'Archived' : STATUS_LABELS[idea.stage]}
        <small>{new Date(idea.updated_at).toLocaleString()}</small>
      </span>
      <span className="row-action">
        Open<Icon name="arrow" size={18} />
      </span>
    </div>
  );
}

function CaptureComposer({ captureValue, onCaptureChange, onCapture, onFocus }) {
  return (
    <form className="capture-composer" onSubmit={onCapture}>
      <div className="capture-heading"><h2>What are you cooking up?</h2><span>Be messy. We’ll handle the rest.</span></div>
      <div className="capture-control">
        <textarea value={captureValue} onFocus={onFocus} onChange={(event) => onCaptureChange(event.target.value)} maxLength="500" placeholder="Sheet-pan shawarma with a lemony feta situation…" aria-label="New idea" />
        <div className="capture-actions"><small>{captureValue.length}/500</small><button className="button button-primary" type="submit" disabled={!captureValue.trim()}>Save idea <kbd>⌘↵</kbd></button></div>
      </div>
    </form>
  );
}

function FilterBar({ active, onChange }) {
  return (
    <div className="filter-bar">
      <div className="filter-tabs" role="tablist" aria-label="Idea stage">
        {filters.map(([id, label]) => (
          <button
            key={id || 'all'}
            type="button"
            className={active.stage === id ? 'is-active' : ''}
            onClick={() => onChange({ stage: id })}
          >
            {label}
          </button>
        ))}
        <button
          type="button"
          className={active.archived === 'true' ? 'is-active' : ''}
          onClick={() => onChange({ archived: active.archived === 'true' ? '' : 'true' })}
        >
          Archived
        </button>
      </div>
      <div className="filter-selects">
        <select value={active.difficulty} onChange={(e) => onChange({ difficulty: e.target.value })}
                aria-label="Filter by difficulty">
          <option value="">Difficulty</option>
          <option value="easy">Easy</option>
          <option value="moderate">Moderate</option>
          <option value="insane">Insane</option>
        </select>
        <select value={active.duration} onChange={(e) => onChange({ duration: e.target.value })}
                aria-label="Filter by duration">
          <option value="">Duration</option>
          <option value="quick">Quick</option>
          <option value="average">Average</option>
          <option value="multi_day">Multi-day</option>
        </select>
        <select value={active.treatment} onChange={(e) => onChange({ treatment: e.target.value })}
                aria-label="Filter by treatment">
          <option value="">Treatment</option>
          <option value="elevated">Elevated</option>
          <option value="non_elevated">Non-elevated</option>
        </select>
        <select value={active.sort} onChange={(e) => onChange({ sort: e.target.value })}
                aria-label="Sort by">
          <option value="created_at">Newest</option>
          <option value="updated_at">Recently updated</option>
          <option value="title">Title</option>
          <option value="difficulty">Difficulty</option>
          <option value="duration">Duration</option>
        </select>
        <Icon name="sliders" size={20} />
      </div>
    </div>
  );
}

function RelatedIdeas({ allIdeas, ids, onSelect }) {
  const related = allIdeas.filter((idea) => ids.includes(idea.id));
  if (!related.length) return <p className="muted-copy">No linked ideas yet. A good sign you have room for one more.</p>;
  return (
    <div className="related-list">
      {related.map((idea) => (
        <button type="button" onClick={() => onSelect(idea.id)} key={idea.id}>
          <FoodThumbnail idea={idea} />
          <span>{idea.title}<small><i className={`status-dot ${idea.stage}`} />{STATUS_LABELS[idea.stage]}</small></span>
        </button>
      ))}
    </div>
  );
}

function LinkControl({ candidates, onLink }) {
  const [relatedId, setRelatedId] = useState('');
  if (!candidates.length) return null;
  return (
    <div className="link-control">
      <select value={relatedId} onChange={(event) => setRelatedId(event.target.value)} aria-label="Related idea">
        <option value="">Link another idea…</option>
        {candidates.map((candidate) => <option value={candidate.id} key={candidate.id}>{candidate.title}</option>)}
      </select>
      <button className="text-button" type="button" disabled={!relatedId} onClick={() => { onLink(relatedId); setRelatedId(''); }}>Link</button>
    </div>
  );
}

// Enumerated fields render as a <select> of the exact values the backend
// accepts; everything else is free text. `equipment` and `tags` are arrays
// on the wire, so they get a comma-separated text representation that is
// parsed back into a list on blur.
const FIELD_OPTIONS = {
  difficulty: ['easy', 'moderate', 'insane'],
  duration_class: ['quick', 'average', 'multi_day'],
  treatment: ['elevated', 'non_elevated'],
  content_type: ['recipe', 'vlog'],
  visual_potential: ['low', 'medium', 'high'],
  seasonality: ['spring', 'summer', 'fall', 'winter', 'all_year'],
  production_effort: ['light', 'average', 'heavy'],
};

const ARRAY_FIELDS = ['equipment', 'tags'];

const FIELD_LABELS = {
  title: 'Title',
  difficulty: 'Difficulty',
  duration_class: 'Duration',
  treatment: 'Treatment',
  content_type: 'Content type',
  cuisine: 'Cuisine',
  primary_ingredient: 'Primary ingredient',
  visual_potential: 'Visual potential',
  seasonality: 'Seasonality',
  production_effort: 'Production effort',
  equipment: 'Equipment',
  tags: 'Tags',
};

const arraysEqual = (a, b) => a.length === b.length && a.every((v, i) => v === b[i]);

// A corrected field is marked so it is obvious which values are yours and
// which the model's — and so the protection from re-enrichment is visible
// rather than an invisible rule.
function CorrectableField({ idea, field, onCorrect }) {
  const options = FIELD_OPTIONS[field];
  const isArray = ARRAY_FIELDS.includes(field);
  const overridden = idea.field_overrides.includes(field);
  const arrayValue = isArray ? (idea.metadata[field] ?? []) : null;
  // `title` is the one correctable field that lives on the idea itself, not
  // under `metadata` — the backend maps it to `Idea.Title`, and
  // `metadata.title` is an unrelated, always-empty field on the wire.
  const value = field === 'title'
    ? idea.title
    : isArray ? arrayValue.join(', ') : (idea.metadata[field] ?? '');

  return (
    <div>
      <dt>
        {FIELD_LABELS[field]}
        {overridden && <span className="override-mark" title="You set this. Re-enrichment will not change it.">✎</span>}
      </dt>
      <dd>
        {options ? (
          <select value={value} onChange={(event) => onCorrect(field, event.target.value)}
                  aria-label={FIELD_LABELS[field]}>
            <option value="">—</option>
            {options.map((option) => (
              <option key={option} value={option}>{labelize(option)}</option>
            ))}
          </select>
        ) : (
          <input
            type="text"
            defaultValue={value}
            aria-label={FIELD_LABELS[field]}
            placeholder={isArray ? 'Comma-separated' : undefined}
            onBlur={(event) => {
              if (isArray) {
                const parsed = event.target.value.split(',').map((v) => v.trim()).filter(Boolean);
                if (!arraysEqual(parsed, arrayValue)) onCorrect(field, parsed);
              } else if (event.target.value !== value) {
                onCorrect(field, event.target.value);
              }
            }}
          />
        )}
      </dd>
    </div>
  );
}

function IdeaInspector({ idea, allIdeas, store, guard, announce, onSelect, onClose }) {
  const [notesDraft, setNotesDraft] = useState('');
  const [mergeTarget, setMergeTarget] = useState('');
  const isArchived = Boolean(idea.archived_at);
  const linkCandidates = allIdeas.filter((candidate) => candidate.id !== idea.id && !idea.linked_ids.includes(candidate.id) && !candidate.archived_at);
  // Merging is a corrective action for duplicates, so it deliberately does
  // not exclude archived ideas or ideas already linked the way linking does.
  const mergeCandidates = allIdeas.filter((candidate) => candidate.id !== idea.id);
  const { enrichment } = idea;

  const handleAddNote = async (event) => {
    event.preventDefault();
    const body = notesDraft.trim();
    if (!body) return;
    const result = await guard(store.addNote, 'Note added.')(idea.id, body);
    if (result) setNotesDraft('');
  };

  const correct = guard(
    (field, value) => store.patch(idea.id, { [field]: value }),
    'Saved. Re-enrichment will leave that field alone.'
  );

  const handleMerge = async () => {
    if (!mergeTarget) return;
    const result = await guard(store.merge, 'Merged.')(idea.id, mergeTarget);
    if (result) setMergeTarget('');
  };

  return (
    <aside className="inspector" aria-label={`${idea.title} details`}>
      <button className="icon-button inspector-close" type="button" onClick={onClose} aria-label="Close details"><Icon name="x" size={20} /></button>
      <h2>{idea.title}</h2>
      <PipelineStepper stage={idea.stage} />

      {enrichment.status === 'pending' && (
        <div className="inspector-pending" role="status">
          <strong>Reading this idea…</strong>
          <p>Metadata will fill in shortly. If this sticks around, retry it.</p>
          <button className="button button-outline" type="button"
                  onClick={guard(() => store.reenrich(idea.id), 'Retrying…')}>
            Retry enrichment
          </button>
        </div>
      )}
      {enrichment.status === 'failed' && (
        <div className="inspector-alert" role="alert">
          <strong>Enrichment failed</strong>
          <p>{enrichment.error || 'Enrichment failed.'}</p>
          <button className="button button-outline" type="button"
                  onClick={guard(() => store.reenrich(idea.id), 'Retrying…')}>
            Retry enrichment
          </button>
        </div>
      )}

      <section className="inspector-section metadata-section">
        <div className="section-title"><h3>Metadata</h3></div>
        <dl className="metadata-list">
          {Object.keys(FIELD_LABELS).map((field) => (
            <CorrectableField key={field} idea={idea} field={field} onCorrect={correct} />
          ))}
        </dl>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Notes</h3></div>
        {idea.notes.length > 0 && (
          <ul className="notes-list">
            {idea.notes.map((note) => (
              <li key={note.id}>
                <p>{note.body}</p>
                <small>{new Date(note.created_at).toLocaleString()}</small>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={handleAddNote} className="capture-control">
          <textarea
            className="notes-input"
            value={notesDraft}
            onChange={(event) => setNotesDraft(event.target.value)}
            placeholder="Add a note…"
            aria-label="Add a note"
          />
          <div className="capture-actions">
            <button className="text-button" type="submit" disabled={!notesDraft.trim()}>Add note</button>
          </div>
        </form>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Related ideas</h3></div>
        <RelatedIdeas allIdeas={allIdeas} ids={idea.linked_ids} onSelect={onSelect} />
        <LinkControl
          candidates={linkCandidates}
          onLink={guard((otherId) => store.link(idea.id, otherId), 'Linked as a related idea.')}
        />
      </section>

      <div className="inspector-actions">
        {mergeCandidates.length > 0 && (
          <div className="link-control">
            <select value={mergeTarget} onChange={(event) => setMergeTarget(event.target.value)}
                    aria-label="Merge a duplicate into this idea">
              <option value="">Merge a duplicate in…</option>
              {mergeCandidates.map((candidate) => (
                <option key={candidate.id} value={candidate.id}>{candidate.title}</option>
              ))}
            </select>
            <button className="text-button" type="button" disabled={!mergeTarget} onClick={handleMerge}>
              Merge
            </button>
          </div>
        )}

        <div className="secondary-actions">
          <button
            type="button"
            className="text-button"
            onClick={guard(() => (isArchived ? store.restore(idea.id) : store.archive(idea.id)), 'Idea status updated.')}
          >
            <Icon name={isArchived ? 'arrow' : 'archive'} size={15} />{isArchived ? 'Restore' : 'Archive'}
          </button>
          <button
            type="button"
            className="text-button delete-button"
            onClick={async () => {
              if (!window.confirm(`Delete “${idea.title}”? This cannot be undone.`)) return;
              const result = await guard(() => store.remove(idea.id), 'Idea deleted.')();
              if (result !== null) onClose();
            }}
          >
            <Icon name="trash" size={15} />Delete
          </button>
        </div>
      </div>
    </aside>
  );
}

export function IdeasWorkspace({
  store, filters: active, onFiltersChange, selectedId, onSelect,
  captureValue, onCaptureChange, onCreate, onCaptureFocus, announce, guard,
}) {
  const { ideas, loading, error } = store;
  const selectedIdea = ideas.find((idea) => idea.id === selectedId) ?? null;
  const retry = guard(store.reenrich, 'Retrying enrichment…');

  // Ticks only while an idea is pending, so a row can notice it has gone
  // stale and grow a Retry affordance without polling forever.
  const [now, setNow] = useState(() => Date.now());
  const hasPending = ideas.some((idea) => idea.enrichment.status === 'pending');
  useEffect(() => {
    if (!hasPending) return undefined;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 15_000);
    return () => window.clearInterval(timer);
  }, [hasPending]);

  return (
    <main className="workspace ideas-workspace">
      <header className="workspace-header">
        <h1>Ideas</h1>
        <label className="search-box">
          <Icon name="search" size={21} />
          <input
            type="search"
            value={active.q}
            onChange={(event) => onFiltersChange({ q: event.target.value })}
            placeholder="Search ideas"
            aria-label="Search ideas"
          />
          <kbd>⌘ K</kbd>
        </label>
        <button className="button button-primary header-capture" type="button" onClick={onCaptureFocus}>
          <Icon name="plus" size={18} />Capture idea
        </button>
      </header>

      <div className="ideas-layout">
        <section className="ideas-main">
          <CaptureComposer
            captureValue={captureValue}
            onCaptureChange={onCaptureChange}
            onCapture={onCreate}
            onFocus={onCaptureFocus}
          />
          <FilterBar active={active} onChange={onFiltersChange} />

          <section className="idea-list" aria-label="Idea backlog">
            <div className="idea-list-heading">
              <span>Title</span><span /><span>Meta</span>
              <span>Content type</span><span>Status</span><span>Next action</span>
            </div>

            {/* A dedicated wrapper keeps the heading row out of the sibling
                chain: .idea-row must be the true :first-child / :last-of-type
                among the rows themselves (Playwright's e2e suite depends on
                that), not merely the first row-shaped element under a parent
                that also holds the heading, loading, and empty states. */}
            <div className="idea-rows">
              {error && <div className="empty-list" role="alert">{error}</div>}
              {loading && !ideas.length && <div className="empty-list">Loading…</div>}

              {ideas.map((idea) => (
                <IdeaRow
                  key={idea.id}
                  idea={idea}
                  isSelected={idea.id === selectedId}
                  onSelect={onSelect}
                  onRetry={retry}
                  now={now}
                />
              ))}

              {!loading && !ideas.length && !error && (
                <div className="empty-list">
                  <Icon name="search" size={28} />
                  {active.q ? 'No ideas match that search.' : 'Nothing captured yet.'}
                </div>
              )}
            </div>
            <footer>{ideas.length} idea{ideas.length === 1 ? '' : 's'}</footer>
          </section>
        </section>

        {selectedIdea && (
          <IdeaInspector
            idea={selectedIdea}
            allIdeas={ideas}
            store={store}
            guard={guard}
            announce={announce}
            onSelect={onSelect}
            onClose={() => onSelect(null)}
          />
        )}
      </div>
    </main>
  );
}
