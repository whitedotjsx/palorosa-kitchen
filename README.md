# palorosa-kitchen

Kitchen list automation for Palorosa. Turns the orders of one delivery day into the aggregated food units the kitchen must prepare. Example output: 6 small semi-natural orange juices, 7 small yogurts, 15 simple sandwiches, 1 large sandwich. Replaces the printed template that is filled by hand today.

Status: the seed is accepted as catalog version 1. The offline single-file app and the Windows desktop app (WebView2 window, tray, WhatsApp bot, Cloudflare tunnel and admin panel) run in local validation. Pending: the WordPress plugin (catalog CRUD, list button, new order hook) and the WhatsApp Cloud API adapter. The retired local team server and catalog editor live in `archive/`.

## Consumers

All consumers share one core library.

| Location | Consumer | Role |
| --- | --- | --- |
| `apps/standalone` | Single-file HTML app | Imports orders (rótulos PDF, store XLSX/CSV or JSON), resolves products and options to kitchen units and exports the daily list. Works offline with no server. |
| `apps/desktop` | Windows desktop shell (Go) | WebView2 window with the admin panel, a system tray icon, autostart, the WhatsApp bot (whatsmeow, no browser), the Cloudflare tunnel and a self-update from GitHub Releases. |
| `apps/panel` | Admin panel (Svelte) | The single-file panel the desktop app serves on loopback: live kitchen state, orders, catalog editor, destinations, bots and settings. |
| `apps/whatsapp-bot` | WhatsApp service (legacy Node) | Superseded by the bot bundled in the desktop app; kept for reference. |
| `apps/wp-plugin` | WordPress plugin (pending) | Kitchen list button on the orders screen, CRUD admin section for the whole catalog and the new order hook. |
| `archive/kitchen-server`, `archive/catalog-editor` | Retired | The former LAN team server and the standalone catalog editor, superseded by the desktop panel. |

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
apps/panel             Admin panel (Svelte single-file) served by the desktop app.
apps/desktop           Windows desktop shell (Go): WebView2 window, tray, bot, tunnel, self-update.
apps/whatsapp-bot      Legacy Node WhatsApp service, superseded by the desktop bot.
apps/wp-plugin         WordPress plugin. Placeholder until Phase 2.
archive                Retired kitchen-server and catalog editor, kept for reference.
tools/migration        Dev only. Snapshot, migrate, diff and source sync. Source constants are private.
tools/orders-export    Dev/server tool. Pulls a delivery day's orders from the store WP All Export (HTTP only).
scripts                Build and watch scripts for the desktop exe.
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
| `pnpm orders <YYYY-MM-DD>` | Pull that day's store orders and print the kitchen list (needs `.env`) |
| `pnpm build:list` | Build the standalone kitchen list app into one HTML file |
| `pnpm bot` | WhatsApp bot (legacy Node): list commands and order notifications (hooks on port 5210) |
| `pnpm desktop` | Windows desktop shell (Go): WebView2 window plus tray icon |
| `pnpm desktop:build` | Build `apps/desktop/dist/palorosa-kitchen.exe` |
| `pnpm dev:desktop` | Watch the desktop app: rebuild the panel and the exe and relaunch on change |
| `pnpm panel` | Panel SPA dev server (Vite, hot reload) |
| `pnpm build:panel` | Build `apps/panel/dist/index.html` |
| `pnpm dev` | Run dev servers when apps exist |

Copy `.env.example` to `.env` and fill the WordPress credentials before using `snapshot` or `diff:wp`.

## Standalone kitchen list

