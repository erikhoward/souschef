import { useState } from 'react';

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

function IdeaRow({ idea, isSelected, onSelect, onRetry }) {
  const { metadata, enrichment } = idea;
  return (
    <button className={`idea-row ${isSelected ? 'is-selected' : ''}`} type="button" onClick={() => onSelect(idea.id)}>
      <span className="row-title-group">
        <span className="selection-mark"><Icon name={isSelected ? 'check' : 'lightbulb'} size={15} /></span>
        <span><strong>{idea.title}</strong><small>{idea.raw_text}</small></span>
      </span>
      <FoodThumbnail idea={idea} />
      <span className="row-meta">
        <MetaIcon icon="lightbulb" tone={metadata.difficulty}>{labelize(metadata.difficulty)}</MetaIcon>
        <MetaIcon icon="clock">{DURATION_LABELS[metadata.duration_class] ?? labelize(metadata.duration_class)}</MetaIcon>
        <MetaIcon icon="leaf">{labelize(metadata.treatment)}</MetaIcon>
        <MetaIcon icon="globe">{metadata.cuisine || '—'}</MetaIcon>
        <MetaIcon icon="egg">{metadata.primary_ingredient || '—'}</MetaIcon>
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
      {enrichment.status === 'failed' ? (
        <span
          className="row-action"
          onClick={(event) => { event.stopPropagation(); onRetry(idea.id); }}
        >
          Retry enrichment<Icon name="arrow" size={18} />
        </span>
      ) : enrichment.status === 'pending' ? (
        <span className="row-action">Enriching…</span>
      ) : (
        <span className="row-action" onClick={(event) => { event.stopPropagation(); onSelect(idea.id); }}>
          View idea<Icon name="arrow" size={18} />
        </span>
      )}
    </button>
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

function IdeaInspector({ idea, allIdeas, store, guard, announce, onSelect, onClose }) {
  const [notesDraft, setNotesDraft] = useState('');
  const isArchived = Boolean(idea.archived_at);
  const linkCandidates = allIdeas.filter((candidate) => candidate.id !== idea.id && !idea.linked_ids.includes(candidate.id) && !candidate.archived_at);
  const { metadata, enrichment } = idea;

  const handleAddNote = async (event) => {
    event.preventDefault();
    const body = notesDraft.trim();
    if (!body) return;
    const result = await guard(store.addNote, 'Note added.')(idea.id, body);
    if (result) setNotesDraft('');
  };

  return (
    <aside className="inspector" aria-label={`${idea.title} details`}>
      <button className="icon-button inspector-close" type="button" onClick={onClose} aria-label="Close details"><Icon name="x" size={20} /></button>
      <h2>{idea.title}</h2>
      <PipelineStepper stage={idea.stage} />

      {enrichment.status === 'pending' && (
        <p className="muted-copy">Enrichment is running — metadata will fill in shortly.</p>
      )}
      {enrichment.status === 'failed' && (
        <div className="empty-list" role="alert">
          {enrichment.error || 'Enrichment failed.'}
          <button className="text-button" type="button" onClick={guard(() => store.reenrich(idea.id), 'Retrying enrichment…')}>
            Retry enrichment
          </button>
        </div>
      )}

      <section className="inspector-section metadata-section">
        <div className="section-title"><h3>Inferred metadata</h3></div>
        <dl className="metadata-list">
          <div><dt><Icon name="lightbulb" size={16} />Difficulty</dt><dd><i className={`status-dot ${metadata.difficulty}`} />{labelize(metadata.difficulty)}</dd></div>
          <div><dt><Icon name="clock" size={16} />Duration</dt><dd><Icon name="clock" size={15} />{DURATION_LABELS[metadata.duration_class] ?? labelize(metadata.duration_class)}</dd></div>
          <div><dt><Icon name="leaf" size={16} />Treatment</dt><dd><Icon name="leaf" size={15} />{labelize(metadata.treatment)}</dd></div>
          <div><dt><Icon name="globe" size={16} />Cuisine</dt><dd><Icon name="globe" size={15} />{metadata.cuisine || '—'}</dd></div>
          <div><dt><Icon name="egg" size={16} />Primary ingredient</dt><dd><Icon name="egg" size={15} />{metadata.primary_ingredient || '—'}</dd></div>
          <div><dt><Icon name="video" size={16} />Content type</dt><dd><Icon name="video" size={15} />{metadata.content_type === 'vlog' ? 'Vlog' : 'Video + Recipe'}</dd></div>
          <div><dt><Icon name="sparkles" size={16} />Visual potential</dt><dd><i className="status-dot easy" />{labelize(metadata.visual_potential)}</dd></div>
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

            {error && <div className="empty-list" role="alert">{error}</div>}
            {loading && !ideas.length && <div className="empty-list">Loading…</div>}

            {ideas.map((idea) => (
              <IdeaRow
                key={idea.id}
                idea={idea}
                isSelected={idea.id === selectedId}
                onSelect={onSelect}
                onRetry={guard(store.reenrich, 'Retrying enrichment…')}
              />
            ))}

            {!loading && !ideas.length && !error && (
              <div className="empty-list">
                <Icon name="search" size={28} />
                {active.q ? 'No ideas match that search.' : 'Nothing captured yet.'}
              </div>
            )}
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
