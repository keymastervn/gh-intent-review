# gh-intent-review

A GitHub CLI extension that replaces traditional `+/-` diff-based code review with **intent-focused review**. Built for reviewers in the agentic era — instead of reading line-by-line diffs, AI pre-generates an intentional diff that surfaces only what truly matters: security risks, performance issues, coupling violations, and more.

The intended workflow: CI or a team lead runs `generate` on a PR, producing a `.intentional.diff`. Reviewers then use `review` to walk through flagged intents interactively. Implementers can also install it locally to pre-emptively catch issues before review.

```diff
diff --git a/handler.js b/handler.js
--- a/handler.js
+++ b/handler.js
@@ -10,6 +10,8 @@
 async function updateUser(req, res) {
+  const query = "SELECT * FROM users WHERE id = " + req.params.id;
+  for (let i = 0; i < users.length; i++) {
+    const profile = await fetchProfile(users[i].id);

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
```

## Install

```bash
gh extension install keymastervn/gh-intent-review
```

Or build from source:

```bash
git clone https://github.com/keymastervn/gh-intent-review.git
cd gh-intent-review
go build -o gh-intent-review .
gh extension install .
```

## Quick Start

```bash
# 1. Generate an intent-focused diff for a PR
gh intent-review generate https://github.com/owner/repo/pull/123

# 2. Interactively review the intents
gh intent-review review https://github.com/owner/repo/pull/123
```

## The Intentional Diff Format

The `.intentional.diff` file has two sections:

**Section 1** — The original git diff, untouched. Standard tools can still read it.

**Section 2** — Intent blocks appended after the diff. Each block:

```
¿!! b/file.txt              ← doubled symbol + file path
@@ +10,2 @@                  ← affected line range
+  flagged code line 1       ← the code being flagged (+ prefix)
+  flagged code line 2
-{¿! explanation here ¿!}   ← the intent comment (- prefix)
 optional context line       ← surrounding code (space prefix)
```

This means:
- The original diff stays **clean** — no inline clutter
- Intents can span **multiple consecutive lines** (e.g. a whole loop)
- **Multiple intents on the same line** = multiple blocks pointing to it
- The file is **human-readable** without any tooling

## Commands

### `generate <pr-url>`

Fetches the PR diff, runs parallel AI review on each file, and produces an intentional diff.

```bash
gh intent-review generate https://github.com/owner/repo/pull/123

# Override the LLM provider/model
gh intent-review generate --provider openai --model gpt-4o https://github.com/owner/repo/pull/123

# Use a local Ollama model
gh intent-review generate --provider ollama --model llama3 https://github.com/owner/repo/pull/123

# Use an installed CLI agent (e.g. Claude Code) — agent traverses $PWD for context
gh intent-review generate --provider agent https://github.com/owner/repo/pull/123

# Control parallelism
gh intent-review generate -p 8 https://github.com/owner/repo/pull/123

# Custom output path
gh intent-review generate -o ./my-review/123.intentional.diff https://github.com/owner/repo/pull/123
```

Output is stored at `~/.gh-intent-review/<owner>/<repo>/<pr>.intentional.diff` by default.

### `review <pr-url>`

Opens an interactive session to walk through each intent. For each one you can:

- **[a]pprove** — mark as acceptable
- **[d]isapprove** — flag with a comment
- **[s]kip** — move on
- **[q]uit** — end the session

```
  [1/4] ¿! Security Risk
  +  const query = "SELECT * FROM users WHERE id = " + req.params.id;
  SQL injection — use parameterized queries

  [a]pprove  [d]isapprove  [s]kip  [q]uit →
```

### `config init` / `config show`

```bash
# Create a default .gh-intent-review.yml in the current directory
gh intent-review config init

# Show the active configuration
gh intent-review config show
```

## Configuration

Create `.gh-intent-review.yml` in your project root (or home directory for global defaults):

```yaml
llm:
  provider: agent                  # "agent" (default) or "custom" (see Custom LLM Providers below)
  model: claude-sonnet-4-6         # passed to the agent via --model (optional)
  agent_command: claude            # CLI binary in PATH (default: "claude")

review:
  ignore_files:                    # skip these files
    - "*.lock"
    - "*.sum"
    - "vendor/**"
    - "node_modules/**"
  # focus_files:                   # only review these (if set)
  #   - "src/**"
  #   - "lib/**"
  # custom_prompt: "Also check for Rails-specific security issues"
  # check_and_fetch: true          # auto-regenerate diff when the PR head changes
  # auto_approve: false

intents:
  # severity: minor                # global threshold: none (default), trivial, minor, major, critical
  #                                # Symbols whose typical severity is below the threshold are
  #                                # excluded from the agent prompt entirely.
  #                                # Within included symbols the agent still judges each instance
  #                                # and skips findings below the threshold.
  symbols:
    # Reliability
    - symbol: "!"
      name: Security Risk
      description: "Vulnerabilities: SQL injection, XSS, exposed secrets"
      enabled: true
      category: reliability
      severity: critical           # per-symbol typical severity
    - symbol: "~"
      name: Performance Drag
      description: "Latency, slow execution, performance bottlenecks"
      enabled: true
      category: reliability
      severity: major
    - symbol: "$"
      name: Resource Cost
      description: "Expensive operations, memory leaks, compute waste"
      enabled: false               # opt-in
      category: reliability
      severity: major
    # Form
    - symbol: "&"
      name: Coupling Violation
      description: "Tight coupling, hardcoded dependencies"
      enabled: true
      category: form
      severity: minor
    - symbol: "#"
      name: Cohesion / SOLID Issue
      description: "Low cohesion, single responsibility violations"
      enabled: true
      category: form
      severity: minor
    - symbol: "="
      name: DRY Violation
      description: "Code duplication, repeated logic"
      enabled: true
      category: form
      severity: trivial
    - symbol: "?"
      name: KISS Violation
      description: "Overly clever, deeply nested, hard to read code"
      enabled: true
      category: form
      severity: trivial

output:
  dir: ""                            # empty = ~/.gh-intent-review/ (default)
  # dir: .gh-intent-review          # set to store diffs in the project directory instead
  format: text
```

