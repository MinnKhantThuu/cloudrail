# Current verification — 2026-09-11

This record separates implemented/local behavior from account-bound production checks. Historical Phase 1 evidence remains in [verification.md](verification.md). The canonical completion tracker is [ROADMAP.md](ROADMAP.md).

## Public canvas update — 2026-09-09

The public Linode was updated with the guarded maintenance workflow from the packaged alpha.4 checkout to verified main commit `c15158aaef06adc3eb1e76b3ed035991b3c5825a`. The update saved recovery record `.data/updates/20260909T034313Z-6f1ade5a`, rebuilt the control services and returned both server and agent healthy. It retained the owner/session data and the active application deployment `00ce3a973726fcfac2e49c17`.

Fresh public checks on 2026-09-09 returned HTTP/2 200 from the dashboard and sample application. Authenticated API checks returned one project, two services, one deployment, an online node and two resources on the production canvas. A real browser reload showed the pan/zoom project canvas and the complete create palette for GitHub, Docker, empty service, worker, cron, PostgreSQL, Redis, MySQL, MongoDB, volume and S3-compatible bucket resources. This is public deployment evidence from before the later observation outage; user UX acceptance and a versioned post-alpha.4 release remain separate.

## Linode 48-hour observation — final result 2026-09-11

The observation ran for 48 hours 8 minutes from `2026-09-09T03:30:04Z`. It included the guarded update at `2026-09-09T03:43:13Z`. The initial and 30-hour samples returned dashboard/application HTTP/2 200 with active deployment `00ce3a973726fcfac2e49c17`; all seven containers were running. Over those samples root use moved from 8.9 GiB (12%) to 9.3 GiB (13%), used host memory from 700 MiB to 785 MiB, available memory from 3,214 MiB to 3,130 MiB, and swap stayed unused. At the 30-hour sample, container memory readings totaled about 166 MiB.

The 36-hour, 42-hour and final 48-hour samples could not connect to either HTTPS endpoint or SSH; all timed out while DNS still resolved to `172.104.38.63`. This is a failed availability observation and does not identify whether the cause was host power, firewall or provider networking. The final Linode billing view required a fresh account login, so no authenticated actual-charge evidence was available. Phase 6.3 requires host/network recovery, state verification, an actual billing record and a clean replacement observation before it can pass.

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
| Networking/metrics | Local route/TLS tests passed; Linode Cloud Firewall 22/80/443, public HTTP/2 dashboard and trusted Let's Encrypt issuance now verified |
| Dashboard | Chrome desktop/mobile workspace, source history/GitHub settings and database creation/backup/binding journeys passed; final combined run: 3 passed in 42.8 seconds; screenshots visually inspected |
| Control backup | Dump plus key/identity backup restored into a separate temporary database; original workspace untouched. This is not a whole-host restore |
| Installer/AWS | Unmodified installer passed on a clean Ubuntu 24.04 Linode amd64 host with real DNS/ACME. AWS remains an optional unverified provider path |
| Packaging | Source and Linux amd64/arm64 binaries/dashboard/notices generated locally. Archive checksums/notices and repeatability/manifest/credential checks passed (`scripts/verify-release.py`) |

## Railway-inspired resource model — 2026-09-09

UX-1 adds backward-compatible service classification, normalized volume/attachment records, future bucket records, service references, server-persisted canvas positions and an environment-scoped canvas graph API. Existing `001–005` application, PostgreSQL, source and volume records were migrated in an isolated legacy-schema test without loss. A separate authenticated API integration test created an application, PostgreSQL service, two volumes and a database binding, verified four resource nodes and three real links, persisted a layout and rejected an unknown resource key.

Targeted tests and the full non-race Go integration suite plus `go vet ./...` passed against disposable PostgreSQL. A local full race attempt could not create its isolated schemas after the larger Debian Go image filled the Docker VM; this was an environment disk failure before assertions. Hosted [CI run 34278082750](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34278082750) subsequently passed Go race/vet, frontend, runtime, migration-aware maintenance and independent-host recovery jobs at `92e7314`. This substep originally had no public-host evidence; the later public canvas update is recorded above.

## Railway-inspired canvas shell — 2026-09-09

