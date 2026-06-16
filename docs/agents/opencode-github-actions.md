# OpenCode in GitHub Actions

Use this guide only for OpenCode runs executed by this repository's GitHub
Actions workflows, such as `.github/workflows/opencode.yml` and
`.github/workflows/opencode-doc-code-alignment.yml`.

These rules are a narrow compatibility layer for the OpenCode GitHub Action.
They do not replace the general repository rules in `AGENTS.md`; they identify
which general pull-request rules do not map cleanly to the Action runtime.

## Scope

GitHub Actions OpenCode runs are different from local or interactive agents:

- They are invoked by workflow events or manual workflow dispatch, not by a
  user running `gh` locally.
- PR creation and commenting may be handled by OpenCode's GitHub integration
  instead of direct `gh pr create` calls.
- Some generated PR metadata, especially PR titles, can be difficult for the
  agent to control because of current Action/runtime behavior.
- The workflow-provided git identity may be controlled by the Action and must
  not be guessed or overwritten by the agent.

When a rule in this file conflicts with the general PR workflow, this file wins
for GitHub Actions OpenCode runs only.

## Invocation

The comment-triggered OpenCode workflow runs only when an authorized comment
contains `/oc` or `/opencode`. Comments without those trigger words do not start
OpenCode.

Do not set a fixed `with.prompt` in `.github/workflows/opencode.yml`. The
OpenCode GitHub Action treats that input as an override for the default
comment-driven prompt, so a fixed prompt can cause OpenCode to ignore the
actual `/oc ...` user request. Put durable instructions in `AGENTS.md` and
`docs/agents/` instead.

The documentation/code alignment workflow is manually dispatched. Follow its
workflow inputs first:

- If `allow_pr` is false, do not modify files and do not open PRs.
- If `allow_pr` is true, open a PR only for narrow, unambiguous fixes supported
  by repository evidence.

## GitHub Operations

OpenCode's GitHub Action infrastructure owns only the operations it supports:

- Posting comments
- Pushing commits
- Opening PRs

Prefer the OpenCode GitHub integration for those operations. In particular, use
it for PR creation when the workflow allows PRs.

Use `gh` where the workflow or prompt explicitly requires an operation that the
OpenCode integration does not support. The documentation/code alignment workflow
is the known exception: it grants `issues: write` and explicitly requires
`gh issue create` because OpenCode's GitHub integration does not provide native
Issue creation in that workflow.

The general rule that PR agents must always use `gh pr create` is waived for
GitHub Actions OpenCode. If the OpenCode GitHub integration creates the PR, that
is sufficient.

Do not substitute a `/pull/new/...` URL for an actual PR. If neither OpenCode's
GitHub integration nor `gh pr create` can create the PR, report the exact
failure and stop.

## Automatic Push Safety

Treat the OpenCode GitHub Action working tree as a write boundary. The Action
infrastructure checks whether the local branch is dirty after the agent
responds; if it is dirty, the infrastructure may create a commit and push it
automatically. This can happen even when the user asked only a question.

Before doing anything that can modify files, decide whether the user explicitly
asked for a content change.

For read-only requests, such as investigation, explanation, verification,
review, triage, or "is this safe?", the agent must not leave any file,
submodule, generated artifact, dependency file, cache output, or formatting
change in the working tree. Use read-only commands where possible. Avoid
commands that update submodules, rewrite generated files, run formatters, run
tidy commands, or otherwise normalize the checkout unless the user asked for a
change.

If a read-only task dirties the working tree anyway, restore the worktree to its
initial state before finishing. If it cannot be restored safely, stop and report
the exact dirty files instead of allowing the infrastructure to auto-push them.

For write requests, keep changes limited to the requested scope and verify the
diff before the run finishes. Never rely on the infrastructure's automatic
commit step as a substitute for checking what will be pushed.

## PR Titles and Bodies

Write PR bodies in English and include durable review context:

- Summary
- Documentation impact
- Tests or verification run
- Linked issue(s), when applicable
- Known limitations or follow-up work

PR titles must follow the repository PR title policy whenever the runtime
exposes enough control to create or update PR metadata. Read
`docs/agents/pull-requests.md#pr-title-policy` before setting or editing a PR
title. Do not use filenames, issue fragments, agent status, session names, or
titles without an allowed semantic prefix as PR titles.

If the OpenCode GitHub Action produces an initial PR title that cannot be
reliably controlled, treat that specific creation-time title as an Action
limitation, not a task failure. This exception is narrow: when a reliable edit
path exists, or when the agent is already updating PR metadata, the agent must
bring the PR title back into compliance before reporting the metadata work
complete. If a title-related status check is failing, inspect the check message
and make the new title satisfy that policy.

Automatically appended OpenCode session links, social-card images, and GitHub
Actions run links are allowed as supplemental metadata. They must not replace
the normal PR body sections.

## Git Identity

Do not configure git author or committer identity from inside the run. In
particular, do not derive an identity from:

- The tool name
- The Action name
- The repository name
- A guessed GitHub noreply address
- A prior disputed commit

If the workflow-provided identity is missing, ambiguous, or known to be wrong,
stop and report the exact evidence. Do not set `user.name`, `user.email`,
`GIT_AUTHOR_*`, `GIT_COMMITTER_*`, or `EMAIL` to make a commit succeed.

This rule exists because an OpenCode investigation documented a
GitHub Actions run that used a squatted-looking email identity.
That kind of value is evidence of a problem, not a source of truth.

For commits created by the OpenCode GitHub Action, the acceptable pattern is an
explicit, provider-owned bot/app identity or a reviewed repository-owner
identity supplied by the workflow. Do not infer identity safety from a commit
headline or PR body alone.

## Verification

GitHub Actions OpenCode must follow the same AI-agent completion gate as every
other agent. Before opening a PR, updating a PR, or reporting a code-changing
task complete, run the non-E2E workflow checks listed in
`docs/agents/verification.md`.

If the workflow prompt specifies stronger verification, follow the prompt. If a
tool is unavailable in the Action environment, use the documented fallback from
`docs/agents/verification.md` and state the limitation in the PR body or final
comment.

For read-only audit workflows, do not run broad local verification unless it is
needed to support a finding.

## Relationship to Other Agent Rules

The following general rules still apply:

- Do not modify `.gitignore` unless explicitly asked.
- Use patch-style edits for existing files.
- Keep documentation in English.
- Update related documentation when behavior or workflow rules change.

The following general rules are narrowed for GitHub Actions OpenCode:

- `gh pr create` is not mandatory when OpenCode's GitHub integration creates the
  PR.
- PR title policy is best-effort when the Action runtime does not provide
  reliable title control.
- GitHub operation routing follows the workflow prompt: OpenCode's integration
  handles comments, pushes, and PRs; unsupported operations such as Issue
  creation use `gh` only when the workflow grants the needed permission and says
  to do so.
