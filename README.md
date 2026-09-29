# palorosa-kitchen

Kitchen list automation for Palorosa. Turns the orders of one delivery day into the aggregated food units the kitchen must prepare. Example output: 6 small semi-natural orange juices, 7 small yogurts, 15 simple sandwiches, 1 large sandwich. Replaces the printed template that is filled by hand today.

Status: the seed is accepted as catalog version 1. The offline single-file app, the local team server and the WhatsApp bot run in local validation. Pending: the WordPress plugin (catalog CRUD, list button, new order hook), the WhatsApp Cloud API adapter and hardening (roles, E2E, CI).

## Consumers

All consumers share one core library.

| Location | Consumer | Role |
| --- | --- | --- |
| `apps/standalone` | Single-file HTML app | Imports orders (rótulos PDF, store XLSX/CSV or JSON), resolves products and options to kitchen units and exports the daily list. Works offline with no server. |
| `apps/kitchen-server` | Local team server | Serves the list app on the LAN, stores the published list per delivery date, renders the sheet PDF with headless Chromium and proxies the catalog editor from localhost. |
| `apps/whatsapp-bot` | WhatsApp service | Notifies the cook about new and updated orders and exports the list on demand (`lista`, `lista mañana`, `estado`, `ayuda`). |
| `apps/wp-plugin` | WordPress plugin (pending) | Kitchen list button on the orders screen, CRUD admin section for the whole catalog and the new order hook. |

## Shared core

`packages/core` is framework free and browser safe. It holds the catalog schema and index, the engine (resolve an order line to units, aggregate the day list, diff two lists, with a default-option fallback for required choice groups), label parsing (rótulos text, JSON export, wide XLSX/CSV export), formatters (list text, CSV, print-HTML sheet with a manual unit-to-row mapping) and the Spanish label maps. The migration tool and every app depend on it.

## Stack

| Area | Choice |
| --- | --- |
| Package manager | pnpm 10 workspaces |
| Runtime | Node 22 or newer |
| Language | TypeScript 6 strict, ESM only |
| Lint and format | ESLint 9 with neostandard (Standard style, `eslint --fix` formats) |
| Tests | Vitest 5 |
| Editor | VS Code config committed in `.vscode` |

Deeper rationale lives in the internal documentation, which is not published.

## Repository layout

```
packages/core          Shared library. Schema, engine, parsing, formatting.
apps/standalone        Single-file HTML app built from core.
apps/kitchen-server    Local team server: list app, list API, sheet PDF, editor proxy.
apps/whatsapp-bot      WhatsApp service for notifications and list export.
apps/wp-plugin         WordPress plugin. Placeholder until Phase 2.
tools/migration        Dev only. Snapshot, migrate, diff and source sync. Source constants are private.
tools/catalog-editor   Dev only. Local review and CRUD web over the generated seed.
data                   Generated catalog seed (published). Other artifacts are private.
docs                   Internal documentation (not published).
```

Private inputs (business constants, overrides, the WordPress snapshot and the
review queue) are not committed. Rebuild them with the WordPress credentials
(`pnpm snapshot`) plus the private evaluation repo (`pnpm sync:source`).

## Commands

