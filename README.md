# pagotask

Fast keyboard capture of Google Tasks on Windows. One tray exe, one hotkey
(`Win+Shift+T`), a popup prefilled from the page in front of you, Enter.

Status: skeleton. Tray, hotkey, config, Google sign-in, offline queue and the
page reader are in; the popup editor is next. Until it lands, the hotkey saves
the prefilled draft straight to the queue (only when a rule or a browser page
matched).

## Build

```
go build -ldflags "-H windowsgui" -o pagotask.exe ./cmd/pagotask
```

Sign-in needs a Google OAuth desktop client. Either bake one in:

```
go build -ldflags "-H windowsgui -X github.com/pashagolub/pagotask/internal/gtasks.DefaultClientID=... -X github.com/pashagolub/pagotask/internal/gtasks.DefaultClientSecret=..." ./cmd/pagotask
```

or put `google.client_id` / `google.client_secret` in `config.yaml`.

## Config

`%APPDATA%\pagotask\config.yaml` is written with defaults on first run and
reloaded when it changes. See `internal/config/default.yaml` for the shape:
lists, tags (emoji + key + default list), sources (what to read per process)
and rules (URL/title regexps that prefill tag, title and list).

## Layout

- `cmd/pagotask` – wiring
- `internal/config` – YAML config and validation
- `internal/rules` – capture → prefilled draft
- `internal/dates` – `tod`, `tom`, `mon`, `+3` …
- `internal/queue` – on-disk outbox with retries
- `internal/gtasks` – OAuth desktop flow and Tasks API
- `internal/platform` – OS interfaces; `windows/` (Win32, UI Automation,
  Credential Manager, systray) and `other/` (headless stand-in)
