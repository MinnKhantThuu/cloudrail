# Linode public pilot

Status, 2026-09-09: preparation, public control-plane install and Railway-inspired canvas update are complete on target `172.104.38.63`. The 4 GB Singapore Linode runs Ubuntu 24.04.4 x86-64 with 2 CPU and a 79 GiB root disk. A dedicated root SSH identity was authorized and verified against the console-published host fingerprint. Cloud Firewall allows TCP 22/80/443 with default inbound drop. Docker 29.8.0/API 1.56 and Compose 5.5.1 are installed.

Exact alpha.4 commit `302aec491fc33fd8a0ef5216b33856d899e5cb5f` passed the original installer preflight. The guarded updater then moved the live checkout to verified main commit `c15158aaef06adc3eb1e76b3ed035991b3c5825a`, retaining the owner, session, database and active workload. The dashboard at `https://console.172-104-38-63.sslip.io` returned HTTP/2 200 with a trusted Let's Encrypt YR1 certificate and exposed the project canvas/resource palette in an authenticated browser. A public Railpack sample remains active at `https://037b9c4141bba7f16d897963.apps.172-104-38-63.sslip.io`; its response still carries deployment `00ce3a973726fcfac2e49c17`. A verified 503 MiB cold backup restored successfully on a separate empty Linode and accepted a new deployment. User UX acceptance, a new versioned release, GitHub App delivery, certificate renewal and the 48-hour observation remain pending.

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
8. [ ] Rehearse update/recovery and observe at least 48 hours including a build/update; record resource use, transfer, availability and the actual Linode charge separately from estimates.

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
