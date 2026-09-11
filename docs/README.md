# Oxilytics — project documentation

Oxilytics is an open-source (MIT), self-hosted analytics dashboard for app publishers. It pulls
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
| [10-store-setup-guides.md](10-store-setup-guides.md) | Step-by-step credential setup for App Store Connect and Google Play (also rendered inside the app) |

## Decisions already taken

- v1 covers **App Store and Google Play only** (Microsoft Store later).
- **Products are first-class**: a product (e.g. "My Notes") groups its store listings (iOS, Android, later Windows). Every stat is aggregated per product and can be broken down by platform.
- **At least one store must be configured** or the app refuses to do anything except show the setup guide.
- **Store apps can be ignored**: an admin marks stale/uninteresting listings as ignored; they disappear from every screen and from sync, and live only in an admin-only "Ignored apps" list where they can be restored.
- **Linking is always manual**: sync never auto-links a store app to a product; it only suggests, an admin confirms.
- **No rating history in v1**: only the current store-wide average/count per store app (refreshed each sync) plus review stars. Daily rating series are v2.
- An App Store listing covering iOS+macOS is platform `ios`; macOS-only is `macos`.
- No single-app sync in v1; stores sync in parallel, one run per store.
- v1 data: **downloads/installs, ratings & reviews, crashes**. No revenue, no subscriptions.
- **Single workspace**, several users with roles `admin` / `viewer`.
- Store credentials are **mounted files / env vars** — never entered through the UI, never stored in the DB. The UI contains a **setup guide** per store with the exact steps and a connection test.
- Sync runs in two modes, **full** (from account creation) and **delta** (since last successful run), started **manually from the dashboard** or **once a day at a configurable time**.
- Charts use **Chart.js** (the only third-party UI dependency); everything else is hand-written Svelte + Tailwind.
- CI is **GitHub Actions**, images go to **ghcr.io/oxisoft/oxilytics** (the repo's owner namespace; forks build their own), UI is **English only** (svelte-i18n is still wired so a second locale is a file drop).
