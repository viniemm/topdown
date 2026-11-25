# topdown

One TUI to launch, resume and manage coding agents: Claude Code, Codex,
OpenCode, Pi, Oh My Pi, Hermes, Antigravity and the rest. One small Go binary
(about 10 MB, no runtime) on Charm's [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and Lip Gloss. The look comes from studying omp's model picker
([pi-tui](https://github.com/can1357/oh-my-pi), MIT): a framed hub with a
sidebar of scopes, a searchable list, a detail panel and a key bar.

```
╭─────────────────────────────────────────────────────────────────────────────╮
│ ▀▀▀▀▀▀▀▀▀    █               █                            6 apps · ● 1 open │
│ ▀███████▀   ▀█▀ ▄▀▀▄ █▀▀▄ ▄▀▀█ ▄▀▀▄ █ █ █ █▀▀▄                 default claude │
│  ▀█████▀     █▄ ▀▄▄▀ █▄▄▀ ▀▄▄█ ▀▄▄▀ ▀▄▀▄▀ █  █                                │
│    ▀█▀               █      v0.8.0                   agents, from the top down │
├──────────────────┬──────────────────────────────────────────────────────────┤
│   ▦ Apps       6 │  Recent conversations                                    │
│   ◷ Recent       │  ›                                                       │
│ ──────────────── │  ● 17:32      claude  topdown      Port the launcher     │
│   ◆ claude   22% │    Wed 18:39  hermes  mintern      Lightest Linux distro │
│   ◆ hermes       │    Nov 12     pi      xfin         Refactor auth module  │
│                  │ ──────────────────────────────────────────────────────── │
│                  │  Port the launcher                                       │
│                  │  claude  ~/Work/topdown  17:32 · 3s ago  ● open          │
├──────────────────┴──────────────────────────────────────────────────────────┤
│ ⏎ open · Alt-⏎ window · ^O options · Tab apps · ^E rename · ^X delete · esc │
╰─────────────────────────────────────────────────────────────────────────────╯
```

Pick something and the hub's own terminal becomes that agent; no second window.

## Keys

| Key | Action |
|---|---|
| Enter / Alt-Enter | Launch or resume here / in a new window (an open conversation: switch to its window, or to its [herdr](https://herdr.dev) pane, attaching herdr if no client is) |
| Enter on an agent | Its page: **+ New conversation** on top, its conversations below (typing searches them). Enter on New picks the folder: your workdir is first, so Enter again starts there; or type to search [zoxide](https://github.com/ajeetdsouza/zoxide)'s folders and the agent's projects, a path (`~/`, `/`, `./`; Tab goes into a folder), or a name that isn't a folder yet (**+ new folder** makes it). Alt-Enter on an agent still launches straight away |
| Ctrl-O | Launch options (Claude: model, effort, permission mode, fork on resume, Chrome) |
| Tab | Apps ↔ recent conversations |
| → / ← | → is Enter (open); ← goes back (it also leaves a search there) |
| Type | Fuzzy search (agent, project, title) |
| Ctrl-D | Make the highlighted agent the default |
| Ctrl-R | Refresh the agent's usage |
| Ctrl-A | Add an app (installed agents, or any command) |
| Ctrl-E / F2 | Rename a conversation (in the agent's own format) or an app |
| Ctrl-X / Del | Delete a conversation (to the Trash, or the agent's own delete) or remove an app; asks first, refuses open ones |
| Esc | Clear the search, go back, or close |

`←` from the list focuses the sidebar (`↑/↓` switch scopes there).

## Commands

    topdown [pick]                 the hub in this terminal
    topdown open                   the hub in a new terminal window
    topdown default                launch the default agent (none yet: the hub chooses)
    topdown launch <app-id>
    topdown resume <agent> <session-id>
    topdown list [agent…]          recent conversations, tab-separated

Suggested Hyprland bindings (`~/.config/hypr/bindings.lua` on Omarchy):

    o.rebind("SUPER + A", "Agent", "topdown default")
    o.rebind("SUPER + SHIFT + A", "Agent hub", "topdown open")

## How it works

- **Agents** are adapters (`internal/agent/`): how to launch one, its launch
  options, and where its conversations live, how to resume, rename and delete
  them, and which are open right now. Claude Code, Pi, omp, Hermes and
  Antigravity have conversation support; the rest of Omarchy's agents launch.
- **Running in place.** When the hub closes, topdown `exec`s the agent, so the
  terminal becomes it and nothing stays running behind it.
- **Omarchy is optional.** When `omarchy-agent` exists, agents launch through
  it (auto-permission flags, account pinning, the agent window class), via a PATH shim that adds resume and extra arguments. Plan usage is
  read (never written) from `~/.local/state/omarchy/agents/usage/`. Without
  Omarchy, agents run directly.
- **Conversations** are read from each agent's store: Claude
  `~/.claude/projects`, Pi `~/.pi/agent/sessions`, omp `~/.omp/agent/sessions`,
  Hermes `~/.hermes/state.db`, Antigravity
  `~/.gemini/antigravity-cli/conversation_summaries.db`. Only rename and delete
  write: Claude gets a `custom-title` entry, Pi a `session_info` entry, Hermes
  its own `hermes sessions rename/delete`; deleted files go to the freedesktop
  Trash (restorable). Open Claude conversations are detected from
  `~/.claude/sessions/<pid>.json`.
- **Config:** `~/.config/topdown/apps.json` (migrated from Omarchy's
  `agent-launcher.json`). An agent app may add `"args"` to every launch and
  resume; Hermes defaults to `--tui`. `~/.config/topdown/settings.json` holds
  `{"workdir": "~/Work"}`: where new conversations start and new folders are
  made (default `~/Work` when it exists, else `~`).

## Install

**Arch (Omarchy or not):** install the package from the
[latest release](https://github.com/viniemm/topdown/releases/latest):

    sudo pacman -U https://github.com/viniemm/topdown/releases/download/v0.9.4/topdown-0.9.4-1-x86_64.pkg.tar.zst

pacman owns the files: run the newer release's line to upgrade, and
`sudo pacman -R topdown` to remove it. (An AUR package is coming; its
PKGBUILD is in `aur/topdown`.)

**Debian, Ubuntu:**

    curl -LO https://github.com/viniemm/topdown/releases/download/v0.9.4/topdown_0.9.4-1_amd64.deb
    sudo apt install ./topdown_0.9.4-1_amd64.deb

**Fedora:**

    sudo dnf install https://github.com/viniemm/topdown/releases/download/v0.9.4/topdown-0.9.4-1.x86_64.rpm

Each release also has arm64 / aarch64 builds and a `.sha256` for every file.
Remove with `sudo apt remove topdown` or `sudo dnf remove topdown`.

**From source, any distro:**

    git clone https://github.com/viniemm/topdown && cd topdown
    bin/install

On Arch, `bin/install` builds and installs a pacman package from your
checkout (`makepkg -si`): `/usr/bin/topdown`, one binary, plus the launcher
entry. It replaces the old `omarchy-agent-launcher` package (that name stays
as a symlink). Run it again to upgrade; `pacman -R topdown` removes it.

On **any other distro** (or to build Fedora, Debian or Ubuntu from source), `bin/install` builds with Go and
installs for your user into `~/.local` (`bin/topdown`, the launcher entry and
icon); no root needed. Building needs Go and git:

    sudo dnf install golang git          # Fedora
    sudo apt install golang-go git       # Debian, Ubuntu

Building needs Go 1.27 or later; a distro Go that's older but 1.21
or later downloads the right toolchain by itself on the first build.

    bin/install --system                 # for everyone, into /usr/local (asks for sudo)
    bin/install --uninstall              # remove it again (add --system for /usr/local)
    PREFIX=/opt/topdown bin/install      # anywhere else; DESTDIR=… for packaging

If `~/.local/bin` isn't on your PATH the installer says so.

**What works where.** Everything beyond a terminal is optional, and topdown
uses it when it's there:

| With | You get |
|---|---|
| Omarchy (`omarchy-agent`) | launches through it, plan-usage meters, theme colours |
| Hyprland | Enter on an open conversation switches to its window |
| [herdr](https://herdr.dev) | the same for conversations in herdr panes |
| [zoxide](https://github.com/ajeetdsouza/zoxide) | folder suggestions ranked by frecency |
| `gio` (glib2) | deleting files on other filesystems to their Trash |
| `notify-send` | errors from a keybinding shown as a notification |

New windows (`topdown open`, Alt-Enter) open in `xdg-terminal-exec` when it's
installed, else `$TERMINAL`, else the first of foot, kitty, alacritty,
ghostty, wezterm, ptyxis, gnome-terminal, konsole, xfce4-terminal or xterm.
Bind `topdown open` (the hub) and `topdown default` (straight to your default
agent) to keys in your desktop's shortcut settings.

## Colours

True colour, taken from your Omarchy theme
(`~/.local/state/omarchy/current/theme/colors.toml`), or, without Omarchy, asked
from the terminal itself (OSC 4/10/11); the 16 ANSI colours are the last
resort. Selection, borders and dim text are blended from that palette.
Colour coding: each agent its own colour (its brand hue where the theme has
it), each project a stable colour, timestamps fading with age, usage meters
green → yellow → red. Override any of it in `~/.config/topdown/theme.json`:
roles (`accent`, `border`, `dim`, `muted`, `success`, `warning`, `error`,
`selected`) and `{"agents": {"claude": "#d97757"}}`.

## Motion

The selection bar fades in as you move; open conversations' ● markers
breathe; a spinner shows while usage refreshes in the background.

The ticker only runs while something moves. Turn motion off with
`"animations": false` in `theme.json`, or `TOPDOWN_REDUCED_MOTION=1`. Motion
is also off without true colour.

## Development

    go run ./cmd/topdown     # the hub
    go test ./...            # against a throwaway home; never touches your real config or Trash

    scripts/mkhome .testhome/demo                    # a fixture home with sample conversations
    PATH=.testhome/demo/stub:$PATH scripts/inhome .testhome/demo go run ./cmd/topdown

See [ROADMAP.md](ROADMAP.md): every agent, MCP control, a skills manager,
search inside conversations.

Earlier versions: bash/fzf at tag `v0.4.4`; TypeScript/Bun up to commit `b88035e` on `main`.
