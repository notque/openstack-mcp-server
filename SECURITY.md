<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please report vulnerabilities via one of these channels:

1. **GitHub Security Advisories**: Use the [Report a vulnerability](https://github.com/notque/openstack-mcp-server/security/advisories/new) feature
2. **Email**: Contact the maintainers directly (see CODEOWNERS)

### What to include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response timeline

- **Acknowledgment**: Within 48 hours
- **Assessment**: Within 7 days
- **Fix**: Dependent on severity (critical: ASAP, high: 14 days, medium: 30 days)

## Security Architecture

This project implements a three-layer security model:

1. **Read-Only Mode** (default): Mutating tools are not registered
2. **Tool Annotations**: MCP clients prompt users before destructive actions
3. **Credential Isolation**: Auth tokens and passwords never reach the LLM

See the [README](README.md#security) for full architecture details.

## Supported Versions

Only the latest release is supported with security updates.
