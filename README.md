# Pigeonhole

A desktop app (macOS + Windows) that shows your Messenger, Instagram, WhatsApp and
Discord chats in one inbox, and lets you reply. Everything runs and is stored locally on your
computer, so there's no server to host.

The name: a pigeonhole is a mail-sorting slot, pigeons carry messages, and by
the pigeonhole principle, putting several networks into one inbox means some
hole ends up holding more than one pigeon.

## How it works

```
 Messenger ─┐                                   ┌─ React UI (Wails webview)
 Instagram ─┤                                   │
 WhatsApp  ─┼─ mautrix connectors ───── localmx ┤
 Discord   ─┘   (in-process, Go)      (SQLite)  └─ inbox service
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
- **Discord**: [mautrix-discord](https://github.com/mautrix/discord)'s
  `bridgev2` rewrite (the `megadiscord` branch, not yet released upstream), pinned to one commit in
  `go.mod` along with the `beeper/discordgo` fork it needs. Its default config bridges
  DMs and group DMs only, no servers. Sign-in is the login window (which reads the
  token from discord.com) or a QR code scanned with the Discord app. Upgrading means
  picking a newer branch commit and checking it still builds against our mautrix.

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
startup). The version number itself is `info.productVersion` in `wails.json`;
bump it in the same commit as each change: the patch number (0.2.0 → 0.2.1)
for fixes, the minor number (0.2.x → 0.3.0) when adding features.
SQLite uses cgo, so a Windows build from a Mac needs the MinGW cross-compiler
(`brew install mingw-w64`):

```bash
scripts/build.sh windows
```

The result, `build/bin/Pigeonhole.exe`, only needs DLLs that ship with Windows
10 and 11 (plus WebView2, which they include).

## Releasing

To share a version, push a tag for it:

```bash
git tag v0.9.2 && git push origin v0.9.2
```

`.github/workflows/release.yml` then builds on GitHub's Mac and Windows
machines and publishes a [release](https://github.com/henzyliu/pigeonhole/releases)
with:

- `Pigeonhole-<version>-mac.dmg`: the app for Apple Silicon and Intel Macs
  (macOS 11 or later), to drag into Applications;
- `Pigeonhole-<version>-windows-setup.exe`: an installer that installs for the
  current user (no admin prompt), closes a running Pigeonhole first, and adds
  Start menu and desktop shortcuts;
- `Pigeonhole-<version>-windows-portable.zip`: the bare exe.

The tag has to match `info.productVersion` in `wails.json`, or the workflow
stops before building. The release notes list the commits since the previous
tag, followed by `.github/release-notes-install.md`, which explains the
one-time warnings: neither app is signed, so macOS asks you to allow it under
Privacy & Security and Windows SmartScreen asks you to "Run anyway". Running
the workflow by hand (Actions → Release → Run workflow) builds everything
without releasing, as a check.

To build the installer locally, install NSIS (`brew install makensis`) and run
`wails build -platform windows/amd64 -nsis` with the MinGW variables above.
Homebrew's `makensis` crashes unless `LANG` is a UTF-8 locale (e.g.
`LANG=en_US.UTF-8`).

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
- Discord doesn't allow unofficial clients ("self-bots"), so a Discord account
  used here could be flagged. Its bridge is unreleased upstream and may break.
- mautrix-meta, mautrix-whatsapp and mautrix-discord are AGPL-3.0, so this app is too if you distribute it.
