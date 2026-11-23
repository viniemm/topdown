"""Data side of omarchy-agent-launcher: picker rows, usage preview, and session discovery.

Usage comes from the records omarchy-agent-usage-update writes for the bar's
agents plugin (~/.local/state/omarchy/agents/usage/<id>.json); this only reads
them. Sessions are read straight from each agent's own store, read-only.

Row keys (first tab-separated field; fzf shows the rest):
  app|<id>                         an app from agent-launcher.json
  add|                             the "Add app" row
  hdr|  more|                      section header / "more conversations" row
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
CACHE_PATH = os.path.join(os.environ.get("XDG_CACHE_HOME", os.path.join(HOME, ".cache")),
                          "omarchy-agent-launcher/sessions.json")
CACHE_VERSION = 2
PAGE = 30

# Usage record ids that differ from the Omarchy agent id; others use the agent id.
USAGE_IDS = {"cursor-agent": "cursor", "mcode": "minimax", "dsh": "deepseek", "ori": "openrouter"}

R, B, D = "\033[0m", "\033[1m", "\033[2m"
GREEN, YELLOW, RED, CYAN, MAG = "\033[32m", "\033[33m", "\033[31m", "\033[36m", "\033[35m"


# ---------- config ----------

def load_apps():
    try:
        return json.load(open(LIST))
    except (OSError, ValueError):
        return []


def app(app_id):
    return next((a for a in load_apps() if a.get("id") == app_id), None)


def app_from_key(key):
    """The app behind an `app|<id>` row key, or a bare id."""
    return app(key.removeprefix("app|")) if "|" not in key.removeprefix("app|") else None


def agent_label(agent, apps=None):
    return next((a["label"] for a in (apps or load_apps()) if a.get("agent") == agent), agent)


def current_default():
    try:
        return open(DEFAULT_FILE).readline().strip()
    except OSError:
        return ""


def read_kv(path):
    """key=value lines: the picker's state, which the bash side sources on every keypress."""
    out = {}
    try:
        for line in open(path) if path else []:
            k, _, v = line.strip().partition("=")
            if k:
                out[k] = v
    except OSError:
        pass
    return out


def write_kv(path, state):
    if path:
        with open(path + ".tmp", "w") as f:
            f.writelines(f"{k}={v}\n" for k, v in state.items())
        os.replace(path + ".tmp", path)


# ---------- formatting ----------

def epoch(v):
    """Seconds since the epoch from seconds or milliseconds (stores differ); 0 if unusable."""
    try:
        v = float(v or 0)
    except (TypeError, ValueError):
        return 0.0
    return v / 1000 if v > 1e11 else v


def parse_time(v):
    if v in (None, ""):
        return None
    if isinstance(v, (int, float)):
        return dt.datetime.fromtimestamp(epoch(v), dt.timezone.utc)
    s = str(v).strip().replace("Z", "+00:00").replace(" ", "T", 1)
    if "." in s:  # trim nanoseconds to microseconds
        head, _, tail = s.partition(".")
        frac = "".join(c for c in tail if c.isdigit())
        s = f"{head}.{frac[:6]}{tail[len(frac):]}"
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


def ago(ts):
    return span(dt.datetime.now().timestamp() - ts) + " ago" if ts else "?"


def stamp(ts):
    """Compact local time: 21:40 today, 'Yest 14:02', 'Mon 09:10' this week, 'Nov 18' older."""
    try:
        t = dt.datetime.fromtimestamp(ts) if ts else None
    except (OverflowError, OSError, ValueError):
        t = None
    if not t:
        return ""
    now = dt.datetime.now()
    days = (now.date() - t.date()).days
    if days == 0:
        return t.strftime("%H:%M")
    if days == 1:
        return t.strftime("Yest %H:%M")
    if days < 7:
        return t.strftime("%a %H:%M")
    return t.strftime("%b %d" if t.year == now.year else "%b %d %Y")


def short_path(p):
    return "~" + p[len(HOME):] if p and p.startswith(HOME) else (p or "")


def project(p):
    p = (p or "").rstrip("/")
    return "~" if p == HOME else os.path.basename(p) or p


def oneline(s, n):
    s = " ".join(str(s or "").split())
    return s if len(s) <= n else s[: n - 1] + "…"


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


# ---------- usage ----------

def usage_record(a):
    uid = a and (a.get("usage") or USAGE_IDS.get(a.get("agent", ""), a.get("agent", "")))
    if not uid:
        return None
    try:
        return json.load(open(os.path.join(USAGE_DIR, f"{uid}.json")))
    except (OSError, ValueError):
        return None


