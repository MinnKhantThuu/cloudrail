# Linode public pilot

Status, 2026-09-08: target `172.104.38.63` supplied by the owner. TCP 22 accepts connections. TCP 80/443 and direct HTTP/HTTPS timed out. Reverse DNS is absent. A read-only `root` SSH attempt using the local default identity was rejected with `Permission denied (publickey)`. No package, firewall, DNS or server state was changed.

## Access handoff

Add the project-specific public key supplied by the operator to `/root/.ssh/authorized_keys` using Linode Cloud Manager/Lish or another already-authorized session. Do not send the root password, SSH private key, API token or recovery data through chat. The private half stays in ignored local `.data` storage with mode `0600`.

After key authorization, the first connection only records:

- `/etc/os-release`, architecture and kernel;
- CPU count, RAM and free root/Docker disk;
- Docker/Compose availability;
- listening ports and active host firewall policy.

The supported installer target is Ubuntu 24.04 or 26.04, amd64/arm64, at least 4 GB RAM and at least 8 GB free Docker disk (20 GB recommended). If this host is smaller or uses another OS, resize/rebuild it before installation.

## Inputs before installation

- Dashboard hostname, for example `cloudrail.example.com`.
- Wildcard application domain, for example `*.apps.example.com`.
- ACME contact email.
- Pilot GitHub repository and its expected user flow.

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
