import { useRef, useState } from 'react';

import { IdeasWorkspace } from './components/IdeasWorkspace.jsx';
import { RecipeWorkspace } from './components/RecipeWorkspace.jsx';
import { Sidebar } from './components/Sidebar.jsx';
import { seedIdeas } from './data/ideas.js';
import { advanceIdea, inferIdeaMetadata, linkIdeas, mergeIdeas, removeIdea } from './lib/pipeline.js';

const createNewIdea = (text) => {
  const metadata = inferIdeaMetadata(text);
  return {
    id: `idea-${crypto.randomUUID()}`,
    title: text.trim(),
    cue: 'Freshly captured. Shape it when you have the bandwidth.',
    ...metadata,
    status: 'idea',
    updatedAt: 'Just now',
    notes: [],
    relatedIdeaIds: [],
    thumbnailPosition: '100%'
  };
};

export default function App() {
  const [ideas, setIdeas] = useState(() => seedIdeas.map((idea) => ({ ...idea, notes: [...idea.notes], relatedIdeaIds: [...idea.relatedIdeaIds], tags: [...idea.tags] })));
  const [activeView, setActiveView] = useState('ideas');
  const [selectedId, setSelectedId] = useState('crispy-chili-eggs');
  const [captureValue, setCaptureValue] = useState('');
  const [search, setSearch] = useState('');
  const [testStatus, setTestStatus] = useState('untested');
  const [toast, setToast] = useState('');
  const toastTimerRef = useRef(null);

  const selectedIdea = ideas.find((idea) => idea.id === selectedId);

  const announce = (message) => {
    window.clearTimeout(toastTimerRef.current);
    setToast(message);
    toastTimerRef.current = window.setTimeout(() => setToast(''), 2600);
  };

  const focusCapture = () => {
    setActiveView('ideas');
    window.setTimeout(() => document.querySelector('.capture-control textarea')?.focus(), 0);
  };

  const updateIdea = (updatedIdea) => setIdeas((current) => current.map((idea) => idea.id === updatedIdea.id ? updatedIdea : idea));

  const handleCreate = (event) => {
    event.preventDefault();
    if (!captureValue.trim()) return;
    const idea = createNewIdea(captureValue);
    setIdeas((current) => [idea, ...current]);
    setSelectedId(idea.id);
    setCaptureValue('');
    announce('Idea saved. Clean counter, clear head.');
  };

  const handleAdvance = (idea, target) => {
    if (target === 'idea') {
      setSelectedId(idea.id);
      return;
    }
    try {
      const updated = advanceIdea(idea, target);
      updateIdea(updated);
      setSelectedId(idea.id);
      if (target === 'recipe') setActiveView('recipes');
      announce(target === 'brief' ? 'Brief generated and ready for your review.' : `${updated.title} moved forward.`);
    } catch (error) {
      announce(error instanceof Error ? error.message : 'That stage is not available yet.');
    }
  };

  const handleArchive = (id) => {
    setIdeas((current) => current.map((idea) => idea.id === id ? { ...idea, status: idea.status === 'archived' ? 'idea' : 'archived', updatedAt: 'Just now' } : idea));
    announce('Idea status updated.');
  };

  const handleMerge = (primaryId) => {
    const duplicate = ideas.find((idea) => idea.id !== primaryId && idea.status !== 'archived');
    const primary = ideas.find((idea) => idea.id === primaryId);
    if (!primary || !duplicate) return;
    const [merged, removedId] = mergeIdeas(primary, duplicate);
    setIdeas((current) => current.filter((idea) => idea.id !== removedId).map((idea) => idea.id === primaryId ? merged : idea));
    announce(`Merged “${duplicate.title}” into this idea.`);
  };

  const handleLinkRelated = (primaryId, relatedId) => {
    const primary = ideas.find((idea) => idea.id === primaryId);
    const related = ideas.find((idea) => idea.id === relatedId);
    if (!primary || !related) return;
    const [updatedPrimary, updatedRelated] = linkIdeas(primary, related);
    setIdeas((current) => current.map((idea) => idea.id === primaryId ? updatedPrimary : idea.id === relatedId ? updatedRelated : idea));
    announce(`Linked “${related.title}” as a related idea.`);
  };

  const handleDelete = (id) => {
    const idea = ideas.find((current) => current.id === id);
    if (!idea || !window.confirm(`Delete “${idea.title}”? This cannot be undone.`)) return;
    const replacementId = ideas.find((current) => current.id !== id)?.id ?? null;
    setIdeas((current) => removeIdea(current, id));
    if (selectedId === id) setSelectedId(replacementId);
    announce('Idea deleted.');
  };

  const handleNotesChange = (value) => {
    if (!selectedIdea) return;
    updateIdea({ ...selectedIdea, notes: value.split('\n').map((note) => note.trim()).filter(Boolean), updatedAt: 'Just now' });
  };

  const handleRecipeApproval = () => {
    if (selectedIdea?.status === 'recipe_review') handleAdvance(selectedIdea, 'script');
    announce('Recipe approved. The script stage is ready when you are.');
  };

  const content = activeView === 'recipes'
    ? <RecipeWorkspace testStatus={testStatus} onTestStatusChange={setTestStatus} onApprove={handleRecipeApproval} onGenerateScript={() => selectedIdea && handleAdvance(selectedIdea, 'script')} />
    : <IdeasWorkspace ideas={ideas} selectedId={selectedId} onSelect={setSelectedId} onCreate={handleCreate} onAdvance={handleAdvance} onArchive={handleArchive} onDelete={handleDelete} onLinkRelated={handleLinkRelated} onMerge={handleMerge} onNotesChange={handleNotesChange} captureValue={captureValue} onCaptureChange={setCaptureValue} onCaptureFocus={focusCapture} search={search} onSearchChange={setSearch} />;

  return (
    <div className="app-shell">
      <Sidebar activeView={activeView} onNavigate={setActiveView} onCapture={focusCapture} />
      {content}
      {toast && <div className="toast" role="status">{toast}</div>}
    </div>
  );
}
