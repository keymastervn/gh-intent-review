# Implementation Plan

## Phase 1: Foundation (Current) ✅

- [x] Project scaffolding with `gh extension create --precompiled=go`
- [x] CLI structure with cobra (root, generate, review, config, version)
- [x] Unified diff parser
- [x] Focused diff model (Intent, FocusedFile, FocusedDiff)
- [x] Config system (YAML, defaults, env var overrides)
- [x] LLM provider interface + Anthropic/OpenAI/Ollama implementations
- [x] Parallel review engine
- [x] Interactive review UI (approve/disapprove/skip/quit)
- [x] JSON storage for focused diffs

## Phase 2: Polish & Robustness

- [ ] Proper glob matching for file ignore/focus patterns
- [ ] Better diff fetching (handle GitHub API media types correctly)
- [ ] Retry logic for LLM API calls (transient failures)
- [ ] Progress bar/spinner during `generate`
- [ ] Color-coded terminal output for intent severity
- [ ] Validate LLM JSON responses more robustly
- [ ] Unit tests for diff parser, config loading, URL parsing

## Phase 3: Enhanced Review Experience

- [ ] Rich terminal UI (bubbletea/lipgloss) for review sessions
- [ ] Side-by-side view: raw diff + intent annotations
- [ ] Post disapproved intents as GitHub PR review comments
- [ ] Export review results (markdown, JSON)
- [ ] `gh intent-review status <pr-url>` — show review progress
- [ ] Resume interrupted review sessions

## Phase 4: CI/CD Integration

- [ ] GitHub Actions workflow for auto-generating intent diffs on PR open
- [ ] `--ci` flag for non-interactive mode (output summary + exit code)
- [ ] Configurable thresholds (fail if N critical intents)
- [ ] PR status check integration

## Phase 5: Confidence Scoring (Future)

- [ ] Confidence score per intent (how sure is the LLM)
- [ ] Reviewer confidence tracking across sessions
- [ ] PR author confidence score
- [ ] Auto-merge suggestions when confidence is high
- [ ] Dashboard for confidence trends

## Phase 6: Custom Symbols & Teams

- [ ] Team-shared config (repo-level `.gh-intent-review.yml`)
- [ ] Custom intent symbol definitions
- [ ] Domain-specific review profiles (e.g. "fintech" enables `$` by default)
- [ ] Intent suppression comments (`// intent-review:ignore ¿!`)

---

## How to Build & Install

```bash
# Build
go build -o gh-intent-review .

# Install as gh extension
gh extension install . # note: gh extension remove gh-intent-review if necessary

# Or run directly
./gh-intent-review generate https://github.com/owner/repo/pull/123
./gh-intent-review review https://github.com/owner/repo/pull/123
```

## Configuration

```bash
# Create default config
gh intent-review config init

# Show current config
gh intent-review config show
```

Edit `.gh-intent-review.yml` to customize:

```yaml
llm:
  provider: anthropic          # anthropic, openai, ollama
  model: claude-sonnet-4-6
review:
  parallel: 4
  ignore_files:
    - "*.lock"
    - "vendor/**"
intents:
  symbols:
    - symbol: "!"
      name: Security Risk
      enabled: true
    # ... add custom symbols here
```
