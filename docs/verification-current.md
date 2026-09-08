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

Real GitHub App installation/push delivery; AWS account/catalog/provisioning; public DNS/ACME/renewal; clean Ubuntu installation on both architectures; full host reboot/disaster recovery; owner final UX review; actual billing observation; consult [GitHub Actions](https://github.com/MinnKhantThuu/cloudrail/actions/workflows/verify.yml) for current hosted CI results. See [AWS pilot](aws-pilot.md) and [release gates](release.md). Source publication is tracked in the roadmap. No AWS resource was created by publishing the repository.

## Public repository and fresh-runner verification

Source and guides are public at [MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail). Anonymous repository access, raw README/guides/image contents and GitHub README rendering were checked. Private vulnerability reporting is enabled. Staged source and release manifests were scanned against local installation credentials; keys, backups, test logins and `.data` were excluded.

The first hosted run exposed two CI fixture issues: PostgreSQL health-command quoting and an assumed cached sample image. Both were corrected. [Run 34201850332](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34201850332), commit `a38a390`, passed the checks and runtime jobs: Go race/vet with isolated PostgreSQL, dependency scan, frontend build/audit, package checks/build, fresh Docker Compose startup, workspace/deployment/recovery acceptance and the Chrome workspace journey.

This is independent Linux amd64 Docker quickstart evidence on Ubuntu 24.04. It does not verify the public VPS installer, real GitHub App delivery, DNS/ACME, AWS billing or cross-version host update/recovery. The deploy-dialog copy was also checked locally against actual PostgreSQL/stateless service resource settings.

## Login UX correction — 2026-09-08

First registration now accepts email/password/confirmation without an installation token and signs the owner in immediately. The existing email/password login and private node enrollment stay separate.

- `go test -race ./...`, `go vet ./...` and the frontend production build passed.
- `TestOwnerRegistration` passed against an isolated schema in the running PostgreSQL: cross-origin/missing-CSRF rejection, confirmation/password validation, concurrent registration (one 201, one 409), closed registration, secure session flags, logout invalidation and subsequent login.
- Chrome `owner-setup.spec.ts` passed (3.7s): real fresh registration, desktop/mobile form, show/hide password, mismatch feedback, automatic session, logout, wrong-password feedback, login and reload persistence. Both rendered layouts were inspected. The test skips configured installations; it never resets an owner.
- The existing dashboard/deployment Chrome journey also passed (32.4s), including normal login, deploy/failure handling and desktop/mobile behavior.
- [Hosted CI 34204399323](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34204399323) passed checks and runtime jobs at `a0cd2c29f5c243776b6a1dbe83db9d0d0edc7d9a`: isolated PostgreSQL tests, fresh browser registration, phase-2/deployment acceptance and the workspace browser journey on Ubuntu 24.04. Public VPS/AWS and final owner UX review remain pending.

## Guarded maintenance — 2026-09-08

[CI run 34205036825](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34205036825) passed its **maintenance** job at `30bfeb3`: a fresh Ubuntu runner installed public commit `a3df208`, deployed an app, then upgraded to alpha.2, restored the control backup to a separate database, refused rollback after actual DDL drift, restored exact previous images on compatible rollback, recovered the old API/agent after injected backup failure, and successfully updated again. New deployments passed after upgrade and rollback; an HTTP probe observed uninterrupted application availability throughout. This is a Linux amd64 Compose rehearsal, not the public HTTPS installer or complete host recovery.

That run's overall status was failed: checks discovered an acceptance helper as a unit test, and the runtime test asserted a stopped route before Traefik's asynchronous file watcher converged. Test selection is now explicit; the stop test checks the actual container is stopped, then bounds route convergence to 10 seconds while continuously checking the unaffected production route. [Follow-up CI 34205416387](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34205416387) passed all three jobs at `b4c5bd2`. The maintenance baseline now runs its general deployment acceptance; the follow-up also verifies encrypted variables survive update/rollback.

## Published alpha.2 — 2026-09-08

[Release v0.1.0-alpha.2](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.2) targets `b4c5bd21636a00a243f636305c3c66c7ae376fc3` and includes the source archive, Linux amd64 and arm64 runtime archives, and SHA256SUMS from successful CI run 34205416387. Every archive checksum was checked; all 158 source-manifest entries matched the exact Git commit. Runtime archives contain the expected ELF architecture, frontend assets, version and dependency notices. These structural checks do not claim both downloaded runtime archives were executed on fresh physical hosts.

GitHub build-provenance attestations for all four files were verified with `gh attestation verify`, enforcing the repository, `.github/workflows/verify.yml` signer, exact source commit and main ref. Remote release asset sizes/digests and anonymous source download are checked separately from CI. Current docs on main may be newer than the immutable version snapshot; release notes pin the authoritative build and CI evidence.

## Host recovery work — alpha.3

CI run [34207758703](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34207758703) at `8b272e8` passed checks, runtime and maintenance. The host-recovery job prepared real PostgreSQL/volume/registry fixtures, completed the cold backup, resumed the source platform and validated the backup inventory. Its replacement daemon failed to start because the rehearsal combined mutually exclusive Docker bridge flags; no successful destination recovery was claimed. The harness now uses no default bridge, a separate configuration/data root/containerd namespace and explicit empty image/volume checks. Archive helpers disable Docker logging so private tar streams are not duplicated into container logs. Follow-up destination recovery is pending.

CI run [34208239705](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34208239705) restored the platform/volumes on its replacement daemon, but application reconciliation did not complete. The custom daemon used a different socket while the platform agent mounts the standard host Docker socket; this single-runner harness did not prove workload recovery. The final harness now transfers an authenticated encrypted fixture between two separate Ubuntu runners, both using their normal Docker socket. The source workloads are fenced before transfer; no key or plaintext backup is uploaded. Independent-host verification remains pending until that workflow passes.

The first cross-runner attempt, [34209252278](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34209252278), refused transfer because the source agent recreated application containers while the harness removed them. The source fence now stops API/agent reconciliation before removing fixtures and verifies no owned workloads remain. The failure was detected before any encrypted backup transfer or destination start.

Run [34209939437](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34209939437) completed source fencing, authenticated encrypted transfer and the ordinary checks/runtime/maintenance jobs. The fresh destination refused recovery because the hosted VM images shared a Docker engine ID. Host identity now includes Linux kernel boot ID instead of assuming Docker IDs are globally unique; the destination test requires a different kernel, and the no-environment/no-containers/no-volumes guard still applies before loading data. No destination data was overwritten in the rejected attempt.
