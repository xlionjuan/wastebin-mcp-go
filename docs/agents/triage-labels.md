# Triage Labels

Issue labels live in GitHub. Always read the current list with `gh label list`
before doing label-sensitive work.

## Known Labels

This table documents the expected label set, but GitHub is authoritative. If
`gh label list` differs from this table, use the live GitHub labels and mention
the documentation drift.

| Label | Meaning | Agent use |
| ----- | ------- | --------- |
| `bug` | Something is not working | Agents may apply when the issue describes a defect. |
| `documentation` | Documentation improvement | Agents may apply for docs-only work. |
| `duplicate` | Existing issue already covers this | Agents may suggest or apply only when the duplicate issue is clearly identified. |
| `enhancement` | New feature or request | Agents may apply for non-bug improvements. |
| `good first issue` | Good for newcomers | Agents may suggest; humans decide final suitability. |
| `help wanted` | Extra attention is needed | Agents may suggest; humans decide final suitability. |
| `invalid` | This doesn't seem right | Agents may suggest; humans should make final closure decisions. |
| `question` | Further information is requested | Agents may apply when asking the reporter for more information. |
| `wontfix` | This will not be worked on | Human decision label; agents may recommend but should not apply without explicit human approval. |
| `security` | Security problems | Agents may apply for security-relevant issues, but do not disclose unvalidated exploit details beyond what is needed for triage. |
| `technical-debt` | Technical debt and design cleanup | Agents may apply for refactoring and design improvement issues. |
| `code-quality` | Code quality and maintainability | Agents may apply for code quality concerns. |
| `performance` | Performance or resource-efficiency concerns | Agents may apply for performance-related issues. |
| `testing` | Tests, coverage, and regression protection | Agents may apply for testing improvements. |
| `maintainability` | Maintainability, structure, and refactoring candidates | Agents may apply for maintainability concerns. |
| `needs-investigation` | Needs deeper investigation before implementation | Agents may apply when the issue needs more research. |
| `mcp` | Model Context Protocol integration | Agents may apply for MCP-related changes. |
| `wastebin` | Wastebin integration and behavior | Agents may apply for wastebin-specific changes. |
| `error-handling` | Error taxonomy, wrapping, and reporting | Agents may apply for error-related changes. |
| `context-timeout-retry` | Context propagation, timeouts, cancellation, and retry behavior | Agents may apply for context/timeout/retry changes. |
| `joke` | It is joke | Agents may apply for humorous or off-topic entries. |

## Human-Only Decision Labels

The following labels are operator decision states. Agents must not apply, remove,
or change these labels unless the user explicitly asks for that exact label
operation. Agents may mention that one of these labels seems appropriate in a
comment or report.

| Label | Meaning |
| ----- | ------- |
| `accepted` | The claims are accepted and approved for action. |
| `needs-explain` | The human operator does not understand the claim or is not convinced yet. |
| `rejected` | The claims are explicitly rejected. |

## Role Mapping

Some skills use generic role names. Map them to this repo's labels as follows:

| Generic role | Use in this repo |
| ------------ | ---------------- |
| `needs-triage` | Do not apply a label automatically; leave unlabeled or use a topical label such as `bug`, `enhancement`, `documentation`, or `security`. |
| `needs-info` | Use `question` when asking for more information. |
| `ready-for-agent` | No direct label. Mention readiness in the issue/comment instead. |
| `ready-for-human` | No direct label. Mention that human judgment is needed instead. |
| `wontfix` | Use `wontfix` only with explicit human approval. |
