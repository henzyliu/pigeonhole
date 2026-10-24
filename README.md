# Pigeonhole

A desktop app (macOS + Windows) that shows your Messenger, Instagram and WhatsApp
chats in one inbox, and lets you reply. Everything runs and is stored locally on your
computer, so there's no server to host.

The name: a pigeonhole is a mail-sorting slot, pigeons carry messages, and by
the pigeonhole principle, putting several networks into one inbox means some
hole ends up holding more than one pigeon.

## How it works

```
 Messenger ─┐                                   ┌─ React UI (Wails webview)
 Instagram ─┼─ mautrix connectors ───── localmx ┤
 WhatsApp  ─┘   (in-process, Go)        (SQLite)  └─ inbox service
```

- **Connectors**: the network code from the [mautrix-meta](https://github.com/mautrix/meta)
  bridges (`pkg/connector` for Messenger, `pkg/igconnector` for Instagram) and
  [mautrix-whatsapp](https://github.com/mautrix/whatsapp), imported as Go libraries. They handle login, sync, E2EE Messenger chats, media, reactions
  and edits.
- **`internal/localmx`**: a tiny in-process stand-in for a Matrix homeserver. The
  connectors are written for the mautrix `bridgev2` framework, which expects to talk
  to Matrix. `localmx` implements that interface (`bridgev2.MatrixConnector`) but
  writes rooms, messages, reactions and media into a local SQLite database instead.
- **`internal/inbox`**: starts one bridge per network, drives login flows and
  exposes chats/messages/send to the UI.
- **`internal/loginbrowser`**: the login window. For the bridges' "cookies" login
  steps, it opens the real website in a private browser window (WebKit on macOS,
  WebView2 on Windows), lets you log in normally, and collects the cookies or
  tokens the bridge asked for. The native code only reports what the page holds;
  deciding when the login is done is shared Go code.
- **`frontend/`**: React + TypeScript UI, talking to Go through Wails bindings.
- **`third_party/mautrix-meta`**: a local copy of mautrix-meta with small patches
  (wired in via a `replace` in `go.mod`). Currently: the thread backfill also pages
  through Messenger's end-to-end encrypted thread list (sync group 95), which
  upstream only fetches the first page of. See `pkg/connector/threadbackfill.go`.

Adding another network (Slack, Discord, X, Telegram, ...) means adding its mautrix
`bridgev2` connector in `inbox.New`; the UI picks it up automatically.

Data lives in `~/Library/Application Support/Pigeonhole` (macOS) or
`%AppData%\Pigeonhole` (Windows): `inbox.db` (what the UI shows),
`<network>.db` (bridge state and sessions), `media/` and `logs/`. Data from
before the rename (`UnifiedInbox`) is moved there automatically on first launch.

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
go test . ./internal/...
```

## Building

```bash
scripts/build.sh
```

This produces `build/bin/Pigeonhole.app` on macOS, with the commit, branch and
build date stamped in (Settings → About shows them, and the log records them at
startup). The version number itself is `info.productVersion` in `wails.json`.
SQLite uses cgo, so a Windows build from a Mac needs the MinGW cross-compiler
(`brew install mingw-w64`):

```bash
scripts/build.sh windows
```

The result, `build/bin/Pigeonhole.exe`, only needs DLLs that ship with Windows
10 and 11 (plus WebView2, which they include). The GitHub Actions workflow in
`.github/workflows/build.yml` builds the macOS app and a Windows installer
(`-nsis`) when you push a `v*` tag or run it manually.

## Notes

- Logging in uses Meta's private web/mobile APIs, the same way the mautrix
  bridges and Beeper do. This is against Meta's terms. Meta may ask you to verify
  a login, and there's a small risk of an account being flagged.
- The login window is the most reliable way to sign in: password login from a
  new "device" can push Meta into account-recovery checks. For Messenger it uses
  facebook.com, since messenger.com hands logins back via cross-site cookies that
  WebKit blocks. Pasting cookies from your own browser still works as a fallback.
- WhatsApp links as a companion device, like WhatsApp Web (listed as
  "Pigeonhole" under Linked Devices on your phone). WhatsApp unlinks companions
  when the phone has been offline for about two weeks. Status updates aren't shown.
- Messenger chats that *started* end-to-end encrypted aren't in any thread list
  the bridge can read; they appear once a new message arrives in them.
- The app icon's source is `build/icon/appicon.svg` (a pigeon in a cabinet of
  mail-sorting cubbies). `build/appicon.png` (macOS) and `build/windows/icon.ico`
  are generated from it with:

  ```bash
  go run -C build/icon/gen . -svg ../appicon.svg -out ../..
  ```
- mautrix-meta and mautrix-whatsapp are AGPL-3.0, so this app is too if you distribute it.
