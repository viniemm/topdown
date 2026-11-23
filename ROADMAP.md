# topdown roadmap

topdown is growing from an Omarchy agent launcher (v0.4.4, bash + fzf + Python)
into a general TUI harness for coding agents: TypeScript on Bun, with a TUI
layer of its own modelled on omp's.

## M1 — parity on the new stack
- Hub screen in the layout of omp's model picker: sidebar of
  agents and conversation scopes, searchable list, detail line, key bar.
- Everything 0.4.4 does: launch / resume (in place or new window), recent
  conversations with paging, per-agent view, usage meters, default agent,
  add / remove apps, rename, delete (Trash), open-conversation marker and
  window switching.
- Claude Code adapter in full, with a launch sheet for its flags (`--model`,
  `--effort`, `--permission-mode`, `--add-dir`, `--agent`, `--mcp-config`,
  `--fork-session`, …).
- Pi, omp, Hermes (`--tui`), Antigravity ported from the Python helper.
- `bun test` with fixture homes.

## M2 — every agent
- Launch adapters for all agents Omarchy knows (opencode, copilot, crush,
  grok, codex, cursor-agent, muse, ori, kimi, qwen, dsh, mcode, kiro-cli,
  freebuff, kilo, devin, amp, commandcode, openclaw), then full adapters
  (sessions, resume, flags) one at a time, starting with codex and opencode.
- Works without Omarchy: Omarchy (usage records, `omarchy-agent` launch path,
  Hyprland focus) becomes an optional integration.

## M3 — MCP control
- One list of MCP servers across agents (Claude `claude mcp list` and its
  config files first), then add / remove / enable per agent.

## M4 — skills manager
- Skills across agents (`~/.claude/skills`, plugins, Pi/omp skills).

## M5 — search conversation contents
Measured on real history: grep over 108 MB of Claude transcripts takes
~25–40 ms, and Hermes already keeps a trigram full-text index
(`messages_fts_trigram`, ~40 ms). Fast enough to search as you type.

## Packaging notes
- topdown has its own TUI layer (`src/tui/`, ~800 lines) modelled on omp's
  pi-tui rather than depending on it, so there is no native addon: the
  package is one `bun build --compile` binary (the Bun runtime, ~107 MB,
  is most of it).
