CREATE TABLE IF NOT EXISTS characters (
    id         UUID         NOT NULL,
    name       VARCHAR(120) NOT NULL,
    attributes JSONB        NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ  NOT NULL,
    updated_at TIMESTAMPTZ  NOT NULL,
    PRIMARY KEY (id)
);