| Command | Action |
| --- | --- |
| `pnpm install` | Install all workspace dependencies |
| `pnpm lint` | Lint every package |
| `pnpm lint:fix` | Lint and apply automatic fixes (this is the formatter) |
| `pnpm check` | Type check every package with `tsc --noEmit` |
| `pnpm test` | Run Vitest in every package |
| `pnpm hooks:install` | Point git at `.githooks/` (the pre-push hook rebuilds the list) |
| `pnpm sync:source` | Mirror constants and overrides from the evaluation repo (needs `.env`) |
| `pnpm snapshot` | Refresh the WordPress product snapshot (needs `.env`) |
| `pnpm migrate` | Build the catalog seed from the snapshot and evaluation constants |
| `pnpm diff:wp` | Compare the snapshot against the seed |
| `pnpm editor` | Open the local catalog review and CRUD web (http://localhost:5199) |
| `pnpm build:list` | Build the standalone kitchen list app into one HTML file |
| `pnpm server` | Local team server: list app, list API and editor proxy (http://localhost:5200) |
| `pnpm bot` | WhatsApp bot: list commands and order notifications (hooks on port 5210) |
| `pnpm dev` | Run dev servers when apps exist |

Copy `.env.example` to `.env` and fill the WordPress credentials before using `snapshot` or `diff:wp`.

## Standalone kitchen list

`pnpm build:list` produces `apps/standalone/dist/index.html`, one file with the catalog seed embedded. Open it from the file system with no server and no network, or let `apps/kitchen-server` serve it on the LAN. Import a rótulos PDF, the store order export (XLSX or CSV) or a JSON export; the app resolves products and options to kitchen units and shows the day list grouped by category. Actions: print the kitchen sheet (the paper-template layout with the day's quantities), copy the list text and download the CSV.

Below the list it shows **Sin resolver** (unknown product, missing recipe or an unmatched required choice; unknown products can be mapped to a catalog product and the mapping is remembered in the browser), **Avisos** (lines the rótulo did not quantify) and **No van a cocina** (non-kitchen or inactive units). When served over HTTP it also publishes the computed list to the team server.

## Local team server

`pnpm server` serves the built list app on the local network (`http://<machine>:5200/`) and stores the last list published per delivery date: `POST`/`GET /api/list` for the app and the bot, an index at `/api/lists`, `/health`, and `/api/pdf` that renders the sheet with headless Chromium. The catalog editor is proxied at `/editor/` but only reachable from the server machine. Configuration in `.env` (`KITCHEN_SERVER_PORT`, `KITCHEN_SERVER_HOST`, `KITCHEN_EDITOR_AUTOSTART`, `KITCHEN_CHROMIUM_PATH`). Run `pnpm build:list` first so the app exists.

## WhatsApp bot

`pnpm bot` starts the WhatsApp service: link the account by scanning the QR (or set `WHATSAPP_PAIRING_PHONE` for a pairing code), fill `WHATSAPP_ALLOWLIST` with the numbers that can command it, and point `KITCHEN_SERVER_URL` at the team server. Commands: `lista`, `lista mañana`, `estado`, `ayuda`. Notifications: `POST /hook/order` with `{ deliveryDate, orderNumber, lines }` stores the order, recomputes the day list and messages the unit delta; `POST /hook/orders` replaces the whole day. Set `BOT_HOOK_TOKEN` to require the `x-bot-token` header.

## Catalog editor

`pnpm editor` serves a local review web over the generated seed: breakfasts with their recipes, kitchen units, products and containers. Every field is editable and saving writes the patch into `catalog/overrides.json` in the evaluation repo, then reruns the migration. The changes tab shows the patch and allows downloads.

## GitHub Pages

The `Deploy kitchen list` workflow (`.github/workflows/pages.yml`) builds the standalone app on every push to `main` and publishes it to GitHub Pages, so the kitchen list web stays current with the code and the seed. Enable Pages once with source "GitHub Actions". The published page is the same self-contained file: it works with no server, and the "publish to the team server" step simply does nothing there.

## Git hooks

`pnpm hooks:install` points git at `.githooks/`. The `pre-push` hook rebuilds the kitchen list app (`pnpm build:list`) before every push, so a broken build never lands.

## Documentation

This README is the public manual. Requirements, the decision log, the
architecture, the data model and the migration notes live in the internal
`docs/` folder, which is not published.

## Principles

- Headless first. The main function must run from a raw `.html` file.
- Mapping rules are editable data, not code.
- One source of truth for catalog inputs: the `palorosa-breakfast-evaluation` repo holds the constants and `catalog/overrides.json`. This repo mirrors them with `pnpm sync:source`. WordPress is the source of truth for products and variations.
- English in code, fields, comments and documentation. Spanish only in end-user surfaces, through central label maps.
- Customer data stays local. The kitchen list does not need names, phones or addresses.

## Out of scope for now

Balloons in any form. Route planning. Payments. Customer-facing interfaces.
