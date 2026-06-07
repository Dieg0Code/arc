<div align="center">

<img src="assets/logo.png" alt="nem" width="300"/>

**Your agent forgets. nem doesn't.**

nem versions agent context the way git versions code.

[![Go Report Card](https://goreportcard.com/badge/github.com/Dieg0Code/nem)](https://goreportcard.com/report/github.com/Dieg0Code/nem)
[![CI](https://img.shields.io/github/actions/workflow/status/Dieg0Code/nem/ci.yml?branch=main)](https://github.com/Dieg0Code/nem/actions)
[![codecov](https://codecov.io/gh/Dieg0Code/nem/graph/badge.svg?token=RFLOXG1BAA)](https://codecov.io/gh/Dieg0Code/nem)
[![Go Reference](https://pkg.go.dev/badge/github.com/Dieg0Code/nem.svg)](https://pkg.go.dev/github.com/Dieg0Code/nem)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Dieg0Code/nem?display_name=release)](https://github.com/Dieg0Code/nem/releases)

</div>

---

When an LLM compacts its context window it throws away the reasoning, the
resolved edge-cases, the decisions. nem persists that context as **immutable,
searchable commits** — and your agent recalls and writes them with the same
commands you would. Single binary, SQLite embedded in pure Go (no cgo), offline.

## Install

```powershell
# Windows (Scoop)
scoop bucket add Dieg0Code https://github.com/Dieg0Code/scoop-bucket
scoop install nem
```
```bash
# macOS / Linux (Homebrew)
brew install Dieg0Code/homebrew-tap/nem

# or with Go
go install github.com/Dieg0Code/nem/cmd/nem@latest
```

## How your agent uses it

`nem init` installs a skill into Claude Code and Codex. The agent **recalls** at
the start of a session and **persists** what it resolves — no human in the loop:

```bash
nem outline                         # the map: project → chat → commit, summarized
nem search "<terms>" --format llm   # hybrid search (BM25 + semantic)
nem read <hash> --format llm        # a frozen snapshot, clean for an agent
nem commit -m "decision about X"    # persist a resolved decision (immutable)
```

The agent navigates the tree, reads what matters, and writes its own commits.
You stay in control of one thing: `nem sync` (sharing) is yours.

## Commands

| | |
|---|---|
| `nem init` / `ingest` | set up `~/.nem`; pull in Codex & Claude Code sessions |
| `nem status` / `log` | active session; commit history |
| `nem add` / `commit` | stage messages; freeze them into an immutable snapshot |
| `nem outline` / `timeline` | navigate the index tree; see how decisions evolved |
| `nem search` / `read` | hybrid retrieval (keyword/semantic); drill into content |
| `nem annotate` | rewrite a node's summary (pinned; survives re-index) |
| `nem index` | (re)build the tree — incremental, only computes what's new |
| `nem sync` / `clone` | push/pull to a git remote, **redacting secrets first** |
| `nem doctor` | check/install the optional pro deps |

## How it works

- **Immutable commits** — a commit copies the *text* of its range (a snapshot),
  not a pointer. What you saved never changes.
- **Navigable index** — a PageIndex-style tree (project → chat → commit) with
  summaries the agent reasons over before drilling in. The agent is the reranker.
- **Hybrid search** — BM25 (SQLite FTS5) over messages + nodes, fused (RRF) with a
  recency boost and optional semantic embeddings.
- **Mutable summary layer** — `nem annotate` lets the agent curate how a commit is
  described and found, without touching the immutable content.

## Optional: the semantic layer

Richer LLM summaries and embeddings are opt-in and run locally (Ollama) or via an
OpenAI-compatible API — nem's core stays embedding-free and pure Go.

```bash
nem config set embed.backend ollama       # turn on the semantic layer
nem doctor --fix                          # installs Ollama + pulls the models
nem index                                 # build summaries + embeddings
```

MCP: `nem mcp` exposes the same capabilities as typed tools
(`nem_outline`, `nem_search`, `nem_read`, `nem_annotate`, …) for agents that
speak MCP.

## Security

Secret redaction runs **only on `nem sync`** — the one boundary where content
leaves your machine. API keys, tokens, connection strings, `Authorization`
headers and sensitive env vars are masked before anything is pushed, and reported:

```
$ nem sync
exported 12 commits
redacted 7 secrets: 5 huggingface-token, 2 env-secret
synced with the remote
```

## Development

```bash
go test ./...
go vet ./...
```

`-race` needs cgo; on Windows run from PowerShell (Git Bash DLL-shadows mingw).
CI runs race + coverage on Linux.

## License

[MIT](LICENSE)