### Custom LLM Providers

Set `provider: custom` to route the Claude Code agent through an alternative backend — useful for OpenRouter, Ollama, or any OpenAI-compatible endpoint.

The three fields `model`, `base_url`, and `api_key` are all required and validated at startup.

**OpenRouter:**

```yaml
llm:
  provider: custom
  agent_command: claude
  model: nvidia/nemotron-3-nano-30b-a3b:free  # any model slug from openrouter.ai/models
  base_url: https://openrouter.ai/api
  api_key: sk-or-v1-xxxxxxxxxxxxxxxxxxxx
```

**Ollama (local):**

```yaml
llm:
  provider: custom
  agent_command: claude
  model: llama3
  base_url: http://localhost:11434
  api_key: ollama  # any non-empty string; Ollama ignores it
```

Under the hood, `provider: custom` injects these environment variables when invoking the agent command:

```
ANTHROPIC_BASE_URL=<base_url>
ANTHROPIC_AUTH_TOKEN=<api_key>
ANTHROPIC_API_KEY=            # explicitly empty (required by the Claude Code agent)
```

### Severity threshold

The `intents.severity` setting filters noise by impact level:

| Value | Effect |
|-------|--------|
| `none` (default) | Report all findings regardless of impact |
| `trivial` | Report everything (same as none in practice) |
| `minor` | Skip trivial cosmetic issues; keep real quality problems |
| `major` | Only report significant reliability and design issues |
| `critical` | Only report security, data-integrity, and stability risks |

Two levels of filtering apply:

1. **Symbol exclusion** — symbols whose `severity` is below the global threshold are removed from the agent prompt entirely (e.g. `severity: major` drops DRY and KISS violations before the agent runs).
2. **Instance judgement** — for included symbols, the agent evaluates each specific finding and skips instances that don't meet the threshold in practice (e.g. a trivial coupling issue is skipped even though `&` is included).

**Example — only security and performance, no form issues:**

```yaml
intents:
  severity: major
```

With the defaults above, this keeps `!` (critical) and `~` (major) in the prompt and silently drops `&`, `#`, `=`, `?` (all minor/trivial).

## Using with Claude Code

`gh-intent-review` pairs well with [Claude Code](https://claude.com/claude-code) as part of an agentic development workflow.

### As a Claude Code hook

Configure a hook in `.claude/hooks.json` to auto-generate intent reviews when PRs are created:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "pattern": "gh pr create",
        "command": "gh intent-review generate $(gh pr view --json url -q .url) 2>/dev/null || true"
      }
    ]
  }
}
```

### Interactive review with Claude Code

Ask Claude Code to interpret the results:

```
> Run gh intent-review generate on PR #123, then read the intentional diff
> and fix any critical security intents
```

Claude Code will:
1. Run `gh intent-review generate` to produce the intentional diff
2. Read the `~/.gh-intent-review/<owner>/<repo>/123.intentional.diff` file
3. Parse the `¿!!` / `¿~~` / etc. intent blocks and fix the flagged issues

### Team workflow

```bash
# CI or team lead pre-generates the intentional diff for a PR
gh intent-review generate https://github.com/org/repo/pull/456

# Reviewer walks through intents interactively
gh intent-review review https://github.com/org/repo/pull/456

# Or ask Claude Code to handle the review
# "Review PR #456 using gh intent-review and summarize the findings"
```

### Implementer self-review (optional)

Implementers can install the extension to catch issues before submitting for review:

```bash
# After pushing a PR, pre-emptively generate intents on your own code
gh intent-review generate https://github.com/org/repo/pull/456
gh intent-review review https://github.com/org/repo/pull/456
# Fix any critical/warning intents before requesting review
```

## Intent Notation

All symbols use the `¿` prefix to avoid collision with standard diff `+`/`-` notation. In intent blocks, symbols are doubled for the header (`¿!!`, `¿~~`, etc.).

| Symbol | Block Header | Category | Name | What it flags |
|--------|-------------|----------|------|---------------|
| `!` | `¿!!` | Reliability | Security Risk | SQL injection, XSS, exposed secrets |
| `~` | `¿~~` | Reliability | Performance Drag | O(n^2) loops, blocking calls, N+1 queries |
| `$` | `¿$$` | Reliability | Resource Cost | Memory leaks, expensive compute (opt-in) |
| `&` | `¿&&` | Form | Coupling Violation | Hardcoded deps, tight coupling |
| `#` | `¿##` | Form | Cohesion / SOLID | God classes, mixed responsibilities |
| `=` | `¿==` | Form | DRY Violation | Duplicated logic |
| `?` | `¿??` | Form | KISS Violation | Over-engineering, deep nesting |

## License

MIT
