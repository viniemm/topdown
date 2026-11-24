# topdown

One TUI to launch, resume and manage coding agents: Claude Code, Codex,
OpenCode, Pi, Oh My Pi, Hermes, Antigravity and the rest. One small Go binary
(about 10 MB, no runtime) on Charm's [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and Lip Gloss. The look comes from studying omp's model picker
([pi-tui](https://github.com/can1357/oh-my-pi), MIT): a framed hub with a
sidebar of scopes, a searchable list, a detail panel and a key bar.

```
╭─ topdown ────────┬──────────────────────────────────────────────────────────╮
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
| Enter / Alt-Enter | Launch or resume here / in a new window (an open conversation: switch to its window) |
| Ctrl-O | Launch options (Claude: model, effort, permission mode, fork on resume, Chrome) |
| Tab | Apps ↔ recent conversations |
| → / ← | An agent's conversations / back (← also leaves a search there) |
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
  it (auto-permission flags, account pinning, `~/Work`, the agent window
  class), via a PATH shim that adds resume and extra arguments. Plan usage is
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
  resume; Hermes defaults to `--tui`.

## Install (Arch)

    git clone https://github.com/viniemm/topdown && cd topdown
    bin/install

The package installs `/usr/bin/topdown` (one static binary) and replaces the
old `omarchy-agent-launcher` package (that name stays as a symlink).

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

## Development

    go run ./cmd/topdown     # the hub
    go test ./...            # against a throwaway home; never touches your real config or Trash

    scripts/mkhome .testhome/demo                    # a fixture home with sample conversations
    PATH=.testhome/demo/stub:$PATH scripts/inhome .testhome/demo go run ./cmd/topdown

See [ROADMAP.md](ROADMAP.md): every agent, MCP control, a skills manager,
search inside conversations.

Earlier versions: bash/fzf at tag `v0.4.4`; TypeScript/Bun up to commit `b88035e` on `main`.
