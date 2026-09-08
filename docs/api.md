# API contract — 0.1.0 alpha

Dashboard/API share an origin (local: `http://localhost:8080`). JSON failures return `{ "error": "message" }`. Regular JSON bodies are limited to 64 KiB, with separate source archive limits. IDs are opaque strings. This API is provisional.

## Authentication

`GET /auth/status` returns `{configured,email}`. `POST /auth/setup` takes `{token,email,password}` once; password is 12–72 bytes. `POST /auth/login` takes `{email,password}`; `POST /auth/logout` invalidates the current session. Setup/login have a 20-attempt/15-minute per-IP limit.

Use the HttpOnly `cloudrail_session` cookie (24 hours, SameSite Strict, Secure on public installations). Mutations require `X-Cloudrail-Request: 1` and same-origin headers when supplied. The bootstrap Bearer token is not an owner API credential. Never put secrets in query strings.

## Workspace and deployment

| Method | Path | Body/result |
| --- | --- | --- |
| GET | `/api/state` | Projects, environments, services, deployments, actions and events; variable values excluded |
| POST | `/api/projects` | `{name}` → 201 project |
| POST | `/api/projects/:id/environments` | `{name}` → 201 environment |
| POST | `/api/projects/:id/services` | `{name,environment}` → 201 HTTP service; default environment `production` |
| POST | `/api/services/:id/deployments` | `{image,port,healthPath}` → 202 deployment; optional `Idempotency-Key` |
| POST | `/api/deployments/:id/cancel` | `{}` → cancellation request |
| GET | `/api/services/:id/variables` | `{names:[...]}`; never returns values |
| PUT | `/api/services/:id/variables/:name` | `{value}` for subsequent deployments |
| DELETE | `/api/services/:id/variables/:name` | Remove from future deployments |
| POST | `/api/services/:id/actions` | `{kind:"start"\|"stop"\|"restart"\|"backup"\|"restore",backupId?}` → 202 |

Images require `repository@sha256:<64 hex>`. Readiness accepts 2xx at an absolute path without query/fragment; redirects do not count. Reusing an idempotency key with the same request returns the original job; different content conflicts. Settings/variables are immutable snapshots per deployment. Redeploy creates a new ID and snapshot.

States: `queued → pulling → starting → checking → routing → active`; failures/cancellation end in `failed`, retired releases become `superseded`. A running cancellation cleans its candidate and restores the prior route. At most 10 nonterminal deployments per service. Database deployments are restricted to the pinned PostgreSQL template.

## Operations

| Method | Path | Body/result |
| --- | --- | --- |
| GET | `/api/node` | Identity, online/revoked status, observed host/container metrics |
| POST | `/api/node/revoke` | Revoke agent access; existing application traffic continues |
| PUT | `/api/services/:id/settings` | `{memoryMB,cpuMillis,mountPath}` for future deployments |
| PUT | `/api/services/:id/domain` | `{host}`; public mode checks DNS against server IPv4 |
| GET | `/api/services/:id/metrics` | Active container observations; missing values mean unknown |
| POST | `/api/projects/:id/databases` | `{name,environment}` → private PostgreSQL with queued first deployment |
| POST | `/api/services/:databaseId/bindings` | `{targetServiceId,variableName}`; private connection variable in the same project/environment |
| GET | `/api/backups` | Latest 100 IDs, service IDs, byte sizes, checksums and dates |
| GET | `/api/backups/:id/download` | Authenticated download: PostgreSQL `.dump`, volume `.tar` |

Memory: 64–4096 MB (database minimum 256); CPU: 100–4000 millicores. Attached volume path/identity is immutable. Stateful replacement stops the previous writer first. Database credentials are managed. Restore requires an empty target; volume targets must also be stopped.

## Source builds

| Method | Path | Body/result |
| --- | --- | --- |
| GET/PUT | `/api/github` | Redacted App state / configure `{appId,slug,privateKey,webhookSecret}` |
| GET | `/api/github/installations` | Installed App accounts |
| GET | `/api/github/repositories?installation=ID&page=1` | Installation repositories |
| GET | `/api/github/branches?installation=ID&repository=owner/repo&page=1` | Branches; 0 selects public access |
| GET/PUT | `/api/services/:id/source` | Source configuration below |
| GET | `/api/services/:id/builds` | History and bounded log tails |
| POST | `/api/services/:id/builds` | Queue configured branch SHA; optional `Idempotency-Key` |
| POST | `/api/builds/:id/cancel` | Cancel build |
| POST | `/webhooks/github` | Public, but requires valid GitHub HMAC signature and delivery/event headers |

Source: `{repository,installation,branch,root,builder,dockerfile,buildCommand,startCommand,port,healthPath,autoDeploy}`. Builder: `dockerfile` or `railpack`; installation 0 supports public repositories. Builds snapshot the exact commit/configuration. Runtime variables are excluded. See [source setup](source-builds.md).

## Private node protocol

`POST /enroll` uses the installation enrollment token over the private Compose management network and signs a CSR. The agent then uses TLS 1.3/client certificates on 8443. Every `/internal/*` request verifies the installation CA and current non-revoked identity; plaintext/Bearer-only access is rejected.

Internal routes cover heartbeat, deployment/action/build claims and reports, cancellation, reconciliation, backup metadata and authenticated source archives. Reports require `X-Cloudrail-Attempt`, matching the claim and deadline. Deployment/action jobs allow at most 3 claims within a 10-minute overall deadline. Builds have a 15-minute attempt/45-minute overall limit. One agent owns the persistent execution lock; this is not distributed scheduling.

Traefik routes directly to applications. Public installations expose 80/443; API/registry loopback ports, control database, BuildKit socket and internal TLS remain private.
