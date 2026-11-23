"""Data side of omarchy-agent-launcher: picker rows, usage preview, and session discovery.

Usage comes from the records omarchy-agent-usage-update writes for the bar's
agents plugin (~/.local/state/omarchy/agents/usage/<id>.json); this only reads
them. Sessions are read straight from each agent's own store, read-only.

Row format (tab-separated, fzf shows fields 2+):
  app|<id>                         an app from agent-launcher.json
  add|                             the "Add app" row
  ses|<agent>|<session>|<cwd>      a resumable session
"""

import datetime as dt
import glob
import json
import os
import sqlite3
import sys

HOME = os.path.expanduser("~")
LIST = os.path.join(HOME, ".config/omarchy/agent-launcher.json")
DEFAULT_FILE = os.path.join(HOME, ".config/omarchy/defaults/agent")
USAGE_DIR = os.path.join(os.environ.get("XDG_STATE_HOME", os.path.join(HOME, ".local/state")), "omarchy/agents/usage")

# Omarchy agent id -> usage record id, where they differ or exist at all.
USAGE_IDS = {
    "claude": "claude", "codex": "codex", "grok": "grok", "agy": "agy", "copilot": "copilot",
    "cursor-agent": "cursor", "kimi": "kimi", "qwen": "qwen", "muse": "muse", "opencode": "opencode",
    "hermes": "hermes", "mcode": "minimax", "dsh": "deepseek", "ori": "openrouter", "devin": "devin",
}

# ANSI
R, B, D = "\033[0m", "\033[1m", "\033[2m"
GREEN, YELLOW, RED, CYAN, MAG = "\033[32m", "\033[33m", "\033[31m", "\033[36m", "\033[35m"


def load_apps():
    try:
        return json.load(open(LIST))
    except (OSError, ValueError):
        return []


def app(app_id):
    return next((a for a in load_apps() if a.get("id") == app_id), None)


def current_default():
    try:
        return open(DEFAULT_FILE).readline().strip()
    except OSError:
        return ""


# ---------- time helpers ----------

def parse_time(v):
    if v in (None, ""):
        return None
    if isinstance(v, (int, float)):
        return dt.datetime.fromtimestamp(v / 1000 if v > 1e11 else v, dt.timezone.utc)
    s = str(v).strip().replace("Z", "+00:00").replace(" ", "T", 1)
    if "." in s:  # trim nanoseconds to micro
        head, _, tail = s.partition(".")
        frac = "".join(c for c in tail if c.isdigit())
        tz = tail[len(frac):]
        s = f"{head}.{frac[:6]}{tz}"
    try:
        t = dt.datetime.fromisoformat(s)
    except ValueError:
        return None
    return t if t.tzinfo else t.replace(tzinfo=dt.timezone.utc)


def span(seconds):
    seconds = int(abs(seconds))
    if seconds < 60:
        return f"{seconds}s"
    m = seconds // 60
    if m < 60:
        return f"{m}m"
    h, m = divmod(m, 60)
    if h < 24:
        return f"{h}h {m:02d}m" if m else f"{h}h"
    d, h = divmod(h, 24)
    return f"{d}d {h}h" if h else f"{d}d"


def ago(t):
    if not t:
        return "?"
    return span((dt.datetime.now(dt.timezone.utc) - t).total_seconds()) + " ago"


def short_path(p):
    if not p:
        return ""
    return "~" + p[len(HOME):] if p.startswith(HOME) else p


def project(p):
    p = (p or "").rstrip("/")
    if p == HOME:
        return "~"
    return os.path.basename(p) or p


def oneline(s, n):
    s = " ".join(str(s or "").split())
    return s if len(s) <= n else s[: n - 1] + "…"


# ---------- rows ----------

def cmd_rows():
    apps = load_apps()
    default = current_default()
    w = max([len(a["label"]) for a in apps] + [10])
    for a in apps:
        mark = "★" if a.get("agent") and a["agent"] == default else " "
        kind = "agent" if a.get("agent") else "tui"
        runs = a.get("agent") or a.get("command", "")
        meter = mini_meter(a)
        print(f"app|{a['id']}\t{mark} {a['label']:<{w}}  {meter}\t{D}{kind}: {runs}{R}")
    print(f"add|\t+ {'Add app…':<{w}}  {'':6}\t{D}Ctrl-A{R}")


