// A stable mark per idea: a letter from the primary ingredient over a hue
// hashed from the same value. Deterministic, so an idea keeps its identity in
// the list across reloads.

function hashHue(input) {
  let hash = 0;
  for (let i = 0; i < input.length; i += 1) {
    hash = (hash * 31 + input.charCodeAt(i)) % 360;
  }
  return hash;
}

export function thumbnailProps(idea) {
  const seed = idea.metadata?.primary_ingredient || idea.title || idea.id;
  const letter = seed.trim().charAt(0).toUpperCase() || '·';
  return {
    letter,
    style: { '--thumb-hue': hashHue(seed) },
    label: idea.metadata?.primary_ingredient
      ? `Primary ingredient: ${idea.metadata.primary_ingredient}`
      : 'No primary ingredient inferred yet',
  };
}
