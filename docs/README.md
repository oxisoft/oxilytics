# Oxilytics — project documentation

Oxilytics is OxiSoft's self-hosted analytics dashboard for its own apps. It pulls
downloads, ratings, reviews and crash counts from **App Store Connect** and
**Google Play**, stores them in one SQLite file and shows them in one web UI.

One Go binary · one Docker image · one SQLite file · one git repository.

| Doc | Content |
|-----|---------|
| [01-overview.md](01-overview.md) | Goals, scope of v1, non-goals, glossary |
| [02-architecture.md](02-architecture.md) | Process layout, layers, repository layout, config, auth, permissions |
| [03-data-model.md](03-data-model.md) | SQLite schema, migrations, retention |
| [04-store-integrations.md](04-store-integrations.md) | App Store Connect and Google Play: auth, endpoints, files, quirks |
| [05-sync.md](05-sync.md) | Sync engine: full vs delta, scheduling, concurrency, progress, errors |
| [06-api.md](06-api.md) | REST API contract (`/api/*`) |
| [07-ui-screens.md](07-ui-screens.md) | Every screen of the SPA, routes, states, components |
| [08-deployment.md](08-deployment.md) | Dockerfile, docker-compose, env vars, reverse proxy, backups, CI |
| [09-roadmap.md](09-roadmap.md) | Milestones, definition of done, open questions |

## Decisions already taken

- v1 covers **App Store and Google Play only** (Microsoft Store later).
- v1 data: **downloads/installs, ratings & reviews, crashes**. No revenue, no subscriptions.
- **Single workspace**, several users with roles `admin` / `viewer`.
- Store credentials are **mounted files / env vars** — never entered through the UI, never stored in the DB.
- Sync runs in two modes, **full** (from account creation) and **delta** (since last successful run), started **manually from the dashboard** or **once a day at a configurable time**.
- Charts use **Chart.js** (the only third-party UI dependency); everything else is hand-written Svelte + Tailwind.
- CI is **GitHub Actions**, images go to **ghcr.io/oxisoft/oxilytics**, UI is **English only** (svelte-i18n is still wired so a second locale is a file drop).
