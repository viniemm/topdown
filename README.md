# omarchy-agent-launcher

A TUI launcher for coding agents and other terminal apps on Omarchy. Pick an
agent and the picker's own terminal becomes that agent — no second window.

- **Super+A** — launch the default agent (no default yet → the picker chooses one)
- **Super+Shift+A** — the picker

## Picker

| Key | Action |
|---|---|
| Enter | Launch the app / resume the session in this window |
| Alt-Enter | Launch / resume in a separate window |
| Tab | Switch between apps and the highlighted agent's sessions |
| Ctrl-D | Make the highlighted agent the default |
| Ctrl-R | Refresh the highlighted agent's usage |
| Ctrl-A / Ctrl-X | Add / remove an app |
| Esc | Close |

The right pane shows plan usage (session/weekly limits, balance) and recent
sessions for the highlighted app.

## How it works

- **Launching** goes through Omarchy's own `omarchy-agent`, so agents get the
  same auto-permission flags, account pinning, working directory (Omarchy's
  `~/Work` convention) and
  `org.omarchy.agent` window class. A small PATH shim
  (`~/.local/share/omarchy-agent-launcher/shim`) makes `omarchy-agent` run the
  picked agent instead of the default, and appends resume flags; the shim is
  removed from PATH before the agent's terminal starts.
- **Usage** is read (never written) from the records the bar's agents plugin
  watches: `~/.local/state/omarchy/agents/usage/<id>.json`, produced by
  `omarchy-agent-usage-update`.
- **Sessions** are read-only from each agent's store:
  Claude `~/.claude/projects`, Pi `~/.pi/agent/sessions`, omp
  `~/.omp/agent/sessions`, Hermes `~/.hermes/state.db`, Antigravity
  `~/.gemini/antigravity-cli/conversation_summaries.db`.
- The picker opens tiled with the agent window class, so when Enter turns
  it into the agent, agent window rules and theming already apply.
- Output of separately launched (Alt-Enter) terminals goes to
  `~/.local/state/omarchy-agent-launcher/launch.log`.

## Files

| Path | Purpose |
|---|---|
| `bin/omarchy-agent-launcher` | Picker, launch/resume, add/remove (bash + fzf/gum) |
| `lib/agent_launcher.py` | Rows, usage meters, session discovery, preview |
| `pkgbuild/` | Arch package: `PKGBUILD`, `.desktop` launcher entry, icon |
| `bin/install` | Builds and installs the package (`makepkg -fsi`) |
| `~/.config/omarchy/agent-launcher.json` | Your app list (created on first run) |

Installed, the package puts the command in `/usr/bin`, the helper in
`/usr/lib/omarchy-agent-launcher/`, and an **Agent Launcher** entry in the app
launcher (Super+Space). Nothing under `/usr/share/omarchy` is modified.

## Install

    git clone https://github.com/viniemm/omarchy-agent-launcher
    cd omarchy-agent-launcher
    bin/install

Remove with `sudo pacman -R omarchy-agent-launcher`.

Keybindings are yours to choose; add them to `~/.config/hypr/bindings.lua`:

    o.rebind("SUPER + A", "Agent", "omarchy-agent-launcher default")
    o.rebind("SUPER + SHIFT + A", "Agent picker", "omarchy-agent-launcher open")

(Super+Shift+A replaces Omarchy's default ChatGPT web app binding.)

## Development

`bin/omarchy-agent-launcher` runs straight from a checkout (it uses `lib/`
next to it). Re-run `bin/install` to update the installed package.
