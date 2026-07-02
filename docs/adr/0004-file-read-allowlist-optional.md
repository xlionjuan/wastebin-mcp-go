# File-read allowlist is optional in default configuration

Issue #126 reported that `DefaultConfig()` enables file reads without an explicit
allowlist. We reviewed and decided not to change the behaviour: the built-in
blocklist already covers credential and system paths, users who configure
`ALLOWED_PATHS` are fully protected, and adding a mandatory opt-in override
would add configuration friction disproportionate to the residual risk. The
existing layered defence (blocklist + binary detection + content limits +
TOCTOU guards) is proportionate for self-hosted MCP deployments.
