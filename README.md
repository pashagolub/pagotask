# pagotask

Fast keyboard capture of Google Tasks on Windows. One tray exe, one hotkey
(`Win+Shift+T`), a popup prefilled from the page in front of you, Enter.

Status: first working cut. Tray, hotkey, config, Google sign-in, offline
queue, page reader and the popup editor are in; nothing is signed or packaged
yet.

## Popup keys

- type `pr pgwatch #345` or `call mom`: a tag word (or one of its aliases), a space, then the title
- Ctrl+L list (one letter), Ctrl+T tag list filtered as you type, Ctrl+D due (`tod`, `tom`, `fri`, `+3`, `24.12`), Ctrl+N notes
- Enter saves, Esc closes

## Build

```
go build -tags production -ldflags "-H windowsgui" -o pagotask.exe ./cmd/pagotask
```

The popup is a [Wails](https://wails.io) v2 window (WebView2, preinstalled on
Windows 10/11). Plain `go build` is enough; the Wails CLI is not needed.

Sign-in needs a Google OAuth desktop client. Either bake one in:

```
go build -tags production -ldflags "-H windowsgui -X github.com/pashagolub/pagotask/internal/gtasks.DefaultClientID=... -X github.com/pashagolub/pagotask/internal/gtasks.DefaultClientSecret=..." ./cmd/pagotask
```

or put `google.client_id` / `google.client_secret` in `config.yaml`.

## Config

`%APPDATA%\pagotask\config.yaml` is written with defaults on first run and
reloaded when it changes. See `internal/config/default.yaml` for the shape:
lists, tags (emoji + optional key word + default list), sources (what to read per process)
and rules (URL/title regexps that prefill tag, title and list).

## Layout

- `cmd/pagotask` – wiring
- `internal/config` – YAML config and validation
- `internal/rules` – capture → prefilled draft
- `internal/dates` – `tod`, `tom`, `mon`, `+3` …
- `internal/editor` – popup and tray icon, one Wails v3 app on Windows (headless elsewhere); `frontend/` is plain HTML/JS
- `internal/queue` – on-disk outbox with retries
- `internal/gtasks` – OAuth desktop flow and Tasks API
- `internal/platform` – OS interfaces; `windows/` (Win32, UI Automation,
  Credential Manager) and `other/` (headless stand-in)
