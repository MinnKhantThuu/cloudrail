# Phase 3 — single-node reliability

Authorized continuation: user requested all phases in order on 2026-09-08.

- Persistent installation CA, private TLS listener, one enrolled execution node. First enrollment is restricted by a one-time bootstrap token over the private Compose management network. Subsequent RPCs require a client certificate; revocation is checked on every request. Enrollment is not intended over an untrusted remote HTTP link.
- Heartbeat every 5 seconds; offline after 20 seconds. Revocation stops new work; already serving containers remain until the operator stops them. Root access to the Docker host remains trusted.
- One execution process owns a persistent filesystem lock. A new claim gets a random attempt token and bounded attempt/deadline; all mutation reports require that token. No cross-host lease takeover or multi-node scheduler.
- Deploy requests use an idempotency key. Identical repeats return the original deployment; mismatched payloads conflict.
- Queued cancellation is immediate. Running cancellation waits for the agent to restore the previous route and remove the candidate; activation is rejected after cancellation. Network failure leaves the outcome visibly pending until reconciliation.
- Automatic resume is capped at three claims with exponential backoff and an overall deadline; exhaustion fails the job and authoritative idle reconciliation restores desired traffic. New attempts start only after the previous execution has stopped.
- Reconcile active routes and running/stopped containers after restart. Failed/superseded containers are cleaned up, and runtime logs remain bounded/redacted.
- Backup database plus encryption key and CA state; restore proof uses a separate temporary database, never overwrites the current workspace.

Acceptance: rejected plaintext/bearer internal RPC, enrolled mTLS heartbeat, revoke, duplicate requests, stale attempt reports, cancellation, bounded failure, restart reconciliation and independent DB restore.

Sources: [Go TLS](https://pkg.go.dev/crypto/tls#Config), [Docker host trust](https://docs.docker.com/engine/security/protect-access/).
