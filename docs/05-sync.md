# 05 — Sync engine (`internal/sync`)

## Modes

| | full | delta |
|-|------|-------|
| When | first run, or admin chooses "Full" (e.g. after a bug fix in parsing) | daily schedule, "Sync now" default |
| Range | from `apps.first_seen_at` / earliest store data to yesterday | from `checkpoint − overlap_days` to yesterday |
| Apple | creates `ONE_TIME_SNAPSHOT` report request, waits for instances, downloads every daily instance; reviews: walks all pages | uses `ONGOING` request; downloads instances newer than checkpoint; reviews: pages until `createdDate < checkpoint` |
| Google | lists every object under the 4 prefixes, downloads all, upserts | downloads objects whose generation changed + current & previous month; `reviews.list` for last 7 days |
| Writes | upsert (`INSERT … ON CONFLICT DO UPDATE`), never delete | same |

A full run **does not wipe** existing data; it re-upserts everything. To wipe, an admin
uses the "Reset store data" action (deletes `metric_days`, `reviews`, checkpoints for that store) and then runs full.

## Lifecycle

```
queued ──▶ running ──▶ succeeded
                  ├──▶ failed        (unrecoverable error; partial data kept, checkpoints not advanced past the failure)
                  ├──▶ cancelled     (admin pressed Cancel; ctx cancelled, current app finishes its upsert)
                  └──▶ interrupted   (process restarted; set on startup for any run still `running`)
```

- At most **one run per store** at a time; a second request returns `409 sync_already_running`.
- Both stores can run in parallel (independent goroutines, independent rate limits).
- Each run gets a `context.Context` with cancel stored in the engine's `running` map.
- Progress: `apps_done/apps_total`, counters, and the last log line are updated in
  `sync_runs` every 2 s and read by the UI through polling (`GET /api/sync/runs/{id}`, 3 s interval). No websockets in v1.

## Steps of a run

1. Load enabled apps for the store (full mode also refreshes the catalogue first and inserts new apps).
2. For each app (sequentially — the stores rate-limit per account, not per app):
   1. metrics (downloads/installs/uninstalls)
   2. crashes
   3. ratings
   4. reviews
   Each step: fetch → parse → upsert in one transaction per (app, source, month) → advance checkpoint → log.
3. Refresh totals (`rating_total_*`, `apps.last_synced_at`).
4. Mark run finished, write `stats` JSON.

A step failure for one app is logged and the run continues; the run ends `failed` only
if **every** app failed or the failure is auth/config (no point continuing).

## Scheduling

`robfig/cron v3` with the `OXI_TZ` location. One entry, rebuilt whenever
`sync.schedule.*` changes (settings service emits an event the scheduler listens to).
Spec: `M H * * *` from `sync.schedule.time`. On tick: for each store in
`sync.schedule.stores` that is configured and not running → enqueue `delta` run with
`trigger=schedule`. Missed ticks (process down) are **not** replayed; the next tick or a
manual run catches up because delta ranges are computed from checkpoints, not from "yesterday".

A second cron entry at `03:15` runs backup + retention pruning.

## Idempotency & correctness

- All upserts are keyed on natural keys; running the same range twice yields identical data.
- Store restatements are absorbed by `overlap_days`.
- Checkpoints advance only after the transaction commits.
- Parsers are pure functions `[]byte → []MetricDay` with table-driven tests against
  real (anonymised) store files in `testdata/`.

## Observability

- `sync_run_logs` rows at `info` for each step and `error` for failures.
- `GET /api/sync/status` summarises: per store → configured?, running run, last run, next scheduled time.
- Metrics counters in `stats` JSON: API calls, bytes downloaded, rows upserted, duration per step.
