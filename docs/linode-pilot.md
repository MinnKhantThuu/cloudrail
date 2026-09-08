# Linode public pilot

Status, 2026-09-08: preparation and public control-plane install complete on target `172.104.38.63`. The 4 GB Singapore Linode runs Ubuntu 24.04.4 x86-64 with 2 CPU and a 79 GiB root disk. A dedicated root SSH identity was authorized and verified against the console-published host fingerprint. Cloud Firewall allows TCP 22/80/443 with default inbound drop. Docker 29.8.0/API 1.56 and Compose 5.5.1 are installed.

Exact alpha.4 commit `302aec491fc33fd8a0ef5216b33856d899e5cb5f` passed installer preflight and is live at `https://console.172-104-38-63.sslip.io`. The dashboard returned HTTP/2 200 with a trusted Let's Encrypt YR1 certificate valid for that hostname. All six platform containers started, with PostgreSQL, server and agent healthy. The first idle snapshot totaled about 82 MiB across the six containers; Docker daemon, OS cache and build peaks are excluded. Owner setup, GitHub App delivery, pilot workload, reboot/recovery and the 48-hour observation remain pending.

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

`sslip.io` provides temporary IP-based public DNS for this pilot. Move to a user-owned domain before relying on the hostname for wider or long-term use. The remaining external input is the pilot GitHub repository and its expected user flow.

Both dashboard `A` and wildcard application `A` records must resolve directly to `172.104.38.63`. TCP 80 and 443 must be allowed in the Linode Cloud Firewall and host firewall; SSH should remain restricted to the operator's source network where practical. The installer performs DNS/RAM/disk checks before changing packages.

## Pilot sequence and exit evidence

1. Read-only host inventory and firewall diagnosis.
2. DNS records and TCP 80/443 verification.
3. Install the exact alpha source release and create the owner's account over valid HTTPS.
4. Install the owner's GitHub App and prove one signed push-to-deploy delivery.
5. Deploy the selected app; prove its important flow and failed-deploy traffic preservation.
6. Reboot; verify identity, routes, application and database/volume data.
7. Export a cold backup off-server, restore to a separate empty host, then deploy again.
8. Rehearse update/recovery and observe at least 48 hours including a build/update; record resource use, transfer, availability and the actual Linode charge separately from estimates.

Until these checks pass, CI and local fixture certificates are not Linode/DNS/ACME evidence. The optional [AWS pilot](aws-pilot.md) remains documented for a later provider comparison.