UX-2 replaces the default service card grid with the environment canvas graph. It supports pan, wheel/button zoom, fit, draggable nodes with server-persisted positions, URL-backed selection, real reference/attachment edges, resource drawers and one create palette opened from the header, canvas, right-click or `Cmd/Ctrl + K`. Existing application, PostgreSQL and volume resources are clickable. Data/storage types that are not implemented remain visibly disabled and labeled with their planned phase instead of acting as dead controls.

`npm run build` passed. Two Chrome journeys against a mocked control API passed in 2.3 seconds: desktop resource/edge/drawer navigation, keyboard/context creation, layout save and refresh restoration; and a 390×844 full-screen volume drawer with no horizontal overflow. Desktop/mobile screenshots were inspected. Existing real-stack browser selectors were updated for the new canvas flow, but those journeys were not rerun in this local phase. The later public update now serves this canvas; user acceptance remains pending.

## Generic compute creation — UX-3.1

The service API now stores `sourceType` (`github`, `image`, `empty`) separately from `workloadMode` (`web`, `worker`, `cron`). The authenticated generic resource endpoint validates both axes, creates the resource atomically and keeps the old `/services` behavior as an empty web-service compatibility path. Service state includes resource/workload/template metadata; GitHub source saves explicitly switch the source record to GitHub.

The isolated authenticated PostgreSQL API test created GitHub worker and image cron resources, rejected an invalid workload and verified their canvas projection/source metadata. Full non-race Go tests, `go vet`, frontend production build and the updated mocked Chrome canvas journeys passed. This proves resource creation and presentation only; worker execution, cron scheduling and their real-container acceptance remain UX-3.2/3.3.

Hosted [CI run 34279315566](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34279315566) passed the complete checks/runtime/maintenance/recovery workflow for UX-3.1 at `07d6791`.

## Route-free worker runtime — UX-3.2

The agent checks that a worker process stays running instead of probing HTTP, removes any stale public route, switches to a healthy candidate before stopping the prior worker and preserves that prior process on candidate failure. Start, stop, restart and reconciliation use the same route-free behavior. Unit tests cover worker activation/failure ordering and the proxy refuses to write a worker route. The mocked Chrome canvas journey opens a live worker node and verifies its route-free active status. A disposable real-Docker acceptance creates an image worker, verifies route absence, injects a pull failure, exercises lifecycle actions and proves agent restart recovery after the worker container is stopped while the agent is offline.

Hosted [CI run 34280088388](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34280088388) passed checks, runtime, maintenance, backup and independent-host recovery for UX-3.2 at `71d9f5c`. The stronger stopped-container recovery assertion is queued with the completion checkpoint commit.

## Scheduled cron runtime — UX-3.3

Cron services now store a validated five-field UTC schedule and calculated next run. The transactional claimer pins each occurrence to the active deployment, advances the next occurrence past the current time, allows one unfinished run per service and retries the same run identity after interruption. The Docker runtime creates an isolated no-restart container, records bounded stdout/stderr and exit code, and can resume a created, running or exited container after an agent restart. Completed containers are removed opportunistically.

The Settings drawer exposes schedule editing, next execution and run history. Full Go tests, `go vet`, the isolated PostgreSQL API/claim/history tests, frontend build and two mocked Chrome canvas journeys passed locally. The cron drawer screenshot was visually inspected. A disposable CI acceptance forced a due execution with the pinned `hello-world` image, verified no public route and injected an exited run while the agent was offline to prove restart recovery.

Hosted [CI run 34282653529](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34282653529) passed race tests, vet, dependency checks, frontend build, runtime browser/real-Docker acceptance, maintenance, backup and independent-host recovery for UX-3.3 at `6f7c76e`.

## Deployment commands and restart policy — UX-3.4 complete

Compute settings now validate and snapshot a shell start-command override, isolated pre-deploy command with a 1–3600 second timeout, and `on-failure`, `always` or `never` restart behavior. GitHub build deployments copy the build's start-command snapshot. Docker overrides the image entrypoint only when a command is supplied; pre-deploy receives deployment variables and private networking but no service volume. A failed or timed-out pre-deploy records its bounded logs, removes its temporary container and restores the prior release.

