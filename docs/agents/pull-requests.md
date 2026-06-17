# Pull Requests

Use this guide when an agent is preparing a branch or pull request for this
repository.

## GitHub Operations

Use the `gh` CLI for all GitHub operations. Do not use browser tools for GitHub.

<a id="opencode"></a>

**OpenCode**

GitHub issue and PR comments must include `/oc` to invoke
`.github/workflows/opencode.yml`; comments without that trigger text will not
invoke OpenCode. The manual
`.github/workflows/opencode-doc-code-alignment.yml` workflow runs a doc/code
alignment audit. These are not yet indexed in this table — see the workflow
files for inputs and permissions.

For OpenCode runs inside GitHub Actions, also follow
[docs/agents/opencode-github-actions.md](opencode-github-actions.md). That file
defines the narrow exceptions for Action-managed PR creation, best-effort PR
title control, and workflow-provided git identity.

Common commands:

- **Inspect current PR context**: `gh pr view --json number,title,body,labels,files,reviewDecision,statusCheckRollup`
- **Discover PR number**: The system does not inject the PR number into the
  agent's context. Before editing a PR, discover it from the working tree. The
  primary path reads it from the current branch:

  ```bash
  gh pr view --json number --jq '.number'
  ```

  If that fails or returns no result, fall back to listing PRs by head branch:

  ```bash
  gh pr list --head "$(git rev-parse --abbrev-ref HEAD)" --json number --jq '.[0].number'
  ```

  Verify the discovered number matches the PR you intend to edit (check at
  least title and head branch) before making changes.
- **Create a PR**: `gh pr create --title "..." --body "..."`
- **Update PR metadata**: `gh pr edit <number> --title "..." --body "..."`
- **Check CI**: `gh pr checks <number>`
- **Read review comments**: `gh pr view <number> --comments`
- **Comment on a PR**: `gh pr comment <number> --body "..."`

When the user asks an agent to create a PR, creating the PR is part of the task.
Do not stop after pushing a branch, and do not hand the user a
`https://github.com/.../pull/new/...` URL as a substitute for `gh pr create`.
If `gh pr create` fails, report the exact failure and leave the task blocked
instead of implying that a PR exists.

## Git Identity

Use the git author and committer identity that already exists in the execution
environment. It is safe to inspect it with `git config --get user.name` and
`git config --get user.email`, but do not change it as part of normal PR work.
In particular, do not run:

- `git config user.name ...`
- `git config user.email ...`
- `git config --global user.name ...`
- `git config --global user.email ...`
- `git -c user.name=... -c user.email=... commit`
- `git commit --author=...`

Do not set `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`,
`GIT_COMMITTER_EMAIL`, or `EMAIL` to influence commits. A generic instruction
such as "use the default git identity", "recreate the commit", or "fix the
author" means to use the currently configured identity with a plain
`git commit ...`; it is not permission to hard-code values such as
`opencode-agent`, `opencode@anomaly.co`, or guessed `*@users.noreply.github.com`
addresses.

If a commit fails because git does not know the author identity, stop and report
the error instead of inventing an identity. If the user explicitly asks you to
investigate missing git identity behavior, use an isolated temporary repository;
do not change global config, this repo's local config, or the commit environment
for this project.

### Tainted Commits

A commit is tainted for publishing when any user message, review comment, PR
metadata, repository instruction, or local evidence says its author or committer
metadata may be wrong. Treat disputed identity as a stop condition, even if the
commit's code changes are otherwise correct.

Do not make a tainted commit the tip of any local or remote branch. Do not
restore, reset to, cherry-pick, revert back to, rebase onto, force-push, or
otherwise republish that commit. This applies even when the request says
"restore the original commit", "try again", "preserve the author", "use the
default identity", or "fix the author".

Do not copy author or committer values from a tainted commit into a new commit,
command-line config, environment variable, workflow setting, or PR explanation as
the intended identity. Those values are evidence of the dispute, not a source of
truth.

To keep the code changes from a tainted commit, first stop and state the metadata
conflict. After the user confirms that the code changes should be recreated,
reapply the patch onto a clean base and create a new commit with a plain
`git commit` using the already configured identity. If the configured identity is
missing or appears to be the disputed identity, stop and ask for human guidance
before committing or pushing.

When a PR adds or changes a GitHub Actions workflow or agent workflow that can
commit, verify the exact author and committer identity used by the workflow or
action. Do not enable workflows that commit as generic or unverified GitHub
noreply identities such as `<tool>@users.noreply.github.com`. Never derive a
commit email from a tool name, action name, repository name, or package name
unless that exact identity is verified. The identity must be an explicit,
reviewed bot/app identity owned by the workflow provider or this repo owner. If
the action config does not make that clear, document the risk and leave the
workflow disabled or blocked for human review.

## Documentation Updates

Update related documentation in the same PR whenever the change affects:

