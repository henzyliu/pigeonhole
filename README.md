# Unified Inbox

A desktop app (macOS + Windows) that shows your Messenger and Instagram DMs in
one inbox, and lets you reply. Everything runs and is stored locally on your
computer, so there's no server to host.

## How it works

```
 Messenger ─┐                                   ┌─ React UI (Wails webview)
            ├─ mautrix-meta connectors ── localmx ┤
 Instagram ─┘   (in-process, Go)        (SQLite)  └─ inbox service
```

- **Connectors**: the network code from the [mautrix-meta](https://github.com/mautrix/meta)
  bridges (`pkg/connector` for Messenger, `pkg/igconnector` for Instagram), imported
  as a Go library. They handle login, sync, E2EE Messenger chats, media, reactions
  and edits.
- **`internal/localmx`**: a tiny in-process stand-in for a Matrix homeserver. The
  connectors are written for the mautrix `bridgev2` framework, which expects to talk
  to Matrix. `localmx` implements that interface (`bridgev2.MatrixConnector`) but
  writes rooms, messages, reactions and media into a local SQLite database instead.
- **`internal/inbox`**: starts one bridge per network, drives login flows and
  exposes chats/messages/send to the UI.
- **`frontend/`**: React + TypeScript UI, talking to Go through Wails bindings.

Adding another network (Slack, Discord, X, Telegram, ...) means adding its mautrix
`bridgev2` connector in `inbox.New`; the UI picks it up automatically.

Data lives in `~/Library/Application Support/UnifiedInbox` (macOS) or
`%AppData%\UnifiedInbox` (Windows): `inbox.db` (what the UI shows),
`<network>.db` (bridge state and sessions), `media/` and `logs/`.

## Developing

Requirements: Go 1.26+, Node 22+, and the Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
```

Run with hot reload:

```bash
wails dev
```

Run the backend tests:

```bash
go test ./internal/...
```

## Building

```bash
wails build -clean
```

This produces `build/bin/unified-inbox.app` on macOS. SQLite uses cgo, so each OS
is easiest to build on itself. The GitHub Actions workflow in
`.github/workflows/build.yml` builds the macOS app and a Windows installer
(`-nsis`) when you push a `v*` tag or run it manually.

## Notes

- Logging in uses Meta's private web/mobile APIs, the same way the mautrix
  bridges and Beeper do. This is against Meta's terms. Meta may ask you to verify
  a login, and there's a small risk of an account being flagged.
- Password login is recommended. The cookie flows are a fallback if Meta blocks
  password login: log in on the website, then paste a "Copy as cURL" request from
  the browser's devtools.
- mautrix-meta is AGPL-3.0, so this app is too if you distribute it.
