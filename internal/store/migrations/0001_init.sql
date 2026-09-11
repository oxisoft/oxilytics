-- +goose Up
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','viewer')),
    totp_secret   TEXT,
    totp_recovery TEXT,
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    last_login_at TEXT
);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE products (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    slug        TEXT NOT NULL UNIQUE,
    icon_url    TEXT,
    description TEXT,
    archived    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE apps (
    id                   INTEGER PRIMARY KEY,
    store                TEXT NOT NULL CHECK (store IN ('appstore','googleplay')),
    store_app_id         TEXT NOT NULL,
    name                 TEXT NOT NULL,
    bundle_id            TEXT,
    platform             TEXT NOT NULL,
    icon_url             TEXT,
    product_id           INTEGER REFERENCES products(id) ON DELETE SET NULL,
    suggested_product_id INTEGER REFERENCES products(id) ON DELETE SET NULL,
    rating_avg           REAL,
    rating_count         INTEGER,
    rating_updated_at    TEXT,
    ignored_at           TEXT,
    ignored_by           INTEGER REFERENCES users(id) ON DELETE SET NULL,
    ignored_reason       TEXT,
    first_seen_at        TEXT NOT NULL,
    last_synced_at       TEXT,
    UNIQUE (store, store_app_id)
);
CREATE UNIQUE INDEX apps_product_platform ON apps(product_id, platform) WHERE product_id IS NOT NULL;
CREATE INDEX apps_ignored ON apps(ignored_at);
CREATE INDEX apps_product ON apps(product_id);

CREATE TABLE metric_days (
    app_id            INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    day               TEXT NOT NULL,
    country           TEXT NOT NULL,
    downloads         INTEGER NOT NULL DEFAULT 0,
    redownloads       INTEGER NOT NULL DEFAULT 0,
    updates           INTEGER NOT NULL DEFAULT 0,
    uninstalls        INTEGER NOT NULL DEFAULT 0,
    active_devices    INTEGER,
    crashes           INTEGER NOT NULL DEFAULT 0,
    anrs              INTEGER NOT NULL DEFAULT 0,
    source_updated_at TEXT,
    PRIMARY KEY (app_id, day, country)
);
CREATE INDEX metric_days_day ON metric_days(day);

CREATE TABLE reviews (
    id                   INTEGER PRIMARY KEY,
    app_id               INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    store_review_id      TEXT NOT NULL,
    rating               INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    title                TEXT,
    body                 TEXT,
    author               TEXT,
    country              TEXT,
    language             TEXT,
    app_version          TEXT,
    device               TEXT,
    created_at           TEXT NOT NULL,
    edited_at            TEXT,
    developer_reply      TEXT,
    developer_replied_at TEXT,
    fetched_at           TEXT NOT NULL,
    UNIQUE (app_id, store_review_id)
);
CREATE INDEX reviews_app_created ON reviews(app_id, created_at DESC);
CREATE INDEX reviews_created ON reviews(created_at DESC);
CREATE INDEX reviews_rating ON reviews(rating);

CREATE VIRTUAL TABLE reviews_fts USING fts5(title, body, content='reviews', content_rowid='id');
-- +goose StatementBegin
CREATE TRIGGER reviews_ai AFTER INSERT ON reviews BEGIN
  INSERT INTO reviews_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER reviews_ad AFTER DELETE ON reviews BEGIN
  INSERT INTO reviews_fts(reviews_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER reviews_au AFTER UPDATE ON reviews BEGIN
  INSERT INTO reviews_fts(reviews_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
  INSERT INTO reviews_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
-- +goose StatementEnd

CREATE TABLE sync_runs (
    id           INTEGER PRIMARY KEY,
    store        TEXT NOT NULL,
    mode         TEXT NOT NULL CHECK (mode IN ('full','delta')),
    trigger      TEXT NOT NULL CHECK (trigger IN ('manual','schedule')),
    requested_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    status       TEXT NOT NULL,
    started_at   TEXT,
    finished_at  TEXT,
    range_from   TEXT,
    range_to     TEXT,
    apps_total   INTEGER NOT NULL DEFAULT 0,
    apps_done    INTEGER NOT NULL DEFAULT 0,
    rows_metrics INTEGER NOT NULL DEFAULT 0,
    rows_reviews INTEGER NOT NULL DEFAULT 0,
    error        TEXT,
    stats        TEXT,
    created_at   TEXT NOT NULL
);
CREATE INDEX sync_runs_store_created ON sync_runs(store, created_at DESC);

CREATE TABLE sync_run_logs (
    id      INTEGER PRIMARY KEY,
    run_id  INTEGER NOT NULL REFERENCES sync_runs(id) ON DELETE CASCADE,
    ts      TEXT NOT NULL,
    level   TEXT NOT NULL,
    app_id  INTEGER,
    message TEXT NOT NULL
);
CREATE INDEX sync_run_logs_run ON sync_run_logs(run_id, id);

CREATE TABLE sync_checkpoints (
    store      TEXT NOT NULL,
    source     TEXT NOT NULL,
    app_id     INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    cursor     TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (store, source, app_id)
);

CREATE TABLE ingested_objects (
    store       TEXT NOT NULL,
    object_name TEXT NOT NULL,
    generation  TEXT NOT NULL,
    md5         TEXT,
    ingested_at TEXT NOT NULL,
    PRIMARY KEY (store, object_name)
);

INSERT INTO settings(key, value, updated_at) VALUES
  ('sync.schedule.enabled', 'true', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('sync.schedule.time', '06:30', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('sync.schedule.stores', 'appstore,googleplay', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('sync.delta.overlap_days', '3', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('metrics.retention_days', '0', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('ui.default_range_days', '30', strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  ('products.suggest', 'true', strftime('%Y-%m-%dT%H:%M:%SZ','now'));

-- +goose Down
DROP TABLE ingested_objects;
DROP TABLE sync_checkpoints;
DROP TABLE sync_run_logs;
DROP TABLE sync_runs;
DROP TRIGGER reviews_au;
DROP TRIGGER reviews_ad;
DROP TRIGGER reviews_ai;
DROP TABLE reviews_fts;
DROP TABLE reviews;
DROP TABLE metric_days;
DROP TABLE apps;
DROP TABLE products;
DROP TABLE settings;
DROP TABLE users;