def mini_meter(a):
    """A 6-cell meter of the tightest limit, so the list shows headroom at a glance."""
    limits = (usage_record(a) or {}).get("limits") or []
    return bar(max(float(l.get("percent") or 0) for l in limits), 6) if limits else " " * 6


def render_usage(a, width):
    rec = usage_record(a)
    if rec is None:
        return [f"{D}No plan limits to track — this is a harness or a plain TUI.{R}"]
    limits, balance, tier = rec.get("limits") or [], rec.get("balance"), rec.get("tierLabel") or ""
    out = [f"{B}Usage{R}" + (f"  {MAG}{tier}{R}" if tier else "")]
    if not limits and not balance:
        status, help_ = rec.get("usageStatusText") or "", rec.get("authHelpText") or ""
        if help_ or status:
            out.append(f"{YELLOW}{status or 'No limits yet'}{R}")
            if help_:
                out.append(f"{D}{oneline(help_, width * 3)}{R}")
        else:
            out.append(f"{D}No plan limits — billed through whatever provider it's pointed at.{R}")
    bw = max(10, min(28, width - 26))
    now = dt.datetime.now(dt.timezone.utc)
    for l in limits:
        p = float(l.get("percent") or 0)
        label = l.get("title") or (l.get("label") or "Limit").split(" (")[0]
        reset = parse_time(l.get("resetsAt"))
        left = "" if not reset else f"resets {span((reset - now).total_seconds())}" if reset > now else "reset due"
        out.append(f"{label:<10} {bar(p, bw)} {p * 100:4.0f}%  {D}{left}{R}")
    if balance:
        rem, funded = balance.get("remaining") or 0, balance.get("funded") or 0
        spent = 1 - rem / funded if funded else 0
        out.append(f"{'Balance':<10} {bar(spent, bw)} {rem:.2f} {balance.get('currency', 'USD')} left of {funded:.2f}")
    if limits and rec.get("limitsStale"):
        why = rec.get("authHelpText") or rec.get("usageStatusText") or ""
        out.append(f"{YELLOW}⚠ limits not current{R} {D}{oneline(why, width * 2)}{R}")
    if rec.get("hasLocalStats") or rec.get("todayPrompts"):
        out.append(f"{D}Today{R} {rec.get('todayPrompts', 0)} prompts · {rec.get('todaySessions', 0)} sessions · "
                   f"{tokens(rec.get('todayTotalTokens'))} tokens   {D}All time{R} {rec.get('totalPrompts', 0)} prompts")
    updated = parse_time(rec.get("updatedAt"))
    out.append(f"{D}updated {ago(updated.timestamp() if updated else 0)} · Ctrl-R refresh{R}")
    return out


# ---------- session stores ----------
#
# Each agent's sessions are read from its own store, read-only. Discovery is
# two-phase so the list can grow as you scroll: candidates are cheap (file
# mtimes, newest-first database rows) and only those about to be shown are
# parsed. Parsed file sessions are cached by path, mtime and size.

def text_of(content):
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(c.get("text", "") for c in content
                        if isinstance(c, dict) and c.get("type") in ("text", "input_text"))
    return ""


def is_user_text(t):
    """Something the user typed, not a harness insert (<command-…> tags, Claude's 'Caveat:' notes)."""
    t = (t or "").lstrip()
    return bool(t) and not t.startswith(("<", "Caveat:"))


def parse_claude(path):
    cwd = custom = ai = ""
    prompts = []
    with open(path, encoding="utf-8", errors="replace") as f:
        for line in f:
            # Cheap substring checks first; only promising lines are JSON-decoded.
            i = line.find('"cwd":"')
            if i != -1 and (j := line.find('"', i + 7)) != -1:
                cwd = line[i + 7:j]  # the latest wins: a moved session resumes where it now lives
            if '-title"' in line[:40] or (len(prompts) < 5 and '"type":"user"' in line):
                try:
                    d = json.loads(line)
                except ValueError:
                    continue
                kind = d.get("type")
                if kind == "custom-title":
                    custom = d.get("customTitle") or custom
                elif kind == "ai-title":
                    ai = d.get("aiTitle") or ai
                elif kind == "user" and not d.get("isMeta"):
                    t = text_of((d.get("message") or {}).get("content"))
                    if is_user_text(t):
                        prompts.append(t.strip()[:600])
    return {"title": custom or ai or (prompts[0] if prompts else ""), "cwd": cwd, "prompts": prompts}


