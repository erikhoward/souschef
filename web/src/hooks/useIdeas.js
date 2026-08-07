import { useCallback, useEffect, useRef, useState } from 'react';

import * as api from '../lib/api.js';

/**
 * Owns the idea list: initial fetch, mutations, and live updates over SSE.
 *
 * This replaces the fifteen-prop drill through App.jsx. Components read what
 * they need from the returned object instead of having it threaded down.
 */
export function useIdeas(filters) {
  const [ideas, setIdeas] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [connected, setConnected] = useState(false);

  // Keep the latest filters in a ref so the SSE effect does not resubscribe
  // on every keystroke in the search box.
  const filtersRef = useRef(filters);
  filtersRef.current = filters;

  const refresh = useCallback(async () => {
    try {
      setError(null);
      const rows = await api.listIdeas(filtersRef.current);
      setIdeas(rows);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setLoading(true);
    refresh();
  }, [refresh, filters.q, filters.stage, filters.difficulty,
      filters.duration, filters.treatment, filters.archived,
      filters.sort, filters.order]);

  // Live updates. EventSource reconnects on its own using the server's
  // `retry` hint, so there is no manual backoff here — but a reconnect can
  // miss events, so we refetch when the connection is re-established.
  useEffect(() => {
    const source = new EventSource('/events');
    let hasConnected = false;

    source.onopen = () => {
      setConnected(true);
      if (hasConnected) refresh(); // resynchronise after a gap
      hasConnected = true;
    };

    source.onerror = () => setConnected(false);

    source.onmessage = (event) => {
      let payload;
      try {
        payload = JSON.parse(event.data);
      } catch {
        return;
      }

      setIdeas((current) => {
        if (payload.type === 'idea.deleted') {
          return current.filter((idea) => idea.id !== payload.id);
        }
        if (!payload.idea) return current;

        const index = current.findIndex((idea) => idea.id === payload.idea.id);
        if (index === -1) {
          return payload.type === 'idea.created' ? [payload.idea, ...current] : current;
        }
        const next = current.slice();
        next[index] = payload.idea;
        return next;
      });
    };

    return () => source.close();
  }, [refresh]);

  const create = useCallback(async (rawText) => {
    // The server broadcasts idea.created, but inserting here too means the
    // row appears even if the SSE stream is momentarily down.
    const idea = await api.createIdea(rawText);
    setIdeas((current) =>
      current.some((i) => i.id === idea.id) ? current : [idea, ...current]);
    return idea;
  }, []);

  const replace = useCallback((updated) => {
    setIdeas((current) => current.map((i) => (i.id === updated.id ? updated : i)));
    return updated;
  }, []);

  const patch    = useCallback(async (id, p)     => replace(await api.patchIdea(id, p)), [replace]);
  // Archiving/restoring can change whether the idea belongs in the
  // *currently filtered* view (the default view excludes archived ideas,
  // the Archived tab excludes everything else). A plain replace() would
  // leave a stale entry visible until some unrelated event happened to
  // trigger a refetch, so these two re-derive the filtered list from the
  // server instead of patching in place.
  const archive  = useCallback(async (id) => {
    const updated = await api.archiveIdea(id);
    await refresh();
    return updated;
  }, [refresh]);
  const restore  = useCallback(async (id) => {
    const updated = await api.restoreIdea(id);
    await refresh();
    return updated;
  }, [refresh]);
  const reenrich = useCallback(async (id)        => replace(await api.reenrichIdea(id)), [replace]);
  const link     = useCallback(async (id, other) => replace(await api.linkIdeas(id, other)), [replace]);
  const addNote  = useCallback(async (id, body)  => replace(await api.addNote(id, body)), [replace]);
  const merge    = useCallback(async (id, dup)   => {
    const merged = await api.mergeIdeas(id, dup);
    setIdeas((current) =>
      current.filter((i) => i.id !== dup).map((i) => (i.id === merged.id ? merged : i)));
    return merged;
  }, []);

  const remove = useCallback(async (id) => {
    await api.deleteIdea(id);
    setIdeas((current) => current.filter((i) => i.id !== id));
  }, []);

  return {
    ideas, loading, error, connected, refresh,
    create, patch, archive, restore, remove, reenrich, merge, link, addNote,
  };
}
