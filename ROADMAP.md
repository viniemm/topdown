# topdown roadmap

topdown is growing from an Omarchy agent launcher (v0.4.4, bash + fzf + Python)
into a general TUI harness for coding agents, built on omp's stack: TypeScript
on Bun with [`@oh-my-pi/pi-tui`](https://www.npmjs.com/package/@oh-my-pi/pi-tui)
(MIT), pinned to an exact version.

## M1 — parity on the new stack
- Hub screen on pi-tui's `HubFrame` (omp's model-picker look): sidebar of
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
- `bun build --compile` gives one ~114 MB binary. pi-tui needs omp's native
  addon (`pi_natives.linux-x64-{modern,baseline}.node`, ~143 MB each), loaded
  from beside the executable. omp itself is ~301 MB, so this is the same class.
  Ship the `modern` variant, plus `baseline` for older CPUs.