def parse_pi(path):
    """Pi and Oh My Pi share a session format."""
    cwd = title = ""
    prompts = []
    with open(path, encoding="utf-8", errors="replace") as f:
        for n, line in enumerate(f):
            if n >= 300:
                break
            try:
                d = json.loads(line)
            except ValueError:
                continue
            kind = d.get("type")
            if kind == "title":
                title = d.get("title", "")
            elif kind == "session":
                cwd = d.get("cwd", "")
            elif kind == "message" and (d.get("message") or {}).get("role") == "user" and len(prompts) < 5:
                t = text_of(d["message"].get("content"))
                if is_user_text(t):
                    prompts.append(t.strip()[:600])
    return {"title": title or (prompts[0] if prompts else ""), "cwd": cwd, "prompts": prompts}


def ro_db(path):
    return sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=1)


def hermes_sessions(limit, sid=None):
    path = os.path.join(HOME, ".hermes/state.db")
    if not os.path.exists(path):
        return []
    q = ("select s.id, s.cwd, s.title, coalesce(s.last_activity_at, s.started_at),"
         " (select content from messages m where m.session_id = s.id and m.role = 'user' order by m.id limit 1)"
         " from sessions s where coalesce(s.hidden,0) = 0 and coalesce(s.archived,0) = 0"
         " and s.source in ('cli','tui','local') and s.message_count > 0"
         + (" and s.id = ?" if sid else "") + " order by 4 desc limit ?")
    try:
        rows = ro_db(path).execute(q, (sid, 1) if sid else (limit,)).fetchall()
    except sqlite3.Error:
        return []
    return [{"agent": "hermes", "id": i, "cwd": cwd or "", "title": title or first, "when": epoch(when),
             "prompts": [first] if first else []}
            for i, cwd, title, when, first in rows if title or is_user_text(first)]


def agy_sessions(limit, sid=None):
    path = os.path.join(HOME, ".gemini/antigravity-cli/conversation_summaries.db")
    if not os.path.exists(path):
        return []
    q = ("select conversation_id, title, preview, last_modified_time, workspace_uris from conversation_summaries"
         + (" where conversation_id = ?" if sid else "") + " order by last_modified_time desc limit ?")
    try:
        rows = ro_db(path).execute(q, (sid, 1) if sid else (limit,)).fetchall()
    except sqlite3.Error:
        return []
    out = []
    for cid, title, preview, when, ws in rows:
        if not (title or preview):
            continue
        try:
            uri = (json.loads(ws) or [""])[0]
        except ValueError:
            uri = ""
        t = parse_time(when)
        out.append({"agent": "agy", "id": cid, "cwd": uri.removeprefix("file://"), "title": title or preview,
                    "when": t.timestamp() if t else 0, "prompts": [preview] if preview else []})
    return out


# Everything agent-specific about sessions: where they live and how to resume one.
#   files: glob under $HOME, parsed with `parse`; the session id is the file stem
#          ("stem") or its full path ("path")
#   query: a function(limit, sid=None) returning session dicts
#   resume: arguments appended to the agent's usual command ({id} = session id)
SOURCES = {
    "claude": {"files": ".claude/projects/*/*.jsonl", "parse": parse_claude, "id": "stem",
               "resume": ["--resume", "{id}"]},
    "pi": {"files": ".pi/agent/sessions/*/*.jsonl", "parse": parse_pi, "id": "path",
           "resume": ["--session", "{id}"]},
    "omp": {"files": ".omp/agent/sessions/*/*.jsonl", "parse": parse_pi, "id": "path",
            "resume": ["--resume", "{id}"]},
    "hermes": {"query": hermes_sessions, "resume": ["--resume", "{id}"]},
    "agy": {"query": agy_sessions, "resume": ["--conversation", "{id}"]},
}


class Cache:
    """Parsed file sessions, keyed by path and invalidated by mtime/size."""

    def __init__(self):
        try:
            data = json.load(open(CACHE_PATH))
        except (OSError, ValueError):
            data = {}
        self.data = data.get("sessions", {}) if data.get("version") == CACHE_VERSION else {}
        self.dirty = False

    def lookup(self, path, st, parse):
        e = self.data.get(path)
        if not e or e.get("mtime") != st.st_mtime or e.get("size") != st.st_size:
            e = dict(parse(path), mtime=st.st_mtime, size=st.st_size)
            self.data[path], self.dirty = e, True
        return e

    def save(self):
        if not self.dirty:
            return
        # Forget files that no longer exist so the cache can't grow forever.
        data = {"version": CACHE_VERSION, "sessions": {k: v for k, v in self.data.items() if os.path.exists(k)}}
        os.makedirs(os.path.dirname(CACHE_PATH), exist_ok=True)
        with open(CACHE_PATH + ".tmp", "w") as f:
            json.dump(data, f)
        os.replace(CACHE_PATH + ".tmp", CACHE_PATH)
        self.dirty = False


