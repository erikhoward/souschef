CREATE TABLE ideas (
    id                 TEXT PRIMARY KEY,
    title              TEXT NOT NULL,
    raw_text           TEXT NOT NULL,
    source             TEXT NOT NULL,
    source_ref         TEXT,
    stage              TEXT NOT NULL DEFAULT 'idea',
    archived_at        DATETIME,
    merged_into_id     TEXT REFERENCES ideas(id) ON DELETE SET NULL,

    difficulty         TEXT,
    duration_class     TEXT,
    treatment          TEXT,
    content_type       TEXT,
    cuisine            TEXT,
    primary_ingredient TEXT,
    equipment          TEXT,
    visual_potential   TEXT,
    seasonality        TEXT,
    production_effort  TEXT,
    field_overrides    TEXT NOT NULL DEFAULT '[]',

    enrichment_status  TEXT NOT NULL DEFAULT 'pending',
    enrichment_error   TEXT,
    enrichment_model   TEXT,
    enriched_at        DATETIME,

    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL
);

CREATE INDEX idx_ideas_stage      ON ideas(stage);
CREATE INDEX idx_ideas_archived   ON ideas(archived_at);
CREATE INDEX idx_ideas_created    ON ideas(created_at DESC);
CREATE INDEX idx_ideas_enrichment ON ideas(enrichment_status);

CREATE TABLE notes (
    id         TEXT PRIMARY KEY,
    idea_id    TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    body       TEXT NOT NULL,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_notes_idea ON notes(idea_id);

CREATE TABLE tags (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE idea_tags (
    idea_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    tag_id  TEXT NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (idea_id, tag_id)
);

-- Links are symmetric and stored once. The CHECK enforces canonical ordering
-- so (a,b) and (b,a) cannot both exist, and a self-link is impossible.
CREATE TABLE idea_links (
    idea_a_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    idea_b_id TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    PRIMARY KEY (idea_a_id, idea_b_id),
    CHECK (idea_a_id < idea_b_id)
);

CREATE TABLE transcripts (
    id          TEXT PRIMARY KEY,
    idea_id     TEXT NOT NULL REFERENCES ideas(id) ON DELETE CASCADE,
    audio_path  TEXT NOT NULL,
    text        TEXT NOT NULL,
    duration_ms INTEGER,
    created_at  DATETIME NOT NULL
);

-- Contentless-adjacent FTS: we store the searchable text directly so tags,
-- which live in a join table, can be denormalised into the index.
CREATE VIRTUAL TABLE ideas_fts USING fts5(
    id UNINDEXED,
    title,
    raw_text,
    cuisine,
    primary_ingredient,
    tags,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER ideas_fts_insert AFTER INSERT ON ideas BEGIN
    INSERT INTO ideas_fts (id, title, raw_text, cuisine, primary_ingredient, tags)
    VALUES (new.id, new.title, new.raw_text,
            coalesce(new.cuisine, ''), coalesce(new.primary_ingredient, ''), '');
END;

CREATE TRIGGER ideas_fts_update AFTER UPDATE ON ideas BEGIN
    UPDATE ideas_fts
       SET title              = new.title,
           raw_text           = new.raw_text,
           cuisine            = coalesce(new.cuisine, ''),
           primary_ingredient = coalesce(new.primary_ingredient, '')
     WHERE id = new.id;
END;

CREATE TRIGGER ideas_fts_delete AFTER DELETE ON ideas BEGIN
    DELETE FROM ideas_fts WHERE id = old.id;
END;