def mini_meter(a):
    """A 6-cell meter of the tightest limit, so the list shows headroom at a glance."""
    rec = usage_record(a)
    limits = (rec or {}).get("limits") or []
    if not limits:
        return " " * 6
    p = max(float(l.get("percent") or 0) for l in limits)
    return bar(p, 6)


# ---------- usage ----------

def usage_record(a):
    if not a:
        return None
    uid = a.get("usage") or USAGE_IDS.get(a.get("agent", ""))
    if not uid:
        return None
    try:
        return json.load(open(os.path.join(USAGE_DIR, f"{uid}.json")))
    except (OSError, ValueError):
        return None


def bar(p, width):
    p = max(0.0, min(1.0, p))
    filled = round(p * width)
    color = GREEN if p < 0.6 else YELLOW if p < 0.85 else RED
    return f"{color}{'█' * filled}{R}{D}{'░' * (width - filled)}{R}"


def tokens(n):
    n = int(n or 0)
    for unit, size in (("B", 1e9), ("M", 1e6), ("k", 1e3)):
        if n >= size:
            return f"{n / size:.1f}{unit}"
    return str(n)


def render_usage(a, width):
    rec = usage_record(a)
    out = []
    if rec is None:
        out.append(f"{D}No plan limits to track — this is a harness or a plain TUI.{R}")
        return out
    limits = rec.get("limits") or []
    balance = rec.get("balance")
    tier = rec.get("tierLabel") or ""
    head = f"{B}Usage{R}" + (f"  {MAG}{tier}{R}" if tier else "")
    out.append(head)
    if not limits and not balance:
        status = rec.get("usageStatusText") or ""
        help_ = rec.get("authHelpText") or ""
        if help_ or status:
            out.append(f"{YELLOW}{status or 'No limits yet'}{R}")
            if help_:
                out.append(f"{D}{oneline(help_, width * 3)}{R}")
        else:
            out.append(f"{D}No plan limits — billed through whatever provider it's pointed at.{R}")
    bw = max(10, min(28, width - 26))
    for l in limits:
        p = float(l.get("percent") or 0)
        label = l.get("title") or (l.get("label") or "Limit").split(" (")[0]
        reset = parse_time(l.get("resetsAt"))
        left = ""
        if reset:
            secs = (reset - dt.datetime.now(dt.timezone.utc)).total_seconds()
            left = f"resets {span(secs)}" if secs > 0 else "reset due"
        out.append(f"{label:<10} {bar(p, bw)} {p * 100:4.0f}%  {D}{left}{R}")
    if balance:
        rem, funded = balance.get("remaining") or 0, balance.get("funded") or 0
        cur = balance.get("currency", "USD")
        spent = 1 - (rem / funded) if funded else 0
        out.append(f"{'Balance':<10} {bar(spent, bw)} {rem:.2f} {cur} left of {funded:.2f}")
    if limits and rec.get("limitsStale"):
        out.append(f"{YELLOW}⚠ limits not current{R} {D}{oneline(rec.get('authHelpText') or rec.get('usageStatusText') or '', width * 2)}{R}")
    if rec.get("hasLocalStats") or rec.get("todayPrompts"):
        out.append(
            f"{D}Today{R} {rec.get('todayPrompts', 0)} prompts · {rec.get('todaySessions', 0)} sessions · "
            f"{tokens(rec.get('todayTotalTokens'))} tokens   {D}All time{R} {rec.get('totalPrompts', 0)} prompts"
        )
    updated = parse_time(rec.get("updatedAt"))
    out.append(f"{D}updated {ago(updated)} · Ctrl-R refresh{R}")
    return out


# ---------- sessions ----------