def file_session(agent, path, cache, st=None):
    src = SOURCES[agent]
    try:
        info = cache.lookup(path, st or os.stat(path), src["parse"])
    except OSError:
        return None
    if not info.get("title"):
        return None
    sid = os.path.basename(path)[:-6] if src["id"] == "stem" else path
    return {"agent": agent, "id": sid, "cwd": info.get("cwd", ""), "title": info["title"],
            "when": info["mtime"], "prompts": info.get("prompts", [])}


def recent_sessions(agents, limit, cache):
    """Yield up to `limit` sessions across `agents`, newest first."""
    cands = []  # (when, agent, path or None, stat or session)
    for agent in agents:
        src = SOURCES.get(agent)
        try:
            if src and "files" in src:
                for path in glob.glob(os.path.join(HOME, src["files"])):
                    try:
                        st = os.stat(path)
                    except OSError:
                        continue
                    cands.append((st.st_mtime, agent, path, st))
            elif src:
                cands += [(s["when"], agent, None, s) for s in src["query"](limit)]
        except Exception:  # a store changing shape must never break the picker
            continue
    cands.sort(key=lambda c: c[0], reverse=True)
    shown = 0
    for _, agent, path, extra in cands:
        if shown >= limit:
            return
        s = file_session(agent, path, cache, extra) if path else extra
        if s:
            shown += 1
            yield s


def session_by_id(agent, sid):
    src = SOURCES.get(agent)
    if not src:
        return None
    if "query" in src:
        return next(iter(src["query"](1, sid)), None)
    if src["id"] == "path":
        path = sid
    else:
        pattern = os.path.join(HOME, os.path.dirname(src["files"]), glob.escape(sid) + ".jsonl")
        path = next(iter(glob.glob(pattern)), None)
    return file_session(agent, path, Cache()) if path else None


# ---------- rows ----------

def session_row(s, label, lw):
    return (f"ses|{s['agent']}|{s['id']}|{s['cwd']}\t"
            f"  {D}{stamp(s['when']):<10}{R} {label:<{lw}.{lw}}  {CYAN}{project(s['cwd']):<12.12}{R} "
            f"{oneline(s['title'], 200)}")


def print_sessions(agents, labels, limit):
    """Stream up to `limit` conversations; return whether older ones remain."""
    lw = max([len(labels.get(a, a)) for a in agents] + [6])
    cache = Cache()
    shown = 0
    more = False
    for s in recent_sessions(agents, limit + 1, cache):  # one extra tells us if there's more
        if shown == limit:
            more = True
            break
        print(session_row(s, labels.get(s["agent"], s["agent"]), lw), flush=True)
        shown += 1
    cache.save()
    if more:
        print(f"more|\t{D}   ↓ more conversations…{R}")
    elif not shown:
        print(f"hdr|\t{D}   no conversations yet{R}")
    return more


def cmd_rows(*args):
    """The picker list.

    mode "all": apps, then 'Recent conversations' across the listed agents.
    mode <agent>: just that agent's conversations (Right on its app row).

    --state BASE: the picker owns BASE.nav (mode, and limit = how many
    conversations to show, grown as you scroll); this writes BASE.rows
    (has_more, and first_session for Tab). Two files, so neither side ever
    overwrites the other's values.
    """
    base = args[1] if len(args) > 1 and args[0] == "--state" else ""
    nav = read_kv(base and base + ".nav")
    limit = int(nav.get("limit") or PAGE)
    mode = nav.get("mode") or "all"
    apps = load_apps()
    labels = {a["agent"]: a["label"] for a in apps if a.get("agent")}
    more = False

    if mode == "all":
        default = current_default()
        w = max([len(a["label"]) for a in apps] + [10])
        for a in apps:
            mark = "★" if a.get("agent") and a["agent"] == default else " "
            kind, runs = ("agent", a["agent"]) if a.get("agent") else ("tui", a.get("command", ""))
            print(f"app|{a['id']}\t{mark} {a['label']:<{w}}  {mini_meter(a)}\t{D}{kind}: {runs}{R}")
        print(f"add|\t+ {'Add app…':<{w}}  {'':6}\t{D}Ctrl-A{R}")
        first_session = len(apps) + 3
        agents = [a for a in labels if a in SOURCES]
        if agents:
            print(f"hdr|\t{D}──── Recent conversations ─────────────────────────{R}", flush=True)
            more = print_sessions(agents, labels, limit)
    else:
        label = labels.get(mode, mode)
        print(f"hdr|\t{D}← {R}{B}{label}{R}{D} conversations ─────────────── Left: back · Tab: all{R}", flush=True)
        first_session = 2
        more = print_sessions([mode], labels, limit)

    write_kv(base and base + ".rows", {"first_session": first_session, "has_more": int(more)})


