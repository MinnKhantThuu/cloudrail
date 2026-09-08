# Implementation recommendations

> Earlier engineering notes. [ROADMAP.md](ROADMAP.md) now defines the phase order, project flow and progress. The numbered sections below are technical recommendations, not the current execution tracker.

Prepared 2026-09-08. Product direction is accepted. See [M1 execution plan](milestone-1.md) for the implementation boundary and [README](../README.md) for verification status. This is a historical proposal; current implementation status is in ROADMAP.md.

## Product boundary

ပထမဗားရှင်းကို ကိုယ်ပိုင်/ယုံကြည်ရတဲ့ repositories တွေအတွက် single-admin, single-node PaaS အဖြစ်စပါ။ Public signup, unrelated customer workloads, billing နဲ့ multi-node scheduling ကို နောက်မှထည့်ပါ။ ကိုယ်ပိုင် infrastructure product တည်ဆောက်ချင်တာဆို ဒီ direction သင့်တော်တယ်။ Hosting bill လျှော့ဖို့တစ်ခုတည်းဆို ကိုယ်တိုင် platform ထိန်းသိမ်းရမယ့်အချိန်ကိုပါ ထည့်တွက်သင့်တယ်။

## Proposed stack

- Go API + Go agent; React/TypeScript dashboard.
- PostgreSQL for application state and durable jobs; Redis is deferred.
- Docker + Traefik for the initial Linux runtime.
- Dockerfile builds first, then Railpack + BuildKit for supported repositories without a Dockerfile.
- M1 uses bounded browser polling and one outbound authenticated HTTP agent transport on the local private Compose network. Add browser SSE and production node enrollment/mTLS in M2; do not implement competing agent transports simultaneously.
- AWS integration after the local flow is verified. Keep AWS-specific implementations behind narrow interfaces; do not add empty ECS/Kubernetes implementations now.

Railpack uses BuildKit; verify/pin compatible versions when implementation begins: [Railpack getting started](https://railpack.com/getting-started/), [architecture](https://railpack.com/architecture/overview).

## Build order and acceptance criteria

### 1. Deploy a prebuilt image

- API, PostgreSQL, deployment worker, agent and Traefik on a local Linux VM or Docker environment.
- Create project → environment → service; deploy a prebuilt image pinned by digest.
- Persist deployment events; show status and redacted logs in a simple service list/detail UI.
- Acceptance: domain request returns the new app; a bad image or failed readiness check leaves the previous app serving.

### 2. Make deployment recovery reliable

- Persist deployment intent before dispatch; use stable deployment IDs and idempotent agent commands.
- Queue claims need leases, heartbeat, bounded retry/backoff and recovery after worker restart.
- Serialize deployment activation per service/environment; reject stale commands with a deployment generation.
- Reconcile desired and observed container/proxy state after agent reconnect.
- Start new container → readiness check → update route → verify externally → drain old connections → retire old container.
- Acceptance: agent disconnects, duplicated jobs, server restarts and overlapping deployments do not activate an obsolete deployment.
- Zero downtime is an acceptance goal for compatible stateless apps, not a guarantee from health checks alone. Allow capacity for old and new containers, and test long-lived connections.
- Image rollback does not reverse database migrations; require backward-compatible migrations and a separate restore plan.

### 3. Deploy from GitHub

- Add admin authentication before network exposure, GitHub App installation and repository selection.
- Verify webhook signatures, deduplicate deliveries, and build the exact commit SHA.
- Dockerfile → BuildKit → registry → image digest → existing deployment pipeline.
- Add Railpack afterward; keep explicit build/start overrides available.
- Acceptance: push to the configured branch updates the app; failed builds preserve the active version.

### 4. Reach the usable MVP

- Environment variables, generated domain, custom domain/HTTPS, redeploy, rollback, restart and stop.
- UI starts with service cards/list and detail tabs. Add the visual canvas after workflows are usable.
- CPU/RAM/PID limits, build concurrency/timeouts, disk thresholds, image retention and log rotation belong in the first usable release.
- Back up control-plane PostgreSQL and verify a restore before relying on the platform.
- Defer user database provisioning and persistent application volumes until their backup/restore behavior is designed.

### 5. Add AWS

- Begin with one on-demand EC2 instance for trusted personal workloads; isolate builders when build contention becomes significant.
- Add ECR, S3 backup, DNS and instance provisioning incrementally.
- Use instance roles instead of storing long-lived AWS keys in the dashboard.
- Track EC2, EBS, public IPv4, registry storage, backups and outbound data transfer. No monthly price is assumed here; estimate for the chosen region and measured workload.
- Add Spot only after interruption/retry behavior is verified for disposable builders or replaceable stateless capacity.

## Security boundaries that affect this design

- The agent controls the host Docker runtime and must be treated as privileged infrastructure. Do not give deployed apps the Docker socket, privileged mode or unrestricted host mounts.
- Docker daemon access is security-sensitive: [Docker Engine security](https://docs.docker.com/engine/security/), [protect daemon access](https://docs.docker.com/engine/security/protect-access/).
- Enroll agents with short-lived bootstrap tokens; issue, rotate and revoke node certificates. An outbound connection alone does not authenticate a node.
- Encrypt stored application secrets with a maintained authenticated-encryption/envelope library; keep the master key separate from the database and redact secrets from events/logs. Use build-secret mounts instead of baking secrets into image layers.
- Public multi-tenant hosting needs a separate isolation/threat-model review and stronger build isolation before accepting untrusted code.

## Decisions for implementation kickoff

1. Use Cloudrail and local module `cloudrail` provisionally. Select the final public module/repository path before publishing.
2. Define deployment state machine, events, retry/cancel semantics and agent command schema.
3. Design initial tables: users, projects, environments, services, deployments, deployment_events, nodes, jobs, variables and domains.
4. Define the minimum API contract and a local acceptance demo.
5. Pin toolchain/dependency versions and create the runnable development environment.
