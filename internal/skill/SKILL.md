---
name: arc
description: Version your conversation context with arc. Use it to RECALL prior context at the start of a session (decisions, resolved edge-cases, agreed conventions) and to PERSIST what you resolve, so context survives across sessions. arc is to your context what git is to code.
metadata:
  domain: workflow
  triggers: arc, context, remember, recall, persist, previous session, what did we decide, resume, continue prior work
---

# arc — version your own context

Your context window is wiped between sessions; arc's isn't. You (the agent) drive
it: recall at the start, persist what you resolve.

## 1. At the start: RECALL

- `arc status` — detected chat, staged messages, latest commit.
- `arc search "<terms>" --format llm` — search versioned context (full-text) with
  the task's keywords (module, feature, bug). `--role all` also includes tools.
- `arc log` — list context commits (hash + message).
- `arc read HEAD --format llm` / `arc read <hash> --format llm` — the frozen
  snapshot of a commit. Read it before proposing anything: avoid redoing work or
  contradicting a prior decision.

> **Scoped access:** you may be limited to a scope (via `ARC_SCOPE` or `--scope`).
> When scoped, `search`/`read`/`log` only see chats in that scope — so a "no
> results" doesn't mean a fact never existed, it may just be out of scope.
> `arc scope list` shows the available scopes.

## 2. While working: IDENTIFY what's worth keeping

Persist only high-signal context: design decisions and their rationale,
edge-cases and how they're handled, agreed conventions, non-obvious fixes. Do NOT
save in-progress exploration, log dumps, or code that already lives in the repo.

## 3. When a thread is resolved: PERSIST

1. Stage the relevant messages:
   - `arc add -L <n>`  (last N messages)  or
   - `arc add --from <msgID> --to <msgID>`  (exact range).
2. `arc commit -m "<message you write yourself>"` — imperative, describing the
   DECISION, not the activity. Your future self will read it in `arc log`.
   e.g. `arc commit -m "store JSONL per commit; keep binary DB out of git"`.

### Choose which roles to stage (`--role`)

`arc add` and `arc search` take `--role` to control which message roles are
included. Valid roles: `user`, `assistant`, `reasoning`, `tool` (comma-separated),
or `all`.

- **Default** (no flag): `user,assistant,reasoning` — the high-signal content,
  excluding noisy `tool` output. This keeps snapshots small and readable.
- `--role assistant` — only your replies. `--role reasoning` — only your thinking.
- `--role all` — everything, including tool calls/outputs (use sparingly; it
  bloats the snapshot and burns tokens on read).

With `-L`, the count applies AFTER the role filter: `arc add -L 10 --role assistant`
stages your last 10 assistant messages, not 10 raw messages.

## 4. Sharing

`arc sync` (push to the team remote) is run by the **human**, not you. You persist
locally with add/commit; the user decides when to sync.

## Token economy

Prefer targeted `arc search` and `arc read <hash>` over dumping the whole log into
context. One commit per coherent decision: small snapshots search and read better.
If `arc status` shows no active session, pass `--chat <id>` to add/commit.
