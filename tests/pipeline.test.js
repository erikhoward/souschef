import { describe, expect, test } from 'bun:test';

import { advanceIdea, inferIdeaMetadata, linkIdeas, mergeIdeas, removeIdea } from '../src/lib/pipeline.js';

describe('inferIdeaMetadata', () => {
  test('derives useful defaults from natural-language capture', () => {
    expect(
      inferIdeaMetadata('Quick charred cabbage Caesar with anchovy breadcrumbs, maybe a little fancy')
    ).toMatchObject({
      difficulty: 'easy',
      duration: 'quick',
      treatment: 'elevated',
      contentType: 'recipe',
      cuisine: 'Italian-inspired',
      primaryIngredient: 'Cabbage',
      tags: expect.arrayContaining(['charred', 'weeknight'])
    });
  });

  test('keeps the default workflow friendly when signals are absent', () => {
    expect(inferIdeaMetadata('Something good with beans')).toMatchObject({
      difficulty: 'moderate',
      duration: 'average',
      treatment: 'non_elevated',
      contentType: 'recipe'
    });
  });
});

describe('advanceIdea', () => {
  test('promotes an idea through the creation pipeline in order', () => {
    const idea = { status: 'idea', title: 'Crispy chili eggs' };

    expect(advanceIdea(idea, 'brief')).toMatchObject({ status: 'brief_ready' });
    expect(advanceIdea({ ...idea, status: 'brief_ready' }, 'recipe')).toMatchObject({
      status: 'recipe_review'
    });
    expect(advanceIdea({ ...idea, status: 'recipe_review' }, 'script')).toMatchObject({
      status: 'script_ready'
    });
  });

  test('rejects jumping over a required approval stage', () => {
    expect(() => advanceIdea({ status: 'idea' }, 'script')).toThrow('not available');
  });
});

describe('mergeIdeas', () => {
  test('retains the primary record and carries over source notes', () => {
    const [merged, removedId] = mergeIdeas(
      { id: 'primary', notes: ['original'] },
      { id: 'duplicate', notes: ['add chili crisp'] }
    );

    expect(removedId).toBe('duplicate');
    expect(merged.notes).toEqual(['original', 'add chili crisp']);
    expect(merged.relatedIdeaIds).toContain('duplicate');
  });
});

describe('linkIdeas', () => {
  test('adds a reciprocal relationship without duplicates', () => {
    const [primary, related] = linkIdeas(
      { id: 'primary', relatedIdeaIds: [] },
      { id: 'related', relatedIdeaIds: ['other'] }
    );

    expect(primary.relatedIdeaIds).toEqual(['related']);
    expect(related.relatedIdeaIds).toEqual(['other', 'primary']);
  });
});

describe('removeIdea', () => {
  test('removes only the requested idea and clears stale links', () => {
    const ideas = [
      { id: 'first', relatedIdeaIds: ['second'] },
      { id: 'second', relatedIdeaIds: [] }
    ];

    expect(removeIdea(ideas, 'second')).toEqual([{ id: 'first', relatedIdeaIds: [] }]);
  });
});