def text_of(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(c.get("text", "") for c in content if isinstance(c, dict) and c.get("type") in ("text", "input_text"))
    return ""


def first_prompt(texts):
    for t in texts:
        t = (t or "").strip()
        if t and not t.startswith("<") and not t.startswith("Caveat:"):
            return t
    return ""


def jsonl_head(path, limit=400):
    out = []
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            for i, line in enumerate(f):
                if i >= limit:
                    break
                try:
                    out.append(json.loads(line))
                except ValueError:
                    pass
    except OSError:
        pass
    return out


def recent_files(pattern, n):
    files = glob.glob(pattern)
    files.sort(key=lambda p: os.path.getmtime(p), reverse=True)
    return files[:n]


def claude_meta(path):
    """(title, cwd): the /rename title, else Claude's AI title, and the latest
    working directory recorded — so a session moved to another project folder
    resumes where it now lives."""
    custom = ai = cwd = ""
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                i = line.find('"cwd":"')
                if i != -1:
                    j = line.find('"', i + 7)
                    if j != -1:
                        cwd = line[i + 7:j]
                if '-title"' not in line[:40]:
                    continue
                try:
                    d = json.loads(line)
                except ValueError:
                    continue
                if d.get("type") == "custom-title":
                    custom = d.get("customTitle") or custom
                elif d.get("type") == "ai-title":
                    ai = d.get("aiTitle") or ai
    except OSError:
        pass
    return custom or ai, cwd


def sessions_claude(n):
    for path in recent_files(os.path.join(HOME, ".claude/projects/*/*.jsonl"), n):
        cwd, prompts = "", []
        for d in jsonl_head(path):
            if d.get("type") == "user" and not d.get("isMeta"):
                cwd = cwd or d.get("cwd", "")
                prompts.append(text_of((d.get("message") or {}).get("content")))
                if len(prompts) > 4:
                    break
        title, last_cwd = claude_meta(path)
        title = title or first_prompt(prompts)
        cwd = last_cwd or cwd
        if not title:
            continue
        yield {"agent": "claude", "id": os.path.basename(path)[:-6], "cwd": cwd,
               "title": title, "when": os.path.getmtime(path), "path": path, "prompts": prompts}


def sessions_pi_like(agent, root, n):
    for path in recent_files(os.path.join(root, "*/*.jsonl"), n):
        cwd, title, prompts = "", "", []
        for d in jsonl_head(path, 200):
            t = d.get("type")
            if t == "title":
                title = d.get("title", "")
            elif t == "session":
                cwd = d.get("cwd", "")
            elif t == "message" and (d.get("message") or {}).get("role") == "user":
                prompts.append(text_of(d["message"].get("content")))
                if len(prompts) > 4:
                    break
        title = title or first_prompt(prompts)
        if not title:
            continue
        yield {"agent": agent, "id": path, "cwd": cwd, "title": title,
               "when": os.path.getmtime(path), "path": path, "prompts": prompts}


def ro_db(path):
    return sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=1)


def sessions_hermes(n):
    path = os.path.join(HOME, ".hermes/state.db")
    if not os.path.exists(path):
        return
    try:
        db = ro_db(path)
        rows = db.execute(
            "select s.id, s.cwd, s.title, coalesce(s.last_activity_at, s.started_at),"
            " (select content from messages m where m.session_id = s.id and m.role = 'user' order by m.id limit 1)"
            " from sessions s where coalesce(s.hidden,0) = 0 and coalesce(s.archived,0) = 0"
            " and s.source in ('cli','tui','local') and s.message_count > 0"
            " order by 4 desc limit ?", (n,)).fetchall()
    except sqlite3.Error:
        return
    for sid, cwd, title, when, first in rows:
        t = title or first_prompt([first])
        if t:
            yield {"agent": "hermes", "id": sid, "cwd": cwd or "", "title": t,
                   "when": float(when or 0), "prompts": [first or ""]}


def sessions_agy(n):
    path = os.path.join(HOME, ".gemini/antigravity-cli/conversation_summaries.db")
    if not os.path.exists(path):
        return
    try:
        rows = ro_db(path).execute(
            "select conversation_id, title, preview, last_modified_time, workspace_uris"
            " from conversation_summaries order by last_modified_time desc limit ?", (n,)).fetchall()
    except sqlite3.Error:
        return
    for cid, title, preview, when, ws in rows:
        try:
            uri = (json.loads(ws) or [""])[0]
        except ValueError:
            uri = ""
        t = parse_time(when)
        yield {"agent": "agy", "id": cid, "cwd": uri.removeprefix("file://"), "title": title or preview,
               "when": t.timestamp() if t else 0, "prompts": [preview]}


