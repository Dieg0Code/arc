# nem

> Your agent forgets. nem doesn't.

`nem` versions **agent context** the way git versions code. When an LLM compacts
its context window, it discards reasoning, resolved edge-cases, and prior
decisions. nem persists that context explicitly, makes it searchable and
portable across agents and machines — and the agent uses it with the same
commands a human would.

## Features

- **Ingests** **Codex** and **Claude Code** sessions (conversation + reasoning +
  tool calls, with tool outputs truncated so they don't burn tokens).
- **Immutable commits**: each commit copies the text of the range (a snapshot),
  not a pointer — what you saved never changes.
- **Full-text search** with BM25 ranking (SQLite FTS5), prioritizing
  conversation over tool noise.
- **Git-based sync** with automatic **secret redaction**: API keys and tokens
  (OpenAI, Anthropic, HuggingFace, AWS, GitHub, GitLab, Stripe, wandb, …),
  connection strings, `Authorization` headers, and sensitive env vars are masked
  **before anything leaves your machine**.
- **Agent skill**: `nem init` installs a `SKILL.md` into Claude Code and Codex so
  the agent knows when and how to use nem on its own.
- **Single binary**, SQLite embedded in **pure Go** (no cgo) → a static
  executable. Works offline.

## Stack

Go · [GORM](https://gorm.io) over [`glebarez/sqlite`](https://github.com/glebarez/sqlite)
(modernc, no cgo) · SQLite FTS5/BM25 · [cobra](https://github.com/spf13/cobra) ·
git (via `os/exec`).

## Install

**Windows (Scoop):**
```powershell
scoop bucket add Dieg0Code https://github.com/Dieg0Code/scoop-bucket
scoop install nem
```

**macOS / Linux (Homebrew):**
```bash
brew install Dieg0Code/homebrew-tap/nem
```

**With Go:**
```bash
go install github.com/Dieg0Code/nem/cmd/nem@latest
```

**Prebuilt binaries:** download the archive for your OS/arch from the
[Releases](https://github.com/Dieg0Code/nem/releases) page and put `nem` on your `PATH`.

**From source:**
```bash
go build -o nem ./cmd/nem
```

## Usage (MVP)

```bash
nem init                          # creates ~/.nem + git, installs the agent skill
nem ingest                        # ingest Codex and Claude (or: nem ingest codex)
nem status                        # active session + uncommitted messages

nem add -L 20                     # stage the last 20 messages
nem add --from <msgID> --to <id>  # ...or an exact range
nem commit -m "decision about X"  # immutable snapshot

nem log                           # commit history
nem read HEAD --format llm        # the commit, clean for an agent
nem search "decay" --format llm   # BM25 search (--role all includes tools)

nem remote add origin <url>       # configure the remote
nem sync                          # export (redacting) → git push → import
nem clone <url>                   # clone the store on another machine
```

### Output formats

`read` and `search` accept `--format`:
- `llm` — clean, no metadata, for agent ingestion
- `json` — structured
- `markdown` — human-readable (default)

## The agent skill

`nem init` installs a `SKILL.md` into the agents present on your machine
(`~/.claude/skills/nem/` and `~/.codex/skills/nem/`). It teaches the agent to
**recall** prior context at the start of a session (`nem status` / `nem search` /
`nem read HEAD`) and to **persist** resolved decisions (`nem add` + `nem commit`,
writing its own commit message). `nem sync` stays in your hands.

```bash
nem init --no-skill    # skip skill installation
nem skill install      # (re)install the skill later, e.g. after upgrading nem
```

nem only ever owns the `nem` skill directory — it never touches your other skills.

## Security: secret redaction

Scrubbing runs **only on `nem sync`** (the one boundary where content leaves your
machine). The local DB keeps the raw text — just like `~/.codex` and `~/.claude`
already do — so ingesting adds no new exposure. `nem sync` redacts and reports:

```
$ nem sync
exported 12 commits
redacted 7 secrets: 5 huggingface-token, 2 env-secret
synced with the remote
```

## Architecture

```
cmd/nem/            entrypoint
internal/
  cli/              cobra commands
  config/           paths (~/.nem, nem.db, store/)
  db/               GORM models + Store (interface) + FTS5
  ingest/           codex/claude parsers (Parser interface)
  session/          active-session detection
  output/           snapshots + llm/json/markdown formats
  redact/           secret detection and masking
  skill/            embedded SKILL.md + installer
  sync/             JSONL export/import + git
```

Every component follows the **interface + functional options** pattern:
`New*(...) (Interface, error)` returns the interface, which makes testing with
mocks trivial.

## Development

```bash
go test ./...        # full suite
go vet ./...
```

The race detector (`go test -race`) requires cgo. On Windows run the tests from
**PowerShell** (not Git Bash, which DLL-shadows the mingw toolchain). In CI it
runs on Linux (`.github/workflows/ci.yml`).