`pnpm build:list` produces `apps/standalone/dist/index.html`, one file with the catalog seed embedded. Open it from the file system with no server and no network, or serve it from any static host. Import a rótulos PDF, the store order export (XLSX or CSV) or a JSON export; the app resolves products and options to kitchen units and shows the day list grouped by category. Actions: print the kitchen sheet (the paper-template layout with the day's quantities), copy the list text and download the CSV.

Below the list it shows **Sin resolver** (unknown product, missing recipe or an unmatched required choice; unknown products can be mapped to a catalog product and the mapping is remembered in the browser), **Avisos** (lines the rótulo did not quantify) and **No van a cocina** (non-kitchen or inactive units).

## Store orders pull

`pnpm orders <YYYY-MM-DD>` pulls one delivery day's orders straight from the store's WP All Export Pro and prints the aggregated kitchen list. It is **HTTP only, no browser**: it logs into wp-admin, points the saved export's delivery-date filter at that day, triggers the export, downloads the XLSX and runs it through the same engine as the app.

```
pnpm orders 2026-10-01
pnpm orders 2026-10-01 --publish http://127.0.0.1:5200   # also post it to a compatible list server
pnpm orders 2026-10-01 --json                            # dump the full list
```

Configuration in `.env`: `WP_ADMIN_URL`, `WP_ADMIN_USER`, `WP_ADMIN_PASSWORD` (a wp-login password, not an application password), `WP_EXPORT_ID` and `WP_EXPORT_CRON_KEY` (WP All Export -> Settings -> Cron Job Key). `WP_EXPORT_TOKEN` is derived from the cron key when left empty. The store sits behind a WAF, so the client sends browser headers.

## WhatsApp bot (legacy Node)

`pnpm bot` starts the Node WhatsApp service kept for reference. The desktop app bundles its own bot; this one needs `KITCHEN_SERVER_URL` pointing at a list server. Commands: `lista`, `lista mañana`, `estado`, `ayuda`. Notifications: `POST /hook/order` with `{ deliveryDate, orderNumber, lines }` stores the order, recomputes the day list and messages the unit delta; `POST /hook/orders` replaces the whole day. Set `BOT_HOOK_TOKEN` to require the `x-bot-token` header.

## Desktop app

`pnpm desktop` runs `apps/desktop`, a Windows shell (Go) that shows the built list app in a WebView2 window with a system tray icon (open, link WhatsApp, start with the system, quit) and autostart through `HKCU\...\Run`. It starts hidden in the tray with no flash on start: the app creates its own host window (hidden) and embeds the WebView2 in it, which is the supported embedding path and needs `CoInitializeEx` (the webview backend only does it when it owns the window). It only shows the window when you ask it to (tray "Abrir"/"Ver lista", or a second launch); closing the window hides it again (tray "Salir" quits). It is single-instance: launching the `.exe` again while it is running just shows the existing window and exits. It loads the admin panel (`apps/panel/dist/index.html`, `pnpm build:panel`), served by its own local panel server; set `KITCHEN_PANEL_HTML` to point at another build. `pnpm desktop:build` produces `apps/desktop/dist/palorosa-kitchen.exe` (a Windows GUI app, so it opens no console window). The window drops the native Windows caption in favour of a branded HTML title bar (minimize, maximize, close, resize) that only shows on desktop, keeps the DWM shadow and Aero Snap, uses the brand icon for the taskbar, and hides the web affordances (F5, devtools keys, text selection). `pnpm dev:desktop` watches the sources, rebuilds the panel and the exe and relaunches the app on every change.

It also bundles the WhatsApp bot using **whatsmeow** (Go, WhatsApp Web multidevice protocol, **no headless browser**), replacing the Node `apps/whatsapp-bot`. It shares `KITCHEN_SERVER_URL`, `WHATSAPP_ALLOWLIST`, `WHATSAPP_PAIRING_PHONE`, `BOT_PORT` and `BOT_HOOK_TOKEN` with the Node bot, and the session plus bot state live in `%APPDATA%\palorosa-kitchen`. On first run, or from the tray item "Vincular WhatsApp", the window shows the QR to link the phone; when the codes expire the page shows a **Retry** button. Commands (plain Spanish with emojis, accent- and case-insensitive): `lista`, `lista mañana`, `lista 2026-10-05` / `lista 5 octubre`, `estado`, `listo` (checkpoint), `nuevo` (what arrived since `listo`, one numbered list of the breakfasts and their food units), `ayuda` (a numbered menu), and notifications with `notificaciones` (status), `notificaciones activar|desactivar`, `notificaciones nuevos activar|desactivar`, `notificaciones cambios activar|desactivar`, `notificaciones hoy activar|desactivar`, `notificaciones mañana activar|desactivar`. Greetings get a short answer and anything unrecognized gets pointed to `ayuda`. No English in the messages.

The exe **updates itself from GitHub Releases**: at launch and then every 6 hours it asks the GitHub API for `releases/latest`, compares the tag with its own version, verifies the asset SHA-256, downloads `palorosa-kitchen.exe` next to the running exe, renames the running exe (Windows allows renaming it), writes the new one and relaunches. The tray has **Buscar actualización** and Ajustes → Diagnóstico shows the state with manual check and apply buttons. `KITCHEN_AUTO_UPDATE=off` disables the automatic path. For scripts and support, the exe also answers `--version`, `--update-check` and `--update-now` on the console, each printing one JSON line.

Its order hooks: `POST /hook/order` and `POST /hook/orders` on `BOT_PORT` (5210), behind `BOT_HOOK_TOKEN`, recompute the day, diff against the previous list and message the unit changes to every allowlisted number. A native WooCommerce webhook can also drive it end to end with no PHP: add it in wp-admin (WooCommerce → Ajustes → Avanzado → Webhooks) for the topics "Pedido creado", "Pedido actualizado" and "Pedido eliminado", delivery URL `https://<tunnel-hostname>/hook/woocommerce`, and paste the same value as `WC_WEBHOOK_SECRET` in the Secret field; the bot rebuilds the export row from the order and verifies the `X-WC-Webhook-Signature`. Only **processing** orders are cooked: any other status (pending payment, cancelled, trashed) removes the order from the day. A **new** order notification names the order (for example `x1 Gold Basic | x12 ROSAS`), lists what to add and the resulting list totals. An **updated** order shows `Pedido original`, `Pedido actualizado` (with `(se eliminó N)` / `(se agregó N)`) and `Así queda la lista` with the new totals (`(nuevo total)`, `(ya no hay que hacer)`, `(nuevo en la lista)`); if no food unit changed it says so. They only fire for deliveries of today or tomorrow. The list commands read the published list from the configured list server and fall back to that computed list.

Set `TUNNEL_HOSTNAME` (for example `cocina.whitesu.dev`) to expose the app through a Cloudflare named tunnel. The connector is the **vendored cf-quick-tunnel library** (`apps/desktop/third_party/cf-quick-tunnel-rs`, built as `cf-tunnel.dll` and embedded in the exe), which speaks QUIC + Cap'n Proto-RPC to the Cloudflare edge in process: no `cloudflared` process at runtime. The tunnel fronts the local panel server (`PANEL_PORT`, default 5211), which also serves the hooks, so the webhook reaches the machine through one origin. The tray shows `Túnel: activo`. With an account certificate (`cloudflared tunnel login`) an installed cloudflared is used once to create the tunnel and route its DNS; without it the app reports the error and keeps the hooks local. `KITCHEN_WHATSAPP=off` runs the window without the bot.

Instead of the browser login, the tunnel can authenticate with a **token**: run `cloudflared tunnel token palorosa-kitchen` on a PC that is already logged in (or create the tunnel in the Zero Trust dashboard and copy its token), paste it in Ajustes → Tienda (or `TUNNEL_TOKEN`), and any other PC runs that one tunnel with no certificate and no login. The token only allows running the tunnel; the configuration export carries it, so moving the host is import-and-go.

On the very first run (no `settings.json` yet) the window opens on a **setup page** that asks for the tunnel hostname and, optionally, the store credentials, and can import a configuration file exported from the kitchen PC. Saving the form detects in place whether this computer is the host or a spectator, so a spectator reaches the invite screen (or signs in with the imported station key) with no manual restart. "Omitir por ahora" skips it and any `.env` in the folder is imported automatically.

Its host settings live in `settings.json` under `%APPDATA%\palorosa-kitchen` and win over `.env` (which is imported there on the first run): the store domain, the tunnel hostname and the WordPress credentials. Secrets are encrypted at rest with the Windows DPAPI. Ajustes → Tienda has **Guardar y reiniciar**: it saves and relaunches the app so the tunnel, port or catalog path apply without closing it by hand. Ajustes → Copia de la configuración exports every setting **including the secrets in clear** (wp-admin password, WooCommerce consumer secret, webhook secret, cron key); it does **not** include the Cloudflare certificate (`%USERPROFILE%\.cloudflared\cert.pem`), the tunnel credentials or the WhatsApp session, which must be copied (or re-created) separately to move a host to another PC.

The local panel server (`PANEL_PORT`, default 5211, loopback only) is the host. A direct local request is the host; anyone reaching it through the tunnel or the LAN is a spectator and needs an invite: the host mints one at `POST /api/panel/invites` and the spectator redeems it at `POST /api/panel/session` for a session cookie. Invites and sessions (tokens stored hashed) live in `sessions.json`; writes are host only, reads are open to any role, and `/hook/*` stays open with its token and signature.

The panel UI is a Svelte single-file app (`pnpm build:panel` → `apps/panel/dist/index.html`) served at `/`. The host sees Resumen, Bots, Destinos, Pedidos, Lista and Ajustes; a spectator sees Resumen, Pedidos and Lista. Resumen, Lista, Pedidos, Bots and Destinos read the live bot state through the panel API and refresh over server-sent events; Ajustes edits the host settings, shows the tunnel state, starts the one-time `cloudflared tunnel login`, and manages invites and connected spectators. Bots manages several WhatsApp accounts (`bots.json`, one session per account under `bots/<id>/`) over one shared kitchen, with the existing session migrated into `bots/default/`. Destinos manages notification **targets** (`targets.json`): each one is a phone with the events it wants and a schedule (immediate, or fixed times with quiet hours); scheduled notices go to a persisted queue that a minute ticker flushes, so a restart does not lose a digest. The panel is an installable PWA (manifest plus service worker) and can receive Web Push (VAPID) notifications. Set `KITCHEN_PANEL_HTML` to point at a different build.

## GitHub Pages

The `Deploy kitchen list` workflow (`.github/workflows/pages.yml`) builds the standalone app on every push to `main` and publishes it to GitHub Pages, so the kitchen list web stays current with the code and the seed. Enable Pages once with source "GitHub Actions". The published page is the same self-contained file: it works with no server, and the "publish to the team server" step simply does nothing there.

## Releases and self-update

The `Release desktop` workflow (`.github/workflows/release.yml`) builds the desktop exe on a `v*` tag — or by manual dispatch with the tag as input — and publishes `palorosa-kitchen.exe` plus its `.sha256` sidecar with `gh release create` (re-runs upload with `--clobber`). The updater compares the running version with the latest release tag, so only tagged releases reach the kitchen PCs:

```
pwsh -File scripts/build-desktop.ps1 -Version 0.2.0   # local build (optional)
git tag v0.2.0 && git push origin v0.2.0              # CI builds and publishes
```

The tag must be `vX.Y.Z`. The exe metadata (`go-winres`), the `main.version` variable (via `-ldflags -X`) and the release tag all carry the same version. A pre-release suffix (`v0.2.0-rc1`) compares as older than the plain release.

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
