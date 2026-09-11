# Linode public pilot

Status, 2026-09-11: preparation, public control-plane install and Railway-inspired canvas update completed on target `172.104.38.63`. The 48-hour observation window elapsed, but the observer could not reach dashboard/app HTTPS or SSH during the final 12 hours of sampled checks. The successful-observation gate therefore remains open pending host/network recovery and a clean rerun. The 4 GB Singapore Linode was last measured running Ubuntu 24.04.4 x86-64 with 2 CPU and a 79 GiB root disk. A dedicated root SSH identity was authorized and verified against the console-published host fingerprint. Cloud Firewall was configured to allow TCP 22/80/443 with default inbound drop. Docker 29.8.0/API 1.56 and Compose 5.5.1 were installed.

Exact alpha.4 commit `302aec491fc33fd8a0ef5216b33856d899e5cb5f` passed the original installer preflight. The guarded updater then moved the live checkout to verified main commit `c15158aaef06adc3eb1e76b3ed035991b3c5825a`, retaining the owner, session, database and active workload. Before the later reachability failure, the dashboard at `https://console.172-104-38-63.sslip.io` returned HTTP/2 200 with a trusted Let's Encrypt YR1 certificate and exposed the project canvas/resource palette in an authenticated browser. The Railpack sample at `https://037b9c4141bba7f16d897963.apps.172-104-38-63.sslip.io` returned deployment `00ce3a973726fcfac2e49c17`. A verified 503 MiB cold backup restored successfully on a separate empty Linode and accepted a new deployment. User UX acceptance, a new versioned release, GitHub App delivery, certificate renewal, actual billing evidence and a successful replacement observation remain pending.

## Access handoff

The project-specific public key is present in `/root/.ssh/authorized_keys`. The private half and generated recovery credential remain in ignored local `.data` storage with mode `0600`; they must never enter source control or chat.

The initial verified connection recorded:

- `/etc/os-release`, architecture and kernel;
- CPU count, RAM and free root/Docker disk;
- Docker/Compose availability;
- listening ports and active host firewall policy.

The supported installer target is Ubuntu 24.04 or 26.04, amd64/arm64, at least 4 GB RAM and at least 8 GB free Docker disk (20 GB recommended). This host passed those checks; no resize or replacement purchase was needed.

## Pilot DNS used for installation

- Dashboard hostname: `console.172-104-38-63.sslip.io`.
- Wildcard-compatible application suffix: `apps.172-104-38-63.sslip.io`.
- Both resolve directly to `172.104.38.63`; the installer preflight passed.
- The contact email came from the repository operator configuration and is retained only in the private installation environment.

`sslip.io` provides temporary IP-based public DNS for this pilot. Move to a user-owned domain before relying on the hostname for wider or long-term use. The remaining repository work is a dedicated push-to-deploy fixture scoped to the GitHub App installation.

Both dashboard `A` and wildcard application `A` records must resolve directly to `172.104.38.63`. TCP 80 and 443 must be allowed in the Linode Cloud Firewall and host firewall; SSH should remain restricted to the operator's source network where practical. The installer performs DNS/RAM/disk checks before changing packages.

## Pilot sequence and exit evidence

1. [x] Read-only host inventory and firewall diagnosis.
2. [x] DNS records and TCP 80/443 verification.
3. [x] Install the exact alpha source release and create the owner's account over valid HTTPS.
4. [ ] Install the owner's GitHub App and prove one signed push-to-deploy delivery.
5. [x] Deploy the selected app; prove its HTTP response and failed-deploy traffic preservation.
6. [x] Reboot; verify identity, routes, owner session and active application.
7. [x] Export a cold backup off-server, restore to a separate empty host, then deploy again.
8. [ ] Rehearse update/recovery and observe at least 48 hours including a build/update; record resource use, transfer, availability and the actual Linode charge separately from estimates. The elapsed window and sampled failure are recorded below, but final availability and actual charge evidence did not pass.

## 48-hour observation outcome — 2026-09-11

