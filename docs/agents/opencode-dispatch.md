> **Dispatcher-only document.** This file is for the human maintainer and
> any coordinator agent that writes `/oc` comments on issues or PRs. It is
> not a runtime document the cloud OpenCode agent reads. Stating the rules
> here does not teach the cloud OpenCode agent to follow them; the agent
> only sees the dispatch comment itself.

# Dispatching to Cloud OpenCode

This document is for whoever writes `/oc` comments on GitHub issues or PRs
that trigger the cloud OpenCode agent.

## What a dispatch comment is for

It states a target and the context the agent needs to start. It does not
direct the agent's process. The agent runs on a managed runtime whose
filesystem layout, installed tools, and pre-run setup are not visible
from the dispatcher.

## What to leave out

- **File system paths to skills, tools, or configuration.** Each runtime
  lays these out differently, so a path that is correct here will be
  wrong there, and the agent will look for something that does not
  exist on its system.
- **References to git checkout state, submodule sync, branch fetch, or
  any other setup the managed runtime is responsible for.** The runner
  prepares the working tree before the agent starts. Comments that
  describe or re-invoke that setup can leave the working tree in a state
  the infrastructure then commits automatically, or can collide with the
  runner's own setup steps.
- **Restatement of anything already in the linked issue body or project
  documentation.** Link instead of duplicating.

## What belongs in a dispatch comment

- The goal of the work and the issue(s) to close.
- Constraints specific to this dispatch that the issue body does not
  already state.
- Project documents the agent should consult by file name. Do not name
  third-party skills or tools that the agent may or may not have
  installed.