The Settings drawer edits these values and each deployment card shows its command/restart snapshot. Full Go tests/vet, isolated PostgreSQL API and snapshot tests, frontend build and mocked Chrome interactions pass locally. The hosted matrix uses pinned Alpine containers to verify the command, pre-deploy success/failure/timeout, active-release preservation and all three Docker restart-policy mappings. Its first run exposed a missing upgrade for the deployment status constraint before any candidate container started; migration 008 and a legacy-schema upgrade assertion now cover that path. The next run passed command/pre-deploy behavior, then exposed an acceptance race because Cloudrail acknowledges the healthy replacement before stopping the old worker to avoid a gap; the assertion now waits a bounded interval for that cleanup ordering.

Hosted [CI run 34286036519](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34286036519) passed race tests, vet, dependency checks, frontend build, owner/workspace browser journeys, all real-Docker deployment controls, maintenance, backup and independent-host recovery at `de3a228`. UX-3.4 and UX-3 are complete. Public Linode deployment and user UX acceptance remain separate.

## UX-4.1 versioned data templates and Redis — 2026-09-09

The data-service path now reads from a versioned built-in template registry instead of a PostgreSQL-only branch. The public catalog exposes safe metadata only. Service records snapshot the template key/version, pin the tested image/port and reject ad hoc template image replacement. PostgreSQL 17.6 remains backward compatible; Redis 8.2.2 creates generated encrypted credentials, `/data` storage, append-only persistence, private network alias and `REDIS_URL` application references.

Local isolated-schema API/migration tests and the production frontend build passed. Mocked Chrome desktop/mobile tests created Redis from the canvas and the resulting screenshot was inspected. Hosted [CI run 34288941666](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34288941666) passed all five jobs. Its Redis acceptance verified the pinned container became healthy without a public port, catalog/create/variable APIs exposed no credential value, application binding and canvas link existed, the named volume was mounted and a written key survived an explicit container stop plus agent restart. The public host now contains this code, but no public Redis workload was added; the real Redis runtime proof remains the recorded CI run.

## UX-4.2 first-class volumes — 2026-09-09

Canvas volume nodes can be created, attached to compatible services at a validated absolute path and detached only while the service is stopped. The store enforces one volume per service, one writer per volume and template-volume ownership. A real-container acceptance wrote data through an attached application volume, redeployed the writer, verified the data survived, rejected unsafe detach and cross-writer cases, then moved a detached volume to a compatible target.

Hosted [CI run 34290515948](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34290515948) passed all five jobs. This is volume-runtime proof on a disposable runner; the public host now contains the code but no public volume workload was added.

## UX-4.3 portable Redis and application-volume recovery — 2026-09-09

Stopped Redis services and stopped HTTP services with an attached volume can create downloadable tar backups. Restore requires a stopped, empty, compatible target; Redis also requires the same template version. The archive reader rejects traversal and unsupported entries, extraction is bounded, and a nonempty Redis target is checked before fresh initialization files are replaced.

Hosted [CI run 34292131015](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34292131015) passed all five jobs. Its real Redis proof restored a key into a fresh stopped target and rejected a second restore while preserving the existing key.

## UX-4.4 S3-compatible bucket connection — 2026-09-09

The canvas can record an existing S3-compatible bucket, bind six prefixed variables to a same-environment HTTP application, rotate the encrypted credential across bindings and disconnect the managed variables/edge. Create and rotate responses contain safe metadata only. Local isolated-schema API integration, frontend production build and mocked desktop/mobile Chrome tests passed, and the bucket settings screenshot was inspected.

Hosted [CI run 34294856835](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34294856835) passed all five jobs. Its runtime job used the bound variables from a deployed worker to upload/download an object through MinIO, restarted the provider with replacement credentials and proved the old login failed, rotated Cloudrail's encrypted credential, redeployed and read the retained object, then disconnected/redeployed and verified the managed environment plus canvas edge were gone. Cloudrail does not provision or delete provider buckets/objects. The public host now contains the bucket UI/API code, but no public bucket provider was connected.

## UX-4.5a MySQL template — verified 2026-09-09