# ---------- preview and small queries ----------

def cmd_preview(key):
    width = int(os.environ.get("FZF_PREVIEW_COLUMNS", "60"))
    kind, _, rest = key.partition("|")
    if kind == "add":
        print(f"{B}Add an app{R}\n\nInstalled agents Omarchy knows how to launch, or any command\n"
              "to run in a terminal (lazygit, aider, …).")
    elif kind in ("hdr", "more"):
        print(f"{B}Recent conversations{R}\n\nEvery listed agent's sessions, newest first.\n"
              f"More load as you scroll; typing searches a larger set.\n\n"
              f"{D}Enter resumes in its project · Tab jumps between apps and conversations{R}")
    elif kind == "app" and (a := app(rest)):
        star = f"  {YELLOW}★ default{R}" if a.get("agent") and a["agent"] == current_default() else ""
        what = f"agent: {a['agent']}" if a.get("agent") else f"command: {a.get('command', '')}"
        print(f"{B}{a['label']}{R}{star}\n{D}{what}{R}\n")
        print("\n".join(render_usage(a, width)))
        agent = a.get("agent", "")
        if agent in SOURCES:
            cache = Cache()
            rows = list(recent_sessions([agent], 5, cache))
            cache.save()
            print(f"\n{B}Latest{R}")
            for s in rows:
                print(f"{D}{stamp(s['when']):<10}{R} {CYAN}{project(s['cwd']):<12.12}{R} "
                      f"{oneline(s['title'], max(10, width - 25))}")
            if not rows:
                print(f"{D}none yet{R}")
    elif kind == "ses":
        agent, sid = (rest.split("|", 2) + ["", ""])[:2]
        s = session_by_id(agent, sid)
        if not s:
            return
        print(f"{B}{oneline(s['title'], width * 2)}{R}")
        print(f"{MAG}{agent_label(agent)}{R}  {CYAN}{short_path(s['cwd'])}{R}  "
              f"{D}{stamp(s['when'])} · {ago(s['when'])}{R}\n")
        print(f"{D}Enter resumes it there · Alt-Enter in a new window{R}\n")
        for p in s["prompts"][:5]:
            print(f"{MAG}›{R} {oneline(p, width * 3 - 2)}\n")


def cmd_title(key):
    """Window title for what's being launched: an app id, app|id, or a session row."""
    if key.startswith("ses|"):
        agent, sid = (key.split("|") + ["", ""])[1:3]
        s = session_by_id(agent, sid)
        print(f"{agent_label(agent)} — {oneline(s['title'], 60)}" if s else agent_label(agent))
    else:
        a = app_from_key(key)
        print(a["label"] if a else "Agent")


def cmd_usage_id(key):
    a = app_from_key(key)
    if a:
        print(a.get("usage") or USAGE_IDS.get(a.get("agent", ""), a.get("agent", "")))


def cmd_session_agent(key):
    """The agent behind an app row, if it has browsable sessions (for Right)."""
    a = app_from_key(key)
    if a and a.get("agent") in SOURCES:
        print(f"{a['agent']}\t{a['label']}")


def cmd_resume_args(agent, sid):
    """One argument per line: what to append to the agent's command to resume `sid`."""
    src = SOURCES.get(agent)
    if not src:
        sys.exit(f"Can't resume {agent} sessions")
    print("\n".join(arg.replace("{id}", sid) for arg in src["resume"]))


def main():
    cmd, *args = sys.argv[1:] or ["rows"]
    {"rows": cmd_rows, "preview": cmd_preview, "title": cmd_title, "usage-id": cmd_usage_id,
     "session-agent": cmd_session_agent, "resume-args": cmd_resume_args}[cmd](*args)


if __name__ == "__main__":
    main()
