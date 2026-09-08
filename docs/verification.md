# M1 verification — 2026-09-08

Verified against the real local stack on macOS ARM64 with a Colima Linux VM and Docker Engine 29.7.1. This is local Docker evidence, not an AWS deployment or a production load test.

## Checks completed

| Check | Result |
| --- | --- |
| Go 1.26.8 `go test -race ./...` | Passed; 11 top-level tests, including replacement safety, failed route recovery, lost acknowledgements, shutdown resumption, input/auth validation and proxy identity checks |
| `go vet ./...` | Passed |
| TypeScript and Vite production build | Passed |
| `npm audit` | Zero reported vulnerabilities after updating Vite to 7.3.6 |
| `scripts/dev-up.sh` | Full container build/start succeeded; API and PostgreSQL health checks passed |
| `python3 scripts/acceptance.py` | All real-runtime cases passed |
| `npm run test:browser` | One complete Chrome journey passed in 33.9 seconds; no JavaScript page errors |
| Desktop/mobile screenshots | Inspected at 1440px desktop and 390px mobile; no document horizontal overflow |

## Runtime evidence

The acceptance project `Acceptance 0908-103738` deployed the public image:

```text
traefik/whoami@sha256:200689790a0a0ea48ca45992e0450bc26ccab5307375b41c84dfc4f2475937ab
```

Verified:

1. Admin and agent credentials cannot be interchanged; unauthenticated state requests are rejected.
2. Mutable image tags are rejected before queueing.
3. A pinned image serves actual HTTP through Traefik, with the expected release response header.
4. During and after a replacement's failed readiness check, the previous app remains reachable. Failed candidate logs are retained.
5. An invalid digest fails the image pull and preserves the active route.
6. Restarting the agent during readiness resumes the job; it fails cleanly and removes its candidate while the old release serves.
7. A healthy replacement activates, then the previous container stops and its history becomes superseded.
8. Restarting the API preserves the active pointer, history and application traffic.

The browser journey separately created a project/service, exercised server-side validation, deployed a real image, opened the actual application URL, inspected runtime logs, submitted a failing replacement, filtered services, inspected mobile layout and dismissed the mobile deploy dialog with Escape.

## Visual evidence

Local screenshots are generated under `.data/screenshots/` (ignored by Git):

- [Dashboard](../.data/screenshots/dashboard.png)
- [Failed deployment](../.data/screenshots/failed-deployment.png)
- [Mobile](../.data/screenshots/mobile.png)
- [Sign-in](../.data/screenshots/login.png)

The local acceptance output is retained in `.data/acceptance.log`.

## Resource observation

An idle `docker stats --no-stream` snapshot after the small demo reported:

| Platform container | Memory |
| --- | ---: |
| Go API | 5.805 MiB |
| Go agent | 6.555 MiB |
| PostgreSQL | 29.82 MiB |
| Traefik | 15.79 MiB |

Total approximately **58 MiB** for these four containers at that instant. This excludes the Linux VM/OS, Docker daemon, deployed apps, filesystem cache accounting differences and any image/build peak. It is not a minimum RAM recommendation or a comparison benchmark against Coolify/Laravel.

## Issues found by live checks

- Docker's internal-only management network did not publish the dashboard port. Added a separate dashboard ingress network while keeping PostgreSQL and app workloads separate.
- Traefik ignored `.json` route files. Atomic route files now use `.yaml`; their JSON encoding is valid YAML, and real proxy requests verify the resulting route.
- This host has standalone Docker Compose without Buildx. Startup detects that configuration and uses the available classic Docker builder.

## Remaining scope

No AWS resources, public TLS/DNS, GitHub builds, metrics, encrypted user variables, persistent application databases, backups/restores or multi-node enrollment have been implemented or verified. Route restoration failure and lost activation acknowledgement were checked with unit fault injection, not by disrupting production infrastructure. Whole-node loss, simultaneous distributed agents, hostile workloads and database migration rollback are outside M1.

The development stack remains running for review. Test projects are retained. No remote repository or commit/push has been created.
