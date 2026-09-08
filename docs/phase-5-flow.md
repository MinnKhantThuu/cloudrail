# Phase 5 — operating a VPS

Implemented and locally verified after source-build verification. Public DNS/ACME evidence requires a user-owned domain; local TLS/routing checks are a separate result.

1. Service settings: validated domain, CPU/memory limits, one named persistent volume and deployment snapshots. Configuration applies to the next release. Once attached, volume identity/path stays fixed to prevent accidental data replacement.
2. Persistent releases use stop-before-start, with the old image retained for recovery; never run two writers against the same volume. Image rollback does not roll back database data/schema.
3. PostgreSQL 17 template: private environment network, named volume, generated credentials, readiness via pg_isready, stable private hostname. A connection variable can be copied into an HTTP service in the same project/environment without revealing the password in an API response.
4. Backups: PostgreSQL dump, immutable backup metadata/checksum, download to the owner, restore into an empty target with transaction/error checks. Volume exports require stopped workloads. No automatic deletion of persistent volumes.
5. Node metrics: actual host RAM/disk and tracked container usage/health, independent heartbeat during builds. Low disk blocks new builds/deployments; bounded log/container/build-cache retention keeps disk usage controlled.
6. Linux installer: Docker/Compose prerequisites, persistent secrets, preflight DNS/ports/disk, owner bootstrap over HTTPS, dashboard + app-domain Traefik HTTPS, ACME state on persistent volume. Upgrade backs up the control plane before migration.

Exit evidence: limits/volume persistence, private DB and variable binding, backup/empty-target restore, actual metrics, installer validation, TLS route test. A clean public VPS + DNS-issued certificate remains pending until a host/domain is supplied; do not label local Compose as a public deployment.
