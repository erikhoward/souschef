// Thin wrappers over the REST surface. Every function throws an Error whose
// message is the server's, so callers can surface it verbatim rather than
// inventing their own copy.

async function request(path, { method = 'GET', body } = {}) {
  const response = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });

  if (response.status === 204) return null;

  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`Unexpected response from ${path}: ${text.slice(0, 200)}`);
    }
  }

  if (!response.ok) {
    throw new Error(payload?.error ?? `${response.status} ${response.statusText}`);
  }
  return payload;
}

export function listIdeas(params = {}) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== '' && value != null && value !== 'all') query.set(key, value);
  }
  // `archived` is meaningful as the literal string "all", so set it explicitly.
  if (params.archived) query.set('archived', params.archived);

  const qs = query.toString();
  return request(`/api/ideas${qs ? `?${qs}` : ''}`);
}

// getIdea is the fallback behind a deep link. The list is the fast path, but
// it cannot answer for an idea that is archived, merged into another, or past
// the 500-row limit — and /ideas/<id> has to resolve for all three, since
// that is what Telegram's [Open] button points at.
export const getIdea = (id) => request(`/api/ideas/${id}`);

export const createIdea = (rawText) =>
  request('/api/ideas', { method: 'POST', body: { raw_text: rawText, source: 'web' } });

export const patchIdea = (id, patch) =>
  request(`/api/ideas/${id}`, { method: 'PATCH', body: patch });

export const archiveIdea = (id) => request(`/api/ideas/${id}/archive`, { method: 'POST' });
export const restoreIdea = (id) => request(`/api/ideas/${id}/restore`, { method: 'POST' });
export const reenrichIdea = (id) => request(`/api/ideas/${id}/reenrich`, { method: 'POST' });
export const deleteIdea = (id) => request(`/api/ideas/${id}`, { method: 'DELETE' });

export const addNote = (id, body) =>
  request(`/api/ideas/${id}/notes`, { method: 'POST', body: { body } });

export const linkIdeas = (id, otherId) =>
  request(`/api/ideas/${id}/links`, { method: 'POST', body: { other_id: otherId } });

export const mergeIdeas = (id, duplicateId) =>
  request(`/api/ideas/${id}/merge`, { method: 'POST', body: { duplicate_id: duplicateId } });
