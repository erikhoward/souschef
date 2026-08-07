import { useEffect, useRef, useState } from 'react';
import { Route, Routes, useNavigate, useParams } from 'react-router';

import { IdeasWorkspace } from './components/IdeasWorkspace.jsx';
import { Sidebar } from './components/Sidebar.jsx';
import { useIdeas } from './hooks/useIdeas.js';

const emptyFilters = {
  q: '', stage: '', difficulty: '', duration: '',
  treatment: '', archived: '', sort: 'created_at', order: 'desc',
};

function Workspace() {
  const { id: selectedId } = useParams();
  const navigate = useNavigate();

  const [filters, setFilters] = useState(emptyFilters);
  const [captureValue, setCaptureValue] = useState('');
  const [toast, setToast] = useState('');
  const toastTimer = useRef(null);

  const store = useIdeas(filters);

  const announce = (message) => {
    window.clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = window.setTimeout(() => setToast(''), 3200);
  };

  useEffect(() => () => window.clearTimeout(toastTimer.current), []);

  const guard = (fn, success) => async (...args) => {
    try {
      const result = await fn(...args);
      if (success) announce(success);
      return result;
    } catch (err) {
      announce(err.message);
      return null;
    }
  };

  const handleCreate = async (event) => {
    event.preventDefault();
    const text = captureValue.trim();
    if (!text) return;
    const idea = await guard(store.create)(text);
    if (idea) {
      setCaptureValue('');
      navigate(`/ideas/${idea.id}`);
      announce('Saved. Reading it now…');
    }
  };

  const focusCapture = () => {
    window.setTimeout(() => document.querySelector('.capture-control textarea')?.focus(), 0);
  };

  return (
    <div className="app-shell">
      <Sidebar activeView="ideas" onNavigate={() => {}} onCapture={focusCapture} />
      <IdeasWorkspace
        store={store}
        filters={filters}
        onFiltersChange={(patch) => setFilters((current) => ({ ...current, ...patch }))}
        selectedId={selectedId}
        onSelect={(id) => navigate(id ? `/ideas/${id}` : '/ideas')}
        captureValue={captureValue}
        onCaptureChange={setCaptureValue}
        onCreate={handleCreate}
        onCaptureFocus={focusCapture}
        announce={announce}
        guard={guard}
      />
      {toast && <div className="toast" role="status">{toast}</div>}
      {!store.connected && (
        <div className="connection-warning" role="status">
          Live updates disconnected — reconnecting…
        </div>
      )}
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Workspace />} />
      <Route path="/ideas" element={<Workspace />} />
      <Route path="/ideas/:id" element={<Workspace />} />
    </Routes>
  );
}
