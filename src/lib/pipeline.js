const DEFAULT_METADATA = Object.freeze({
  difficulty: 'moderate',
  duration: 'average',
  treatment: 'non_elevated',
  contentType: 'recipe',
  cuisine: 'Open to interpretation',
  primaryIngredient: 'To be decided',
  equipment: 'Standard stovetop setup',
  visualPotential: 'medium',
  seasonality: 'all_year',
  productionEffort: 'average',
  tags: []
});

const STAGE_TRANSITIONS = Object.freeze({
  idea: Object.freeze({ brief: 'brief_ready' }),
  brief_ready: Object.freeze({ recipe: 'recipe_review' }),
  recipe_review: Object.freeze({ script: 'script_ready' }),
  script_ready: Object.freeze({ production: 'production_ready' })
});

const hasMatch = (text, expression) => expression.test(text);

function inferCuisine(text) {
  if (hasMatch(text, /caesar|anchov/i)) return 'Italian-inspired';
  if (hasMatch(text, /shawarma|tahini|halloumi/i)) return 'Middle Eastern';
  if (hasMatch(text, /chili crisp|scallion|soy sauce|sesame/i)) return 'Chinese-inspired';
  if (hasMatch(text, /pot pie|cheddar|ranch/i)) return 'American';
  return DEFAULT_METADATA.cuisine;
}

function inferPrimaryIngredient(text) {
  const ingredients = [
    ['Chicken', /chicken/i],
    ['Eggs', /egg/i],
    ['Cabbage', /cabbage/i],
    ['Halloumi', /halloumi/i],
    ['Zucchini', /zucchini/i],
    ['Beans', /bean/i],
    ['Mushrooms', /mushroom/i]
  ];
  const match = ingredients.find(([, expression]) => hasMatch(text, expression));
  return match?.[0] ?? DEFAULT_METADATA.primaryIngredient;
}

function inferTags(text) {
  const candidates = [
    ['quick', /quick|15 minute|20 minute|weeknight/i],
    ['weeknight', /quick|weeknight|sheet.pan/i],
    ['charred', /char|grill|broil/i],
    ['comfort food', /pot pie|cozy|comfort/i],
    ['vegetarian', /cabbage|zucchini|halloumi|bean/i],
    ['video', /video|reel|tiktok|show/i]
  ];
  return candidates.filter(([, expression]) => hasMatch(text, expression)).map(([tag]) => tag);
}

export function inferIdeaMetadata(value) {
  if (typeof value !== 'string') throw new TypeError('Idea text must be a string.');

  const text = value.trim();
  if (!text) return { ...DEFAULT_METADATA };

  const duration = hasMatch(text, /quick|15 minute|20 minute|weeknight/i)
    ? 'quick'
    : hasMatch(text, /multi.day|ferment|overnight|braise/i)
      ? 'multi_day'
      : 'average';
  const difficulty = hasMatch(text, /quick|sheet.pan|simple|easy/i)
    ? 'easy'
    : hasMatch(text, /insane|laminat|from scratch|project/i)
      ? 'insane'
      : 'moderate';

  return {
    ...DEFAULT_METADATA,
    difficulty,
    duration,
    treatment: hasMatch(text, /elevated|fancy|restaurant|crispy|charred/i)
      ? 'elevated'
      : 'non_elevated',
    contentType: hasMatch(text, /vlog|day in|behind the scenes/i) ? 'vlog' : 'recipe',
    cuisine: inferCuisine(text),
    primaryIngredient: inferPrimaryIngredient(text),
    equipment: hasMatch(text, /sheet.pan/i) ? 'Sheet pan' : DEFAULT_METADATA.equipment,
    visualPotential: hasMatch(text, /crispy|charred|sizzle|pull|sauce/i) ? 'high' : 'medium',
    seasonality: hasMatch(text, /august|summer|zucchini/i) ? 'summer' : 'all_year',
    productionEffort: duration === 'quick' ? 'light' : 'average',
    tags: inferTags(text)
  };
}

export function advanceIdea(idea, target) {
  if (!idea || typeof idea !== 'object') throw new TypeError('An idea is required.');
  if (typeof target !== 'string') throw new TypeError('A target stage is required.');

  const nextStatus = STAGE_TRANSITIONS[idea.status]?.[target];
  if (!nextStatus) throw new Error(`The ${target} stage is not available from ${idea.status}.`);

  return {
    ...idea,
    status: nextStatus,
    updatedAt: 'Just now'
  };
}

export function mergeIdeas(primary, duplicate) {
  if (!primary?.id || !duplicate?.id) throw new TypeError('Both ideas need stable identifiers.');

  const notes = [...(primary.notes ?? []), ...(duplicate.notes ?? [])];
  const relatedIdeaIds = [...new Set([...(primary.relatedIdeaIds ?? []), duplicate.id])];

  return [
    {
      ...primary,
      notes,
      relatedIdeaIds,
      updatedAt: 'Just now'
    },
    duplicate.id
  ];
}

export function linkIdeas(primary, related) {
  if (!primary?.id || !related?.id) throw new TypeError('Both ideas need stable identifiers.');
  if (primary.id === related.id) throw new Error('An idea cannot be related to itself.');

  return [
    {
      ...primary,
      relatedIdeaIds: [...new Set([...(primary.relatedIdeaIds ?? []), related.id])],
      updatedAt: 'Just now'
    },
    {
      ...related,
      relatedIdeaIds: [...new Set([...(related.relatedIdeaIds ?? []), primary.id])],
      updatedAt: 'Just now'
    }
  ];
}

export function removeIdea(ideas, id) {
  if (!Array.isArray(ideas)) throw new TypeError('Ideas must be an array.');
  if (typeof id !== 'string' || !id) throw new TypeError('An idea identifier is required.');

  return ideas
    .filter((idea) => idea.id !== id)
    .map((idea) => ({
      ...idea,
      relatedIdeaIds: (idea.relatedIdeaIds ?? []).filter((relatedId) => relatedId !== id)
    }));
}