The measured window ran from `2026-09-09T03:30:04Z` through `2026-09-11T03:38:37Z`, totaling 48 hours 8 minutes. The guarded control-plane update at `2026-09-09T03:43:13Z` occurred inside this window. Raw HTTP and host snapshots are retained only in ignored `.data/linode-pilot` evidence; no credential or private host record is published.

- At the start, dashboard and application returned HTTP/2 200, the application header selected deployment `00ce3a973726fcfac2e49c17`, all seven Docker containers were running, root disk use was 8.9 GiB of 79 GiB (12%), and the host reported 700 MiB used with 3,214 MiB available RAM.
- At `2026-09-10T09:33:52Z`, 30 hours 3 minutes into the window, dashboard and application still returned HTTP/2 200 with the same deployment header and body. Root disk use was 9.3 GiB (13%), host memory was 785 MiB used with 3,130 MiB available, swap remained unused, and the seven container memory readings totaled about 166 MiB. Container network counters were captured as operational evidence, but they are not provider transfer or billing totals.
- At `2026-09-10T15:36Z`, 36 hours 6 minutes into the window, dashboard HTTPS, application HTTPS and SSH all timed out. The same result repeated at `2026-09-10T21:37Z` and at the final `2026-09-11T03:38Z` observation. DNS continued resolving the pilot hostname to `172.104.38.63`. These probes establish observer-side unavailability; without SSH or an authenticated Linode status view they do not distinguish a powered-off host from firewall or provider-network failure.
- The Linode billing page required a fresh account login during the final check, so no authenticated charge or invoice amount was available to record. No estimate is presented as an actual charge.

The elapsed-time requirement was measured, but the Phase 6.3 exit did not pass. Recover or verify the Linode, confirm the same deployment/data, capture authenticated billing evidence, then restart a clean observation window.

## Recorded workload and recovery evidence

- The 2026-09-09 guarded update created recovery record `.data/updates/20260909T034313Z-6f1ade5a`, rebuilt the server and agent, and passed their health checks. The existing application container was not replaced. Authenticated public API verification returned one project, two services, one deployment, an online node and a two-resource production canvas. Browser verification showed `PROJECT CANVAS` plus ready GitHub repository, Docker image, empty service, background worker, cron, PostgreSQL, Redis, MySQL, MongoDB, volume and S3-compatible bucket choices.

- Project `2433c1a853804a005d28db20`, service `037b9c4141bba7f16d897963` built the public `railwayapp-templates/expressjs` repository with Railpack and serves `{"body":"Hello world!"}` over HTTPS.
- Active deployment `00ce3a973726fcfac2e49c17` remained in the response header after an intentional build using a missing Dockerfile failed.
- After `systemctl reboot`, SSH returned with a new boot ID, the authenticated state retained the same project/service/deployment IDs, and both dashboard and application returned HTTP 200.
- `scripts/host-recovery.py backup` stopped the control services, captured state and resumed them. Its verifier passed on the source and again after SSH transfer to ignored off-server storage. Both copies had manifest SHA-256 `b077db3b82402bef058753af1440019eef77947208acf364a6372f1c0a1796c5`; dashboard and app returned HTTP 200 afterward.
- Temporary Linode `104604476` (`172.104.183.192`) used Ubuntu 24.04.4 amd64, 2 CPU, 3.8 GiB usable RAM, an encrypted 79 GiB root filesystem and an empty Docker 29.8/Compose 5.5.1 installation. Its boot ID differed from the source. The destination verifier matched the same manifest checksum before restore.
- The source was fenced before `restore --source-fenced --public-ip 172.104.183.192`. Restored owner/session, project/service/deployment IDs, app response/header and retained Let's Encrypt certificate passed using direct DNS overrides. A new source build became active as deployment `d41f0e2223a08071cf0617e3` on the restored host.
- The destination was fenced before the original host resumed. The original public dashboard/app returned HTTP 200 with deployment `00ce3a973726fcfac2e49c17`. The temporary Linode was then deleted; the source pilot remained running.

Until these checks pass, CI and local fixture certificates are not Linode/DNS/ACME evidence. The optional [AWS pilot](aws-pilot.md) remains documented for a later provider comparison.
