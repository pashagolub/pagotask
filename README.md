# pagotask

Fast keyboard capture of Google Tasks on Windows. One tray exe, one hotkey
(`Win+Shift+T`), a popup prefilled from the page in front of you, Enter.
A second hotkey (`Win+Shift+D`) lists your open tasks to check them off.

Status: first working cut. Tray, hotkey, config, Google sign-in, offline
queue, page reader and the popup editor are in; nothing is signed or packaged
yet.

## Popup keys

- type `pr pgwatch #345` or `call mom`: a tag word (or one of its aliases), a space, then the title
- the line under the title shows the tags that match the word you are typing (all of them while it is empty); Tab picks the first, Backspace at the start of the title removes the tag
- recent tasks (the last 20 added here or touched in Google Tasks in the past 30 days) are listed below when the popup opens empty, or after Down; typing filters them, Up/Down highlight, Enter fills tag, title and list, Enter again saves
- Ctrl+L list (one letter), Ctrl+T tag list filtered as you type, Ctrl+D due (`tod`, `tom`, `fri`, `+3`, `24.12`), Ctrl+N notes
- Enter saves, Esc closes

## Open tasks keys

`Win+Shift+D` (or "Open tasks" in the tray menu) shows open tasks from the
configured lists, due today or overdue, sorted by date. It opens instantly
from a local copy that refreshes on open and in the background. The `tasks:`
section of `config.yaml` sets its hotkey, which lists it shows and the refresh
interval.

- arrows move, Space checks or unchecks; a checked task stays struck through until the popup closes
- typing filters: title text, and tag words match their emoji (`pr pgw`); while you type, Space separates words, after an arrow key it checks
- Enter opens the first link in the task's notes, Tab switches between today and all open tasks, Esc closes

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

`sources` maps a process name (as in Task Manager's Details tab) to what is read
from that app when the hotkey is pressed:

| `read`  | What is read                 | Effect                                                                  |
|---------|------------------------------|-------------------------------------------------------------------------|
| `url`   | address bar and window title | URL and title rules; no match prefills page title + URL in notes        |
| `title` | window title only            | title rules only (also what happens for apps not listed)                |
| `none`  | nothing                      | the popup opens empty                                                   |

## Layout

- `cmd/pagotask` – wiring
- `internal/config` – YAML config and validation
- `internal/rules` – capture → prefilled draft
- `internal/dates` – `tod`, `tom`, `mon`, `+3` …
- `internal/editor` – popup and tray icon, one Wails v3 app on Windows (headless elsewhere); `frontend/` is plain HTML/JS
- `internal/queue` – on-disk outbox with retries (new tasks, checks and unchecks)
- `internal/opentasks` – local copy of open tasks and the rows the tasks popup shows
- `internal/recent` – recently used tasks for the add popup
- `internal/gtasks` – OAuth desktop flow and Tasks API
- `internal/platform` – OS interfaces; `windows/` (Win32, UI Automation,
  Credential Manager) and `other/` (headless stand-in)

## Credits

Flag emoji in the popup use the [Twemoji Country Flags](https://github.com/talkjs/country-flag-emoji-polyfill) font, with art from [Twemoji](https://github.com/twitter/twemoji) under CC-BY 4.0. See `internal/editor/frontend/fonts/`.