The versioned template catalog and canvas include MySQL 8.4.7 with a digest-pinned image, generated encrypted app/root credentials, a private `MYSQL_URL` binding, Docker health check and managed `/var/lib/mysql` volume. Logical backup and same-version empty-target restore use the container's MySQL tools. CI run [34298629858](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34298629858) passed all five jobs. Its disposable runtime created and bound a private MySQL service, retained a real InnoDB row through restart, restored its logical dump into a fresh same-version target, rejected a second restore after the target became nonempty, and preserved the restored row. Race tests, vet, vulnerability scan, frontend build/audit, bucket regression, browser workspace journey, maintenance upgrade and independent-host recovery also passed.

## UX-4.5b MongoDB template — verified 2026-09-09

The versioned template catalog and canvas include MongoDB 8.0.29 with a digest-pinned image, generated encrypted root credentials, a private `MONGO_URL` binding, authenticated Docker health check and managed `/data/db` volume. Logical archive backup and same-version empty-target restore use `mongodump` and `mongorestore` from the pinned image. CI run [34300250908](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34300250908) passed all five jobs. Its disposable runtime created and bound a private MongoDB service, retained a real document through restart, restored its logical archive into a fresh same-version target, rejected a second restore after the target had a collection, and preserved the restored document. The same run passed race tests, vet, vulnerability scan, frontend build/audit, all earlier data/storage regressions, browser workspace, maintenance upgrade and independent-host recovery.

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
python3 scripts/redis-backup-acceptance.py
python3 scripts/bucket-acceptance.py
python3 scripts/tls-acceptance.py
npm run test:browser --prefix apps/web
```

The owner helper creates a test owner only on an unconfigured installation and stores credentials privately in `.data/test-owner.json`; it never replaces an existing owner. Source/operations browser tests use `.data/phase4-result.json` and `.data/phase5-result.json` and skip when their fixture is absent. Workspace browser tests require a local owner fixture. Chrome is the configured channel.

## Resource observation and limits

The idle snapshot at 2026-09-08 06:06 UTC measured API 8.09 MiB, agent 7.73 MiB, control PostgreSQL 49.95 MiB, proxy 22.23 MiB, BuildKit 110.9 MiB and registry 12.51 MiB: about **211 MiB**. Application containers, Docker daemon and build peaks are excluded. This is not an AWS benchmark.

The local 30 GB Docker VM approached capacity during source builds. Only Cloudrail's dedicated BuildKit cache was pruned; unrelated Docker data was preserved. An isolated database test initially failed for disk exhaustion and passed after that cleanup. The completed Dockerfile/Railpack proofs were retained; repeated heavy builds were avoided after adding disk-reserve checks. Low disk can reject future builds even while existing apps keep serving.

## Unverified external gates

Real GitHub App installation/push delivery; ACME renewal; clean public installation on arm64; owner final UX review; a successful 48-hour billing/resource observation; and the optional AWS provider path remain unverified. The first 48-hour window elapsed but failed availability at its 36-hour, 42-hour and final samples, and no authenticated Linode charge was available. Public DNS, first ACME issuance, clean Ubuntu 24.04 amd64 installation, owner secure-session use, public workload deployment, reboot recovery and real separate-host recovery had passed before that outage. Consult [GitHub Actions](https://github.com/MinnKhantThuu/cloudrail/actions/workflows/verify.yml) for hosted CI results and [release gates](release.md) for the remaining scope.

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

Run [34210689983](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34210689983) passed source backup and the other three jobs, but destination envelope authentication failed before restore. The transfer now verifies a source-side decrypt/byte comparison and checks the exact encrypted file SHA-256 again on the destination before decryption; these checks distinguish source sealing from transfer integrity. No destination restore success is claimed.

[Run 34211555644](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34211555644) at `85bbdb3` passed all five jobs. The source encrypted fixture decrypted byte-for-byte before upload; the destination verified its exact SHA-256 and authenticated decryption, loaded the retained platform images and restored onto a different Ubuntu VM/kernel and empty Docker storage. Owner/session, project/service IDs, PostgreSQL proof rows, private networking, application files, registry digest, backup checksum, certificate-volume fixture, encrypted database binding and stopped-service state passed. A new deployment served HTTP afterward. The temporary encrypted artifact was deleted successfully. This is Linux amd64 independent-host recovery evidence; public installer, actual DNS/ACME and AWS are still pending. The preceding authentication failure was not reproduced and its original cause remains undetermined.

Final alpha.3 candidate also rejects composed symlink escapes, symlink cycles and archive-member parent traversal; portable internal symlink chains remain supported. Targeted archive-boundary tests passed locally. The transfer secret is scoped only to decrypting the fixture, outside the restore/application step. Final candidate CI/package verification passed as recorded below.

## Published alpha.3 — 2026-09-08

[Release v0.1.0-alpha.3](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.3) targets `991798dfb339173caaef4e34b613fdd19d2cd492`. [CI run 34212164749](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34212164749) passed all five jobs, including the final archive guards and separate secret-scoped decrypt step. All four CI assets passed checksum and GitHub provenance verification against the exact source commit, main ref and expected verify workflow. All 165 source-manifest entries matched Git; Linux amd64/arm64 ELF architecture, VERSION, dashboard assets and dependency notices were checked. Anonymous GitHub API/source download, the exact remote tag and all four published asset digests also passed. Structural runtime archive checks do not claim fresh-host execution on both architectures.

AWS CLI has no configured profiles. Public installer/DNS/ACME, actual GitHub App delivery, AWS pilot/billing and owner final UX acceptance remain outstanding. The handed-over local owner-registration workspace was not rebuilt or used for these tests.

## Public dashboard recovery correction — alpha.4 candidate

Source audit found that alpha.3 cold recovery intentionally excludes generated route storage, but the agent only rebuilds application routes; the public dashboard route was missing. Alpha.4 generates it from the retained Compose dashboard domain and copies it into the restored route volume after the agent starts. Installer and restore share the route generator. Six local boundary tests pass, including domain validation and dashboard backend/TLS configuration. The independent-host job now also exercises that same route restoration function with the real restored API/dashboard behind a separate Traefik proxy and explicitly trusted local certificate. Hosted results passed as recorded below. Real ACME/public-host recovery remains an external gate.

## Published alpha.4 — 2026-09-08

[Release v0.1.0-alpha.4](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.4) targets `302aec491fc33fd8a0ef5216b33856d899e5cb5f`. [Run 34213340630](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34213340630) passed all five jobs. The destination restored platform/application data, then exercised the actual dashboard-route restoration function against the retained route volume and real API/dashboard through Traefik. A trusted fixture TLS certificate served configured-owner status and dashboard HTML. No ACME resolver was configured for that isolated proxy, so it made no public certificate request. This proves route regeneration/routing, not a full public-mode restore, public secure-cookie login or real DNS/ACME.

All four release assets passed SHA-256 and exact source-commit/main-ref/workflow provenance verification. The source archive had 167 entries matching Git; runtime ELF architecture, version, web assets and notices were verified. Anonymous release/source download, remote tag and all published asset digests matched the candidate. Alpha.3 release notes now document its dashboard-route omission and link to the repair guide; its original assets were retained.

The alpha.4 [tag-triggered CI run 34213842668](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34213842668) also completed all five jobs successfully at the same release commit. The completion audit in [ROADMAP](ROADMAP.md#9-completion-audit-and-external-block--2026-09-08) records the remaining target-bound gates and read-only confirmation that AWS credentials, local App configuration and public domains are unavailable. No real pilot or owner acceptance is inferred from CI success.

## Linode pilot preflight — 2026-09-08

The owner selected `172.104.38.63` as the first public pilot, replacing AWS for this run. TCP 22 accepted a connection. TCP 80/443 and direct HTTP/HTTPS timed out, and reverse DNS returned no record. A read-only `root` SSH attempt with the available local identity reached the SSH service but was rejected with public-key authentication; therefore OS, capacity, Docker and firewall state remain unverified. No package, DNS, firewall or server data was changed. A dedicated local Ed25519 pilot key was generated under ignored `.data` storage for explicit authorization. The public half may be installed by the owner; the private half must never enter source control or chat.

## Linode alpha.4 public install — 2026-09-08

The owner authorized root recovery, the dedicated SSH identity and HTTP/HTTPS firewall rules. Root access was recovered through LISH, the key was installed with mode-restricted SSH files, and a batch SSH request succeeded after independently matching the live Ed25519 host fingerprint to the boot-console fingerprint.

The target is Ubuntu 24.04.4 x86-64 on a Linode 4 GB plan: 2 CPU, 3.8 GiB usable RAM, 79 GiB root filesystem with 72 GiB initially free. Cloud Firewall ID `159893558` accepts TCP 22, 80 and 443 plus ICMP; its default inbound policy remains drop. After the rule update, ports 80/443 changed from timeout to immediate refusal before a listener started, proving the network path without confusing it with application readiness.

Docker Engine 29.8.0 (API 1.56) and Compose 5.5.1 were installed from Docker's Ubuntu repository. Exact alpha.4 commit `302aec491fc33fd8a0ef5216b33856d899e5cb5f` was checked out under `/opt/cloudrail`. The unmodified public installer passed OS, architecture, DNS, RAM and disk preflight using `console.172-104-38-63.sslip.io` and `apps.172-104-38-63.sslip.io`, then built and started PostgreSQL, server, agent, proxy, registry and BuildKit. PostgreSQL, server and agent reported healthy.

The public dashboard returned HTTP/2 200. Its certificate has subject/SAN `console.172-104-38-63.sslip.io`, issuer Let's Encrypt YR1 and validity 2026-09-08 through 2026-12-07. `/auth/status` returned `configured:false`, which proves the one-time owner screen remains available; it is not owner login acceptance. The first no-load container snapshot totaled about 82 MiB (API 4.66, agent 4.06, PostgreSQL 30.59, proxy 17.12, registry 15.02 and BuildKit 10.52 MiB). Docker daemon, kernel cache, applications and build peaks are excluded.

This closes the first-host access, firewall, clean Ubuntu amd64 installer and real ACME issuance gates. Temporary `sslip.io` DNS is suitable for the pilot but is not a user-owned production domain.

## Linode owner, workload, reboot and backup — 2026-09-08

The owner account was created through the public HTTPS API and the first browser reload showed the returning-owner login state. The session cookie used `HttpOnly`, `Secure` and `SameSite=Strict`; authenticated state was available before and after reboot. Credentials and cookies remain only in ignored mode-restricted local storage.

A public Railpack build of `railwayapp-templates/expressjs` produced project `2433c1a853804a005d28db20`, service `037b9c4141bba7f16d897963` and active deployment `00ce3a973726fcfac2e49c17`. Its public HTTPS endpoint returned HTTP 200, body `{"body":"Hello world!"}` and the matching `X-Cloudrail-Deployment` header. Its Let's Encrypt certificate is valid for the generated application hostname. An intentional build configured with a missing Dockerfile failed, the original source configuration was restored, and the active endpoint continued returning the same body and deployment ID.

A real host reboot changed the kernel boot ID. The six platform containers and active application returned; server/PostgreSQL were healthy, the authenticated state retained the same IDs and both public endpoints returned HTTP 200. This proves retained control/application state and route regeneration for this pilot. It does not prove certificate renewal or destination-host recovery.

The cold host-backup command stopped the platform writers, captured 503 MiB and resumed the original platform. Inventory, checksums and archive boundaries passed on the source and again after SSH-encrypted transfer to ignored off-server storage. Source and destination manifest SHA-256 both equal `b077db3b82402bef058753af1440019eef77947208acf364a6372f1c0a1796c5`. All platform/application containers and both public HTTP endpoints recovered after backup.

The backup was then copied to temporary Singapore Linode `104604476` at `172.104.183.192`. It was a separate Ubuntu 24.04.4 amd64 kernel with empty Docker storage, 2 CPU, 3.8 GiB usable RAM, encrypted disk and the exact alpha.4 source. The source host was fenced before restore. `host-recovery.py restore` loaded the retained platform images and volumes, started the platform/agent, regenerated the dashboard route and returned PASS. Direct DNS overrides proved the restored owner session, original project/service/deployment IDs, public app body/header and retained Let's Encrypt certificate. A post-restore source build also completed and served new active deployment `d41f0e2223a08071cf0617e3`.

The restored destination was fenced before resuming the source. The source returned to the original active deployment `00ce3a973726fcfac2e49c17` with authenticated dashboard and application HTTP 200. The temporary Linode was deleted after acceptance. This proves a real same-architecture public-host restore without a DNS cutover; it does not prove certificate renewal or arm64 recovery.
