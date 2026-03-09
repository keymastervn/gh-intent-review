# Architecture

## Overview

`gh-intent-review` is a GitHub CLI extension that replaces traditional `+/-` diff-based code review with **intent-focused review** using symbolic notation (`¿!`, `¿~`, `¿&`, `¿#`, `¿=`, `¿?`).

The extension has two main commands:
1. **`generate`** — Fetches a PR diff, runs parallel agentic review, produces an `.intentional.diff`
2. **`review`** — Interactive terminal session to walk through intents and approve/disapprove

```
┌─────────────┐     ┌──────────────┐     ┌───────────────┐     ┌──────────────┐
│  GitHub API  │────→│  Diff Parser │────→│ Review Engine │────→│  .intentional│
│  (PR diff)   │     │  (unified)   │     │  (parallel)   │     │    .diff     │
└─────────────┘     └──────────────┘     └───────┬───────┘     └──────┬───────┘
                                                  │                    │
                                          ┌───────▼───────┐   ┌──────▼───────┐
                                          │  LLM Provider  │   │   Review UI  │
                                          │ (pluggable)    │   │ (interactive)│
                                          └───────────────┘   └──────────────┘
```

## Directory Structure

```
gh-intent-review/
├── main.go                          # Entry point
├── cmd/                             # CLI commands (cobra)
│   ├── root.go                      # Root command + subcommand registration
│   ├── generate.go                  # `gh intent-review generate <pr-url>`
│   ├── review.go                    # `gh intent-review review <pr-url>`
│   └── config.go                    # `gh intent-review config init|show`
├── internal/
│   ├── config/
│   │   └── config.go               # YAML config loading, defaults, intent symbols
│   ├── diff/
│   │   ├── parser.go               # Unified diff → structured FileDiff
│   │   └── focused.go              # Intentional diff format: parse, render, read/write
│   ├── github/
│   │   └── client.go               # PR URL parsing, diff fetching via gh auth
│   ├── reviewer/
│   │   ├── engine.go               # Parallel review orchestration
│   │   ├── provider.go             # LLMProvider factory
│   │   ├── response.go             # Shared LLM JSON response parsing
│   │   ├── anthropic.go            # Anthropic (Claude) provider
│   │   ├── openai.go               # OpenAI provider
│   │   └── ollama.go               # Ollama (local) provider
│   └── ui/
│       └── review_session.go       # Interactive terminal review loop
├── examples/
│   └── 42.intentional.diff         # Example intentional diff for reference
├── ARCHITECTURE.md
├── PLAN.md
└── README.md
```

## Intentional Diff Format

The `.intentional.diff` file has two sections:

### Section 1: Original diff (unchanged)

The raw `git diff` output, preserved exactly as GitHub returns it. This means the file is a valid diff — you can pipe it through standard tools.

```diff
diff --git a/handler.js b/handler.js
--- a/handler.js
+++ b/handler.js
@@ -10,6 +10,8 @@
 async function updateUser(req, res) {
+  const query = "SELECT * FROM users WHERE id = " + req.params.id;
+  for (let i = 0; i < users.length; i++) {
+    const profile = await fetchProfile(users[i].id);
```

### Section 2: Intent blocks

Each intent is a self-contained block that references back into the diff:

```
¿!! b/handler.js
@@ +12,1 @@
+  const query = "SELECT * FROM users WHERE id = " + req.params.id;
-{¿! SQL injection — use parameterized queries ¿!}

¿~~ b/handler.js
@@ +14,3 @@
+  for (let i = 0; i < users.length; i++) {
+    const profile = await fetchProfile(users[i].id);
+    enriched.push({ ...users[i], profile });
-{¿~ N+1 query — use Promise.all() for parallel fetching ¿~}
   }
```

**Block anatomy:**

| Line | Meaning |
|------|---------|
| `¿!! b/file.txt` | Block header: doubled symbol + file path |
| `@@ +10,2 @@` | Affected line range in the new file |
| `+code line` | Flagged code (can span multiple consecutive lines) |
| `-{¿! comment ¿!}` | The intent explanation |
| ` context` | Optional surrounding context (space-prefixed) |

**Why this format:**
- The original diff stays clean — no inline annotations cluttering the code
- Intent blocks use familiar diff-like syntax (`+`, `-`, ` ` prefixes)
- Multi-line intents naturally span consecutive `+` lines
- Multiple intents on the same line = multiple blocks pointing to it
- Parseable with simple regex: `^¿(!!|~~|##|...)` for headers, `^-\{¿` for comments

## Key Design Decisions

### 1. Pluggable LLM Providers

The `LLMProvider` interface:

```go
type LLMProvider interface {
    ReviewFile(fileDiff *diff.FileDiff, symbols []config.IntentSymbol) ([]diff.Intent, error)
}
```

The LLM returns JSON (reliable to parse), and the engine assembles the intent blocks. Adding a new provider means implementing this interface and registering it in `provider.go`.

Supported: **Anthropic**, **OpenAI**, **Ollama** (local).

### 2. Intent Symbol System

Symbols are defined in config and can be enabled/disabled per-project:

| Symbol | Block Header | Category | Name | Default |
|--------|-------------|----------|------|---------|
| `!` | `¿!!` | Reliability | Security Risk | On |
| `~` | `¿~~` | Reliability | Performance Drag | On |
| `$` | `¿$$` | Reliability | Resource Cost | Off |
| `&` | `¿&&` | Form | Coupling Violation | On |
| `#` | `¿##` | Form | Cohesion / SOLID | On |
| `=` | `¿==` | Form | DRY Violation | On |
| `?` | `¿??` | Form | KISS Violation | On |

All symbols use `¿` prefix to avoid collision with standard diff `+`/`-`/` ` notation.

### 3. Parallel Review

The review engine uses a semaphore-based worker pool. Each file is reviewed independently and concurrently, with configurable parallelism (default: 4 workers). The engine collects all intents and assembles the final `.intentional.diff`.

### 4. Storage

Intentional diffs are stored as plain text at `~/.gh-intent-review/<owner>/<repo>/<pr>.intentional.diff` by default. If `output.dir` is set in config, they're stored relative to the project. This allows:
- Offline review (generate once, review later)
- Persistence across sessions
- The file is human-readable without any tooling

### 5. GitHub Auth

The extension piggybacks on `gh`'s authentication (`gh auth login`), so no separate token management is needed. Uses the `go-gh` library for API access.

## Extensibility Points

- **LLM Providers**: Implement `LLMProvider` interface, register in `provider.go`
- **Intent Symbols**: Add custom symbols via `.gh-intent-review.yml`
- **File Filters**: `ignore_files` and `focus_files` glob patterns in config
- **Custom Prompts**: `custom_prompt` field appended to the review system prompt
