# Security policy

Cloudrail is an early alpha for one owner and trusted repositories on one Linux node. It is not a public multi-tenant hosting service. Docker socket access gives the agent host-level control, and the privileged BuildKit worker executes trusted build instructions. Resource limits and separate networks are not a hostile-code security boundary.

## Report a vulnerability privately

Use GitHub's **Security → Report a vulnerability** for this repository:

[Open a private vulnerability report](https://github.com/MinnKhantThuu/cloudrail/security/advisories/new)

Include affected versions, reproduction steps, impact and a minimal proof where possible. Do not include real access keys or production data. If private reporting is unavailable, do not publish exploit details in Issues; contact the installation owner privately while the repository reporting channel is restored. There is no guaranteed response time.

## Supported version

The current `0.1.0-alpha.1` development line is the only supported line. No production-readiness guarantee or external security audit is claimed. Check [verification](docs/verification-current.md) and [release gates](docs/release.md) before running a public pilot.

The dashboard uses an owner password and same-origin HttpOnly sessions. Public installations require HTTPS. The first browser registration creates the installation owner without a setup token, so register immediately on a public server; further registrations are rejected atomically. Node enrollment tokens, encryption key, GitHub App key, database dumps, node identity and backups are private installation data. Keep them out of logs/source control and keep encrypted off-server recovery copies.

Application log redaction is best effort. Review logs before sharing them, and do not submit `.env`, private keys, owner credentials or backup archives in a public issue.
