# Current verification — 2026-09-08

This record separates implemented/local behavior from account-bound production checks. Historical Phase 1 evidence remains in [verification.md](verification.md). The canonical completion tracker is [ROADMAP.md](ROADMAP.md).

## Recorded local evidence

| Area | Evidence and result |
| --- | --- |
| Go correctness | `go test -race ./...` and `go vet ./...` passed; real PostgreSQL tests run separately in isolated schemas |
| Owner/workspace | One-time setup, cookie auth, origin checks, encrypted variables, environment separation, start/stop/restart passed |
| Deployment | Real image → HTTP route, failed readiness/pull preserves old release, successful replacement, agent/API restart and history persistence passed |
| Reliability | mTLS-only node requests, heartbeat/revoke, cancelled candidate recovery, duplicate requests, stale attempts and bounded retries passed |
| Source | Real `traefik/whoami` Dockerfile and `railwayapp-templates/expressjs` Railpack builds published digests and served HTTP; failed Dockerfile build preserved serving release |
| GitHub handler | Signed HTTP payload/replay rejection, delivery deduplication and build cancellation/stale-attempt tests passed against isolated PostgreSQL schemas; real installed App delivery remains pending |
| PostgreSQL | Private network/no public port, persistent container replacement, encrypted same-environment binding, backup download/checksum, empty-target restore and nonempty rejection passed |
| HTTP volumes | Actual memory/CPU limits, immutable mount, data across redeploy, stopped-volume backup/restore to an empty target and preserved source passed |
| Networking/metrics | Actual container measurements, custom host route and separate local HTTPS proxy with a trusted test certificate passed; no public ACME issuance yet |
| Dashboard | Chrome desktop/mobile workspace, source history/GitHub settings and database creation/backup/binding journeys passed; final combined run: 3 passed in 42.8 seconds; screenshots visually inspected |
| Control backup | Dump plus key/identity backup restored into a separate temporary database; original workspace untouched. This is not a whole-host restore |
| Installer/AWS | Shell syntax, public Compose rendering and CloudFormation JSON parsed; AWS CLI availability-zone response shape checked. No authenticated AWS validation or clean Ubuntu install |
| Packaging | Source and Linux amd64/arm64 binaries/dashboard/notices generated locally. Archive checksums/notices and repeatability/manifest/credential checks passed (`scripts/verify-release.py`) |

The final dependency scan found reachable advisories in pgx v5.7.6 and x/text v0.24.0. They were updated to **pgx v5.9.2** and **x/text v0.39.0**. `govulncheck v1.7.0` then reported zero affected code paths and zero vulnerabilities in imported packages; it still lists advisories elsewhere in required modules that the application does not call. This is not an external security audit or a container-image scan. [Go pgx advisory](https://pkg.go.dev/vuln/GO-2026-5004), [Go x/text advisory](https://pkg.go.dev/vuln/GO-2026-5970). `npm audit` reported zero known vulnerabilities in the locked frontend dependency tree.

## Repeat the checks

```sh
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
npm ci --prefix apps/web
npm run build --prefix apps/web
npm audit --prefix apps/web
python3 scripts/verify-release.py
```

Database integration tests are skipped unless explicitly enabled. Supply an expendable PostgreSQL URL and a 64-hex test key through environment variables, then run:

```sh
CLOUDRAIL_INTEGRATION=1 go test -count=1 ./internal/deployment ./internal/builds
```

Each test creates/drops its own schema. The Docker acceptance scripts below target the local `cloudrail` stack and create test-owned projects; some intentionally restart/revoke the local agent/API. Use a development workspace, not a production node.

```sh
python3 scripts/phase2_acceptance.py
python3 scripts/acceptance.py
python3 scripts/phase3_acceptance.py
python3 scripts/phase4_acceptance.py
python3 scripts/phase5_acceptance.py
python3 scripts/volume-acceptance.py
python3 scripts/tls-acceptance.py
npm run test:browser --prefix apps/web
```

The owner helper creates a test owner only on an unconfigured installation and stores credentials privately in `.data/test-owner.json`; it never replaces an existing owner. Source/operations browser tests use `.data/phase4-result.json` and `.data/phase5-result.json` and skip when their fixture is absent. Workspace browser tests require a local owner fixture. Chrome is the configured channel.

## Resource observation and limits

The idle snapshot at 2026-09-08 06:06 UTC measured API 8.09 MiB, agent 7.73 MiB, control PostgreSQL 49.95 MiB, proxy 22.23 MiB, BuildKit 110.9 MiB and registry 12.51 MiB: about **211 MiB**. Application containers, Docker daemon and build peaks are excluded. This is not an AWS benchmark.

The local 30 GB Docker VM approached capacity during source builds. Only Cloudrail's dedicated BuildKit cache was pruned; unrelated Docker data was preserved. An isolated database test initially failed for disk exhaustion and passed after that cleanup. The completed Dockerfile/Railpack proofs were retained; repeated heavy builds were avoided after adding disk-reserve checks. Low disk can reject future builds even while existing apps keep serving.

## Unverified external gates

Real GitHub App installation/push delivery; AWS account/catalog/provisioning; public DNS/ACME/renewal; clean Ubuntu installation on both architectures; full host reboot/disaster recovery; cross-version upgrade; owner final UX review; actual billing observation; signed/versioned releases; consult [GitHub Actions](https://github.com/MinnKhantThuu/cloudrail/actions/workflows/verify.yml) for current hosted CI results. See [AWS pilot](aws-pilot.md) and [release gates](release.md). Source publication is tracked in the roadmap. No AWS resource was created by publishing the repository.

## Public repository and fresh-runner verification

Source and guides are public at [MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail). Anonymous repository access, raw README/guides/image contents and GitHub README rendering were checked. Private vulnerability reporting is enabled. Staged source and release manifests were scanned against local installation credentials; keys, backups, test logins and `.data` were excluded.

The first hosted run exposed two CI fixture issues: PostgreSQL health-command quoting and an assumed cached sample image. Both were corrected. [Run 34201850332](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34201850332), commit `a38a390`, passed the checks and runtime jobs: Go race/vet with isolated PostgreSQL, dependency scan, frontend build/audit, package checks/build, fresh Docker Compose startup, workspace/deployment/recovery acceptance and the Chrome workspace journey.

This is independent Linux amd64 Docker quickstart evidence on Ubuntu 24.04. It does not verify the public VPS installer, real GitHub App delivery, DNS/ACME, AWS billing or cross-version host update/recovery. The deploy-dialog copy was also checked locally against actual PostgreSQL/stateless service resource settings.
