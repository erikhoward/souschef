import { useState } from 'react';

import { PIPELINE_STEPS, STATUS_LABELS } from '../data/ideas.js';
import { Icon } from './Icon.jsx';

const filters = [
  ['all', 'All'],
  ['idea', 'Backlog'],
  ['brief_ready', 'Brief'],
  ['recipe_review', 'Recipe'],
  ['script_ready', 'Script'],
  ['archived', 'Archived']
];

const nextActions = {
  idea: ['Generate brief', 'brief'],
  brief_ready: ['Generate recipe', 'recipe'],
  recipe_review: ['Review recipe', 'recipe'],
  script_ready: ['Review script', 'script'],
  production_ready: ['Plan production', 'production']
};

const labelize = (value) => value.replaceAll('_', ' ');

function FoodThumbnail({ idea }) {
  return (
    <span
      className="food-thumbnail"
      style={{ '--thumb-position': idea.thumbnailPosition ?? '0%' }}
      aria-label={`Visual reference for ${idea.title}`}
      role="img"
    />
  );
}

function MetaIcon({ icon, children, tone = '' }) {
  return <span className={`meta-value ${tone}`}><Icon name={icon} size={14} />{children}</span>;
}

function PipelineStepper({ status }) {
  const index = Math.max(0, PIPELINE_STEPS.findIndex(([id]) => id === status));
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

function IdeaRow({ idea, isSelected, onSelect, onAdvance }) {
  const [action, target] = nextActions[idea.status] ?? ['View idea', 'idea'];
  return (
    <button className={`idea-row ${isSelected ? 'is-selected' : ''}`} type="button" onClick={() => onSelect(idea.id)}>
      <span className="row-title-group">
        <span className="selection-mark"><Icon name={isSelected ? 'check' : 'lightbulb'} size={15} /></span>
        <span><strong>{idea.title}</strong><small>{idea.cue}</small></span>
      </span>
      <FoodThumbnail idea={idea} />
      <span className="row-meta">
        <MetaIcon icon="lightbulb" tone={idea.difficulty}>{labelize(idea.difficulty)}</MetaIcon>
        <MetaIcon icon="clock">{idea.duration === 'quick' ? '20 min' : idea.duration === 'average' ? '45 min' : 'Multi-day'}</MetaIcon>
        <MetaIcon icon="leaf">{labelize(idea.treatment)}</MetaIcon>
        <MetaIcon icon="globe">{idea.cuisine}</MetaIcon>
        <MetaIcon icon="egg">{idea.primaryIngredient}</MetaIcon>
      </span>
      <span className="content-type"><Icon name={idea.contentType === 'vlog' ? 'video' : 'book'} size={19} />{idea.contentType === 'vlog' ? 'Vlog' : 'Video + Recipe'}</span>
      <span className="status-cell"><i className={`status-dot ${idea.status}`} />{STATUS_LABELS[idea.status]}<small>{idea.updatedAt}</small></span>
      <span className="row-action" onClick={(event) => { event.stopPropagation(); onAdvance(idea, target); }}>
        {action}<Icon name="arrow" size={18} />
      </span>
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

function FilterBar({ filter, onFilterChange, difficulty, onDifficultyChange, duration, onDurationChange, treatment, onTreatmentChange }) {
  return (
    <div className="filter-bar">
      <div className="filter-tabs" role="tablist" aria-label="Idea status">
        {filters.map(([id, label]) => <button className={filter === id ? 'is-active' : ''} type="button" onClick={() => onFilterChange(id)} key={id}>{label}</button>)}
      </div>
      <div className="filter-selects">
        <select value={difficulty} onChange={(event) => onDifficultyChange(event.target.value)} aria-label="Filter by difficulty"><option value="all">Difficulty</option><option value="easy">Easy</option><option value="moderate">Moderate</option><option value="insane">Insane</option></select>
        <select value={duration} onChange={(event) => onDurationChange(event.target.value)} aria-label="Filter by duration"><option value="all">Duration</option><option value="quick">Quick</option><option value="average">Average</option><option value="multi_day">Multi-day</option></select>
        <select value={treatment} onChange={(event) => onTreatmentChange(event.target.value)} aria-label="Filter by treatment"><option value="all">Treatment</option><option value="elevated">Elevated</option><option value="non_elevated">Non-elevated</option></select>
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
      {related.map((idea) => <button type="button" onClick={() => onSelect(idea.id)} key={idea.id}><FoodThumbnail idea={idea} /><span>{idea.title}<small><i className={`status-dot ${idea.status}`} />{STATUS_LABELS[idea.status]}</small></span></button>)}
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

function IdeaInspector({ idea, allIdeas, onAdvance, onArchive, onDelete, onLinkRelated, onMerge, onNotesChange, onSelect, onClose }) {
  const [action, target] = nextActions[idea.status] ?? ['View idea', 'idea'];
  const isArchived = idea.status === 'archived';
  const linkCandidates = allIdeas.filter((candidate) => candidate.id !== idea.id && !idea.relatedIdeaIds.includes(candidate.id) && candidate.status !== 'archived');
  return (
    <aside className="inspector" aria-label={`${idea.title} details`}>
      <button className="icon-button inspector-close" type="button" onClick={onClose} aria-label="Close details"><Icon name="x" size={20} /></button>
      <h2>{idea.title}</h2>
      <PipelineStepper status={idea.status} />

      <section className="inspector-section metadata-section">
        <div className="section-title"><h3>Inferred metadata</h3><button className="text-button" type="button">Edit</button></div>
        <dl className="metadata-list">
          <div><dt><Icon name="lightbulb" size={16} />Difficulty</dt><dd><i className={`status-dot ${idea.difficulty}`} />{labelize(idea.difficulty)}</dd></div>
          <div><dt><Icon name="clock" size={16} />Duration</dt><dd><Icon name="clock" size={15} />{idea.duration === 'quick' ? '20 minutes' : labelize(idea.duration)}</dd></div>
          <div><dt><Icon name="leaf" size={16} />Treatment</dt><dd><Icon name="leaf" size={15} />{labelize(idea.treatment)}</dd></div>
          <div><dt><Icon name="globe" size={16} />Cuisine</dt><dd><Icon name="globe" size={15} />{idea.cuisine}</dd></div>
          <div><dt><Icon name="egg" size={16} />Primary ingredient</dt><dd><Icon name="egg" size={15} />{idea.primaryIngredient}</dd></div>
          <div><dt><Icon name="video" size={16} />Content type</dt><dd><Icon name="video" size={15} />{idea.contentType === 'vlog' ? 'Vlog' : 'Video + Recipe'}</dd></div>
          <div><dt><Icon name="sparkles" size={16} />Visual potential</dt><dd><i className="status-dot easy" />{idea.visualPotential}</dd></div>
        </dl>
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Notes</h3><button className="text-button" type="button">Edit</button></div>
        <textarea className="notes-input" value={idea.notes.join('\n')} onChange={(event) => onNotesChange(event.target.value)} aria-label="Idea notes" />
      </section>

      <section className="inspector-section">
        <div className="section-title"><h3>Related ideas</h3></div>
        <RelatedIdeas allIdeas={allIdeas} ids={idea.relatedIdeaIds} onSelect={onSelect} />
        <LinkControl candidates={linkCandidates} onLink={(relatedId) => onLinkRelated(idea.id, relatedId)} />
      </section>

      <div className="inspector-actions">
        <div className="secondary-actions"><button type="button" className="text-button" onClick={() => onMerge(idea.id)}>Merge duplicate</button><button type="button" className="text-button" onClick={() => onArchive(idea.id)}><Icon name={isArchived ? 'arrow' : 'archive'} size={15} />{isArchived ? 'Restore' : 'Archive'}</button><button type="button" className="text-button delete-button" onClick={() => onDelete(idea.id)}><Icon name="trash" size={15} />Delete</button></div>
        {!isArchived && <button className="button button-primary generate-button" type="button" onClick={() => onAdvance(idea, target)}><Icon name="sparkles" size={19} />{action}</button>}
      </div>
    </aside>
  );
}

export function IdeasWorkspace({ ideas, selectedId, onSelect, onCreate, onAdvance, onArchive, onDelete, onLinkRelated, onMerge, onNotesChange, captureValue, onCaptureChange, onCaptureFocus, search, onSearchChange }) {
  const [filter, setFilter] = useState('all');
  const [difficulty, setDifficulty] = useState('all');
  const [duration, setDuration] = useState('all');
  const [treatment, setTreatment] = useState('all');
  const selectedIdea = ideas.find((idea) => idea.id === selectedId) ?? ideas[0];
  const query = search.trim().toLowerCase();
  const visibleIdeas = ideas.filter((idea) => {
    const matchesText = !query || [idea.title, idea.cue, idea.cuisine, idea.primaryIngredient, ...idea.tags].join(' ').toLowerCase().includes(query);
    const matchesStatus = filter === 'all' || idea.status === filter;
    return matchesText && matchesStatus && (difficulty === 'all' || idea.difficulty === difficulty) && (duration === 'all' || idea.duration === duration) && (treatment === 'all' || idea.treatment === treatment);
  });

  return (
    <main className="workspace ideas-workspace">
      <header className="workspace-header">
        <h1>Ideas</h1>
        <label className="search-box"><Icon name="search" size={21} /><input type="search" value={search} onChange={(event) => onSearchChange(event.target.value)} placeholder="Search ideas" aria-label="Search ideas" /><kbd>⌘ K</kbd></label>
        <button className="button button-primary header-capture" type="button" onClick={onCaptureFocus}><Icon name="plus" size={18} />Capture idea</button>
      </header>
      <div className="ideas-layout">
        <section className="ideas-main">
          <CaptureComposer captureValue={captureValue} onCaptureChange={onCaptureChange} onCapture={onCreate} onFocus={onCaptureFocus} />
          <FilterBar filter={filter} onFilterChange={setFilter} difficulty={difficulty} onDifficultyChange={setDifficulty} duration={duration} onDurationChange={setDuration} treatment={treatment} onTreatmentChange={setTreatment} />
          <section className="idea-list" aria-label="Idea backlog">
            <div className="idea-list-heading"><span>Title</span><span>Visual cue</span><span>Meta</span><span>Content type</span><span>Status</span><span>Next action</span></div>
            {visibleIdeas.map((idea) => <IdeaRow idea={idea} isSelected={idea.id === selectedIdea.id} onSelect={onSelect} onAdvance={onAdvance} key={idea.id} />)}
            {!visibleIdeas.length && <div className="empty-list"><Icon name="search" size={28} />No ideas match that set of filters.</div>}
            <footer>{visibleIdeas.length} idea{visibleIdeas.length === 1 ? '' : 's'}</footer>
          </section>
        </section>
        {selectedIdea && <IdeaInspector idea={selectedIdea} allIdeas={ideas} onAdvance={onAdvance} onArchive={onArchive} onDelete={onDelete} onLinkRelated={onLinkRelated} onMerge={onMerge} onNotesChange={onNotesChange} onSelect={onSelect} onClose={() => onSelect(null)} />}
      </div>
    </main>
  );
}
