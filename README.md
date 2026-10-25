# Pigeonhole

A desktop app (macOS + Windows) that shows your Messenger, Instagram, WhatsApp and
Discord chats in one inbox, and lets you reply. Everything runs and is stored locally on your
computer, so there's no server to host.

**[⬇ Download the latest version](https://github.com/henzyliu/pigeonhole/releases/latest)** for Mac or Windows.

The name: a pigeonhole is a mail-sorting slot, pigeons carry messages, and by
the pigeonhole principle, putting several networks into one inbox means some
hole ends up holding more than one pigeon.

## Download

Go to the **[latest release](https://github.com/henzyliu/pigeonhole/releases/latest)**
(or the **Releases** section on the right of this page) and, under **Assets**,
download the file for your computer:

| Computer | File | Then |
|---|---|---|
| Mac (macOS 11 or later) | `Pigeonhole-<version>-mac.dmg` | Open it and drag Pigeonhole into Applications. |
| Windows 10 or 11 | `Pigeonhole-<version>-windows-setup.exe` | Run it. It installs just for you, with no admin prompt. |
| Windows, without installing | `Pigeonhole-<version>-windows-portable.zip` | Unzip it and run `Pigeonhole.exe`. |

**The first time you open it** you'll see a warning, because the app isn't
signed with a paid developer certificate. You only need to get past it once:

- **Mac:** macOS says it can't check Pigeonhole for malware. Click **Done**, go to
  System Settings → Privacy & Security, scroll down and click **Open Anyway**.
- **Windows:** if Windows says it protected your PC, click **More info** →
  **Run anyway**.

Then click the person icon to connect your accounts. Pigeonhole keeps running in
the menu bar (Mac) or by the clock (Windows) when you close its window, so you
keep getting messages; quit it from that icon.

**Updates install themselves:** when there's a new version, Pigeonhole tells you,
and **Install update** in Settings downloads it and restarts the app. Your
accounts and chats are kept. (Versions before 0.10.0 can't do this; download
the latest version once by hand.)

Your messages stay on your computer. Connecting uses the same unofficial method
as the mautrix bridges, which the networks don't officially allow, so there's a
small risk of an account being asked to verify a login or being flagged.

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
git tag v0.10.2 && git push origin v0.10.2
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

Pigeonhole updates itself from these releases (`internal/update`, `updates.go`):
it checks `releases/latest` 30 seconds after starting and every 6 hours, sends
one notification per new version, and shows it in Settings → About and as a
banner above the chat list. **Install update** downloads this system's file and
installs it: on Windows it runs the installer silently, then starts the new
version; on macOS it copies the app out of the disk image beside the running
one, and a small script swaps them once Pigeonhole quits and reopens it. Files
the app downloads itself aren't quarantined, so updates skip the first-launch
prompt. A Mac copy in a folder it can't write to offers the download instead.
Versions before 0.10.0 can't update themselves, so install 0.10.0 by hand once.

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
- For licensing, see [Licence](#licence) below.

## Licence

Copyright © 2026 Henry Liu.

Pigeonhole is free software: you can redistribute it and/or modify it under the
terms of the **GNU Affero General Public License, version 3 or (at your option)
any later version**, as published by the Free Software Foundation. It's
distributed in the hope that it will be useful, but **without any warranty**;
without even the implied warranty of merchantability or fitness for a particular
purpose. See [`LICENSE`](LICENSE) for the full text.

In short: you can use, share and change Pigeonhole, but if you give anyone a copy
(changed or not), you must offer them its source code under these same terms.
The source is this repository.

Pigeonhole is AGPL because it's built on the
[mautrix](https://github.com/mautrix) bridges, which are licensed under the
AGPL-3.0-or-later. Pigeonhole also includes or uses:

| Project | Used for | Licence |
|---|---|---|
| [mautrix-meta](https://github.com/mautrix/meta) (a modified copy in `third_party/mautrix-meta`) | Messenger and Instagram | AGPL-3.0-or-later |
| [mautrix-whatsapp](https://github.com/mautrix/whatsapp) | WhatsApp | AGPL-3.0-or-later |
| [mautrix-discord](https://github.com/mautrix/discord) | Discord | AGPL-3.0-or-later |
| [mautrix-go](https://github.com/mautrix/go) and [go.mau.fi/util](https://github.com/mautrix/go-util) | The bridge framework | MPL-2.0 |
| [whatsmeow](https://github.com/tulir/whatsmeow) | The WhatsApp protocol | MPL-2.0 |
| [discordgo](https://github.com/bwmarrin/discordgo) (Beeper's fork) | The Discord protocol | BSD-3-Clause |
| [Wails](https://github.com/wailsapp/wails) and [go-webview2](https://github.com/wailsapp/go-webview2) | The desktop app shell | MIT |
| [go-sqlite3](https://github.com/mattn/go-sqlite3) | Local storage (SQLite) | MIT |
| [React](https://react.dev) | The interface | MIT |
| [zerolog](https://github.com/rs/zerolog), [go-qrcode](https://github.com/skip2/go-qrcode), [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) | Logging, QR codes, image handling | MIT / MIT / BSD-3-Clause |

Each project keeps its own copyright and licence (in its source, or in
`third_party/` for the copied one). The `third_party/mautrix-meta/LICENSE.exceptions`
file grants extra rights to Beeper only; it doesn't apply to Pigeonhole.

Pigeonhole isn't affiliated with or endorsed by Meta, WhatsApp, Discord or the
mautrix project. Messenger, Instagram, WhatsApp and Discord are trademarks of
their owners.
