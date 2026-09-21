-- +goose Up
-- Opt-in capability for API tokens.
--
-- Tokens are read-only by default and that stays the default: this column is
-- NOT NULL DEFAULT 0, so every existing token keeps exactly the access it had,
-- and a token only gains the ability to start a sync if someone deliberately
-- ticked the box when creating it.
--
-- The capability is fixed at creation and never editable afterwards. A token's
-- powers are therefore whatever they were when it was issued, which makes a
-- leaked token's blast radius knowable from its creation record alone.
--
-- This grants nothing beyond triggering a sync that is already configured in
-- the UI. It cannot change credentials, schedules, products or users, and the
-- route still requires the owner's own run_sync permission, so a viewer's
-- token cannot start a sync however it was created.
ALTER TABLE api_tokens ADD COLUMN can_run_sync INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN can_run_sync;
