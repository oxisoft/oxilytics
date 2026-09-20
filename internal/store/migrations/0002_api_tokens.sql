-- +goose Up
-- Read-only API tokens. Owned by the user who created them: ON DELETE CASCADE
-- means deleting a user revokes their tokens in the same transaction, so a
-- departed employee's scripts stop working without a separate cleanup step.
--
-- Only the SHA-256 of the token is stored. The plaintext is shown once at
-- creation and is unrecoverable afterwards, so a database leak does not hand
-- over working credentials.
CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at   TEXT
);

CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);
CREATE UNIQUE INDEX idx_api_tokens_hash ON api_tokens(token_hash);

-- +goose Down
DROP TABLE api_tokens;