- CLI flags, environment variables, defaults, exit codes, or examples
- MCP tool schema, parameters, response fields, or error behavior
- Output formatting or JSON field presence
- Build, CI, lint, or test workflows
- Domain terminology in `CONTEXT.md`
- Agent instructions in `AGENTS.md` or `docs/agents/`
- Architecture decisions in `docs/adr/`

If a code change deliberately does not need documentation updates, state that in
the PR body with a short reason.

## ADR Awareness

Read `CONTEXT.md` and all ADRs under `docs/adr/` before opening the PR. If the
PR contradicts an accepted ADR, either update or supersede the ADR in the same
PR.

## Verification

PR agents must run local verification themselves before opening or updating a
PR. Do not rely on CI or reviewers as the first validation pass.

For Go code, test, CI, or script changes, the minimum local gate is:

- `go test ./...`
- `golangci-lint run ./...`

If `golangci-lint` is unavailable, run `go vet ./...` as the fallback static
check and state in the PR body that the linter itself could not be run. If any
minimum check fails because of the PR's changes, fix the failure before opening
or updating the PR. If a failure is pre-existing or environment-specific, record
the exact command, failure summary, and why it is not caused by the PR.

After the minimum gate, broaden verification when the blast radius is larger:

- `go build ./...`
- `go test -race -shuffle=on ./...`

Pure documentation changes (`.md` files only) do not require the build, test,
or lint gates.

For the full completion gate specification, see
[docs/agents/verification.md](verification.md).

## PR Title Policy

PR titles are persistent repository records, not agent progress messages.

Use a concise English title with a semantic prefix:

```
<type>: <concise change summary>
```

Allowed `type` values:

- `fix`: user-visible bug fixes, CI failures, flaky tests, or broken behavior
- `feat`: new user-visible functionality
- `docs`: documentation-only changes
- `test`: test-only changes that do not change runtime behavior
- `ci`: GitHub Actions, release workflows, or other CI configuration
- `refactor`: behavior-preserving code restructuring
- `chore`: maintenance work that does not fit the categories above

Write the summary in sentence case without a trailing period. Name the
user-visible or reviewer-visible change:

- Prefer titles such as `fix: reject overlapping sandbox mounts at startup`
- Use `docs: document sandbox path translation for agents` for documentation-only
  changes
- Keep titles specific enough to distinguish the PR in history

Do not include:

- Agent execution state such as `PR pushed`, `branch pushed`, `created PR`, or `ready`
- Prefixes that describe the agent instead of the change, such as `agent:`,
  `opencode:`, `codex:`, or `bot:`
- Session names, run IDs, timestamps, model names, or tool names unless the PR
  changes that tool directly
- Decorative arrows or symbols used as status shorthand
- Vague titles such as `fix: issue`, `docs: update docs`, or `chore: changes`

This policy applies whenever an agent creates a PR or updates PR metadata. Before
running `gh pr create` or `gh pr edit`, read the final title once as a reviewer
would. If it describes what the agent did operationally instead of what the code
or docs change, rewrite it.

## PR Body Checklist

Every PR body should include:

- Summary of code changes
- Documentation changes, or "No documentation changes needed" with a reason
- Tests run
- Linked issue(s)
- Known limitations or follow-up work, if any

Use this structure unless the user provides a stricter repository-specific
template:

```markdown
## Summary
- ...

## Documentation
- ...

## Tests
- ...

## Risks / Follow-up
- ...

Closes #...
```

Keep the body focused on durable review context. Automatically-appended agent
session cards, social-card images, HTML embeds, links to transient agent
sessions, and GitHub Actions run links are allowed, but they are supplemental
metadata and must not replace the normal summary, documentation, tests, issue
link, and risk sections.

Do not include:

- `https://github.com/.../pull/new/...` links after the PR has been created
- Multiple issues combined under a single closing keyword (e.g. `Closes #22 and #23`, `Closes #22, #23`) — only the first issue is closed. Each issue must have its own `Closes #N` line.
- Duplicated closing keywords such as two separate `Closes #22` lines
- Long pasted chat transcripts or hidden reasoning
- Claims that tests passed without the exact commands run

If the PR supersedes another branch or PR, state that plainly in the summary.
Do not use supersession as a reason to omit the normal summary, documentation,
tests, issue link, or risk sections.

## Metadata Self-Check

Before running `gh pr create` or `gh pr edit`, verify:

- The title is English, uses an allowed semantic prefix, and describes the
  change rather than the agent workflow
- The body is English and contains summary, documentation impact, tests, linked
  issues, and risks or follow-up
- The body has no `/pull/new/` URL, duplicated closing keywords, or branch-only
  handoff language
- Any non-English user instructions have been translated or summarized in
  English rather than copied verbatim
- If the user explicitly asked to create the PR, the final result is an actual
  PR URL from `gh pr create`, not only a pushed branch URL

## PR Title and Body Language

The PR title and body must be in English even if the user originally discussed
the change in another language. PR comments and review replies should reply in
whatever language the user is using. See the `GitHub and PR Work` section in the
root `AGENTS.md` for the canonical summary.
