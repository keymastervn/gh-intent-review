# Changelog

All notable changes to this project will be documented in this file.

## [1.1.0] - 2026-03-10

### Added

- `[o]pen` action in review session — opens the PR Files tab in the browser anchored to the exact flagged line (`#diff-{sha256}R{line}`)
- File path and line number displayed under each intent header during review (e.g. `handler.js:42`)
- `--force-fetch` flag on `review` command — always regenerates the intentional diff before starting the session, bypassing any cached file
- `provider: custom` in LLM config — routes the Claude Code agent through an alternative backend (OpenRouter, Ollama, etc.) by injecting `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, and `ANTHROPIC_API_KEY` as environment variables
- `install.sh` — copies `.gh-intent-review.yml.example` to `~/.gh-intent-review.yml` on first install

### Fixed

- Line-specific PR comments now correctly target the flagged line — `StartLine` was not being parsed from the hunk header (`@@ +N,M @@`) when reading `.intentional.diff` files back from disk, causing all comments to fall back to general PR comments

## [1.0.0] - 2026-01-01

### Added

- `generate <pr-url>` command — fetches a PR diff, runs agentic review, and writes an `.intentional.diff` file
- `review <pr-url>` command — interactive session with `[e]laborate`, `[c]omment`, `[s]kip`, `[q]uit` actions
- `version` command — prints the current version
- `config init` / `config show` commands
- `auto_approve` config option — automatically approves the PR when no intents remain after review, including version/model/provider in the approval body
- `check_and_fetch` config option — auto-regenerates the intentional diff when the PR head SHA has changed since the last generate
- Intent symbols: `!` Security Risk, `~` Performance Drag, `$` Resource Cost, `&` Coupling Violation, `#` Cohesion/SOLID, `=` DRY Violation, `?` KISS Violation
- Severity threshold filtering (`intents.severity`) — excludes low-impact symbols from the agent prompt entirely and instructs the agent to skip below-threshold findings
- Robust agent response parsing — strips fenced code blocks and fixes invalid JSON escape sequences from LLM output
