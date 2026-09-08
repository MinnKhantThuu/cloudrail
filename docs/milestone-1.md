# M1: a safe first deployment

Accepted 2026-09-08. Working name: Cloudrail. Historical Phase 1 scope; the current executable module is named `cloudrail`. This milestone implements a local, single-admin, single-Linux-node development preview, not a production hosting service.

**Status:** Phase 1 prototype implemented and verified locally; final user UX review is still pending. See [verification record](verification.md) for recorded Docker/browser evidence. Follow the [master roadmap](ROADMAP.md) for the current checkpoint and next phase; the earlier M2 sequence below is historical guidance.

## Product acceptance

1. Start the stack with one documented command. Open a real React dashboard.
2. Create a project (with a production environment) and an HTTP service.
3. Submit a public OCI image pinned by SHA-256 digest, container port, and readiness path.
4. Follow persisted deployment events and bounded container logs in the dashboard.
5. Request the service through Traefik and receive the deployed application.
6. Submit a failing replacement. The previous app remains reachable and the UI reports the failure.
7. Restart the agent during a deployment. Resume the same deterministic container without creating duplicate releases.

## Scope and decisions

- Go API owns PostgreSQL. Go agent owns Docker operations and Traefik route files. React/Vite is served by the API from a production build.
- A deployment row is the durable job. One agent processes jobs in FIFO order. An exclusive filesystem lock prevents two local agent processes from operating concurrently; retain its named state volume across restarts. No distributed scheduler or lease takeover is claimed.
- **Transport adjustment:** local M1 uses outbound HTTP polling with a separate agent bearer token on an unexposed Compose management network. Browser/API authentication uses a separate admin token. Production node enrollment and mTLS transport remain M2 work; plaintext bootstrap tokens must not cross a public network.
- The agent retries unfinished jobs after restart. Container names derive from deployment IDs; route changes use atomic file replacement. Activation is persisted only after direct readiness and proxy verification of a release-specific response header. The previous release is retained until activation succeeds.
- A database/reporting outage is not a reason to stop the old container or mark the new deployment successful. Retrying reports is safe. Failure to restore a previous route leaves the candidate in place for retry/manual recovery.
- HTTP workloads only. No user-supplied shell commands, arbitrary host mounts, Docker socket mounts, privileged containers, variables/secrets, GitHub builds, public domains/TLS, volumes, or customer databases in M1.
- Application containers have memory, CPU, PID and log limits. They are trusted workloads; Docker is not a hostile-tenant sandbox. The API has no Docker socket. Traefik reads route files without a Docker socket.
- Routes use `<service-id>.localhost`, port 8088, and require no external DNS provider. API/UI port 8080 is bound to host loopback. PostgreSQL is not published.
- Keep old and new containers within node capacity. No general zero-downtime guarantee, especially for stateful applications, long-lived connections, or schema changes.

## Deployment lifecycle

`queued → pulling → starting → checking → routing → active`

Any pre-activation failure restores the previous route before `failed`. The old deployment becomes `superseded` only in the transaction that marks the new one active. A failed candidate never changes the database's active pointer. Event records survive process restarts.

## Work breakdown

1. Contracts/schema, validated API, persistence, auth and local Compose setup.
2. Docker adapter, resumable deployment runner and atomic Traefik routing.
3. Dashboard: projects, service list/detail, deployment form, events and logs.
4. Unit tests for validation/replacement safety; real Compose acceptance script with good and bad replacements and agent restart.
5. Record actual verification and remaining limitations in README.

## Earlier milestone outline — superseded by ROADMAP.md

- M2: node enrollment/mTLS, leases/fencing for multiple workers, reconciliation, ongoing health and failure alerts, cancellation, backups with restore drill, upgrade recovery.
- M3: GitHub App, signed/deduplicated webhooks, exact-commit Dockerfile builds, image registry, then Railpack.
- M4: encrypted variables, custom domains/TLS, metrics, tested restart/stop, usable installer, canvas and keyboard UX, limits/retention UX.
- M5: AWS deployment. Compare Lightsail to EC2 using measured app/build demand. ECR/S3/IAM and separate builders come after the local deployment lifecycle is proven.

## Required evidence

`go test ./...`, `go vet ./...`, TypeScript check and frontend build; a real Linux Docker acceptance run. Include unauthenticated rejection, valid deploy, failed readiness, invalid image pull, successful replacement, persisted history, and restart recovery. Browser review must verify real rendering and interactions separately from build success.
