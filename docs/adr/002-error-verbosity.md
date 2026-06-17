# ADR-002: MCP Tool Error Verbosity — Design Principle

## Status

Accepted

## Context

Security audits (both manual and automated) commonly flag MCP tool error
messages that include configuration details such as the server hostname,
`MaxContentSize`, or blocked path names. The golang-security skill recommends
"return generic messages, log details server-side" — a rule designed for web
applications serving untrusted end users in browsers.

However, this project is an MCP server whose client is an AI agent operating
the tool on behalf of a user, not a browser talking to an unknown attacker.
Applying the web-app error-handling rule here without adjustment produces
false positives that, if "fixed," would make the tool less useful.

## Decision

MCP tool error messages SHALL include operationally relevant details. The
following items are explicitly **not** secrets, not PII, and not subject to
sanitization in error messages:

### 1. Server hostname

A pastebin paste must include its hostname in the response — otherwise the
user has no way to access it. The agent needs the hostname to construct
usable paste URLs to return to the user. Hostname is therefore not a secret,
not PII, and not something to sanitize in this context.

### 2. MaxContentSize

The AI agent needs to know the content size limit to decide whether to
truncate, split, or refuse a payload before attempting to upload. The limit
is a capacity configuration, not a security credential, and exposing it in
error messages helps the agent self-correct.

### 3. Hardcoded default blocklist entries

The built-in blocked paths (`/etc`, `/proc`, `/sys`, `/dev`) and blocked
components (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`, `.git`) are
hardcoded defaults visible in the public source code. Their names in error
messages reveal nothing an attacker could not learn from reading the
repository.

### 4. What SHALL be sanitized

Operator-configured security policy details SHALL NOT appear in MCP tool error
messages. Specifically:

- **`BLOCKED_PATHS`** (user-defined blocklist): The matched path SHALL be
  omitted from the error. The client receives a generic "file path is in a
  user-blocked directory" without naming the blocked path.

This is the line between "operationally useful information" and "security
policy disclosure."

## Reasoning

### MCP server vs web application threat model

| Dimension | Web application | MCP server |
|-----------|----------------|------------|
| Client | Untrusted browser | AI agent (trusted deployment) |
| Error consumer | End user (potential attacker) | AI agent (tool operator) |
| Hostname visibility | Must be hidden (CORS, referrer) | Must be visible (paste URL) |
| Config details | Leak internal topology | Aid agent in correct operation |
| Attack vector | Direct HTTP requests | Prompt injection (indirect) |

The prompt injection vector is real but has a different blast radius: a
subverted agent already has access to the configured server URL and can probe
it directly. Protecting the hostname in error messages adds no meaningful
defense against this threat.

### Practical necessity

A "return generic error" approach would mean:

```
"Create paste error: operation failed" — unknown why
"Create paste error: size limit exceeded" — unknown limit, agent can't adapt
"Create paste error: paste created" — no hostname, user can't open it
```

None of these serve the user or the agent. The MCP protocol is a tool-calling
protocol, not a public API — errors are diagnostic feedback to the agent, not
output sanitized for browser rendering.

## Alternatives considered

- **Full sanitization** (return generic codes, log all details server-side):
  Rejected. Makes the agent blind to actionable feedback and requires the user
  to check server logs for every failure, defeating the purpose of an
  autonomous agent.

- **Strip only hostname and MaxContentSize** (as initially proposed in the
  audit): Rejected after the above analysis. Hostname and MaxContentSize are
  operationally necessary, not sensitive.

## Consequences

- Error messages from MCP tools will include hostname, size limits, and
  default blocklist entries.
- User-configured `BLOCKED_PATHS` values will be sanitized from error messages.
- Future security reviews that flag error verbosity will be referred to this
  ADR. The ADR itself should be referenced in review checklists.
- This principle applies to this project only — other MCP servers may have
  different threat models and should make their own determination.

## Effective Date

2026-06-17