SESSION_SOURCES = {
    "claude": sessions_claude,
    "pi": lambda n: sessions_pi_like("pi", os.path.join(HOME, ".pi/agent/sessions"), n),
    "omp": lambda n: sessions_pi_like("omp", os.path.join(HOME, ".omp/agent/sessions"), n),
    "hermes": sessions_hermes,
    "agy": sessions_agy,
}


def sessions_for(agent, n=40):
    src = SESSION_SOURCES.get(agent)
    if not src:
        return []
    try:
        return list(src(n))
    except Exception:  # a store changing shape must never break the picker
        return []


def session_label(s, width=60):
    when = dt.datetime.fromtimestamp(s["when"], dt.timezone.utc) if s["when"] else None
    age = ago(when).replace(" ago", "")
    return f"{CYAN}{project(s['cwd']):<12.12}{R} {D}{age:>7}{R}  {oneline(s['title'], width)}", ""


def cmd_sessions(key):
    a = app(key.split("|", 1)[1]) if key.startswith("app|") else None
    agent = (a or {}).get("agent", "")
    rows = sessions_for(agent)
    if not rows:
        print(f"app|{(a or {}).get('id', '')}\t{D}No resumable sessions for {(a or {}).get('label', 'this app')} — Tab to go back{R}\t")
        return
    for s in rows:
        label, _ = session_label(s, 200)
        print(f"ses|{agent}|{s['id']}|{s['cwd']}\t{label}")


# ---------- preview ----------

def cmd_preview(key):
    width = int(os.environ.get("FZF_PREVIEW_COLUMNS", "60"))
    kind, _, rest = key.partition("|")
    if kind == "add":
        print(f"{B}Add an app{R}\n\nInstalled agents Omarchy knows how to launch, or any command\nto run in a terminal (lazygit, aider, …).")
        return
    if kind == "app":
        a = app(rest)
        if not a:
            return
        default = current_default()
        star = f"  {YELLOW}★ default{R}" if a.get("agent") and a["agent"] == default else ""
        print(f"{B}{a['label']}{R}{star}")
        print(f"{D}{'agent: ' + a['agent'] if a.get('agent') else 'command: ' + a.get('command', '')}{R}\n")
        for line in render_usage(a, width):
            print(line)
        agent = a.get("agent", "")
        if agent in SESSION_SOURCES:
            print(f"\n{B}Recent sessions{R}  {D}Tab to browse & resume{R}")
            rows = sessions_for(agent, 8)
            for s in rows:
                label, _ = session_label(s, max(10, width - 24))
                print(label)
            if not rows:
                print(f"{D}none yet{R}")
        return
    if kind == "ses":
        agent, sid, cwd = (rest.split("|", 2) + ["", ""])[:3]
        s = next((x for x in sessions_for(agent, 200) if x["id"] == sid), None)
        if not s:
            return
        when = dt.datetime.fromtimestamp(s["when"], dt.timezone.utc) if s["when"] else None
        print(f"{B}{oneline(s['title'], width * 2)}{R}")
        print(f"{CYAN}{short_path(s['cwd'])}{R}  {D}{ago(when)}{R}\n")
        print(f"{D}Enter resumes it there · Tab back to apps{R}\n")
        for p in [x for x in s.get("prompts", []) if x and not x.startswith("<")][:5]:
            print(f"{MAG}›{R} {oneline(p, width * 3 - 2)}\n")


def cmd_usage_id(key):
    a = app(key.split("|", 1)[1]) if key.startswith("app|") else None
    if a:
        print(a.get("usage") or USAGE_IDS.get(a.get("agent", ""), ""))


def main():
    cmd, *args = sys.argv[1:] or ["rows"]
    {"rows": cmd_rows, "preview": cmd_preview, "sessions": cmd_sessions, "usage-id": cmd_usage_id}[cmd](*args)


if __name__ == "__main__":
    main()
