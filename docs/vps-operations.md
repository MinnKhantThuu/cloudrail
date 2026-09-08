# VPS installation and operations

The installer targets Ubuntu 24.04/26.04 on amd64/arm64. Use at least 4 GB RAM and 8 GB free Docker disk space (20 GB recommended). These are build/workload capacity requirements; the Go control-plane processes are smaller.

## Install

Prepare a dashboard A record (`console.example.com`) and wildcard application A record (`*.apps.example.com`) pointing directly to the VPS IPv4. Allow public inbound TCP 80/443 and restrict SSH to your own CIDR. Docker's published ports need provider firewall rules; don't rely solely on UFW.

From a reviewed Cloudrail source release on the VPS:

```sh
sudo bash scripts/install-vps.sh \
  --domain console.example.com --app-domain apps.example.com \
  --email owner@example.com --public-ip YOUR_PUBLIC_IPV4 --check-only

sudo bash scripts/install-vps.sh \
  --domain console.example.com --app-domain apps.example.com \
  --email owner@example.com --public-ip YOUR_PUBLIC_IPV4
```

The installer validates OS/DNS/resources, installs Docker from its official apt repository when absent, preserves installation secrets and data, and starts the public Compose overlay. It creates no AWS resources. Read the bootstrap token from `/opt/cloudrail/deploy/local/.env` and create the owner account over HTTPS. The bootstrap token cannot log into an already configured workspace.

The public overlay exposes only 80/443 publicly. API port 8080, local preview port 8088 and image registry 5001 remain on loopback. PostgreSQL and the agent TLS listener are private. BuildKit uses a shared UNIX socket. Traefik issues/renews certificates and stores ACME state in the `cloudrail_certificates` volume.

If HTTPS isn't ready, inspect `scripts/compose.sh logs --tail=100 proxy`, check DNS/port 80 reachability and certificate errors, and retry after correcting the cause. Do not disable secure cookies to work around public HTTPS problems. A green deployment status proves internal readiness; public DNS/certificate issuance must also be verified.

## Application settings

Resource limits are snapshotted into each deployment. Save settings and redeploy to apply them; restart retains that deployment's configuration. Once a persistent volume is attached, its path/identity cannot be moved or detached through the UI. Containers can be removed without deleting the named volume.

A shared volume uses stop-before-start replacement. A failed candidate is removed before restarting the old container. This prevents concurrent writers. Application/schema changes to data are not reverted by image rollback.

Private networks belong to a project/environment. PostgreSQL has no published port and is attached only to that network. **Connect application** writes `DATABASE_URL` into an application in the same environment without revealing the password. Redeploy that application to apply the binding. PostgreSQL initialization credentials are managed and immutable through variable editing. The template is pinned to PostgreSQL 17.6; major upgrades require a separately tested migration.

## Back up and restore

Use **Settings → Create backup** for PostgreSQL dumps. Download the result and keep an encrypted copy off the VPS. Restoring requires an empty target database and uses a single transaction with exit-on-error, without dropping existing objects. Create a fresh PostgreSQL service when restoring production data.

For application volumes, stop the service before creating/restoring a backup. Volume exports are limited to 1 GB. The restore target must be empty and stopped; links, special files and unsafe paths are rejected. A failed file extraction can leave a partial target, so inspect it or create another empty target rather than retrying over existing files. The source volume is preserved.

Control-plane backup and independent restore rehearsal:

```sh
scripts/backup-control-plane.sh
scripts/verify-control-restore.sh .data/backups/control-TIMESTAMP
```

Control-plane backups include the database, encryption key, installation credentials, CA and node identity. Treat the entire directory as secret. It does **not** include application volumes, the registry, application backup volume or ACME volume. Back those up separately. Restoring only the database without its matching encryption key cannot recover encrypted variables or App credentials.

For a lost/revoked node identity, an operator with SSH access can run `scripts/node-reenroll.sh --replace-node`. It backs up the control plane, stops the agent, rotates the enrollment token and replaces the certificate identity. Existing application containers keep running. Verify the new heartbeat before resuming changes.

## Disk and retention

New image pulls reserve 1 GB free disk, new containers reserve 512 MB, and builds reserve 2 GB. Low space is shown in node metrics. BuildKit has bounded cache policy; runtime logs rotate at 5 MB × 2 files, failed/superseded containers are cleaned, and stored log tails are bounded. Persistent volumes, registry images and backups are never broadly deleted automatically.

To clear only Cloudrail's dedicated build cache:

```sh
docker exec cloudrail-agent-1 buildctl --addr unix:///buildkit/buildkitd.sock prune --all --keep-storage 256 --free-storage 2048
```

Do not use a global Docker prune on a host with unrelated applications. Remove retained backups/images only after checking references and keeping a verified off-server copy. Historical image redeploy depends on retaining its registry manifest/layers.

## Evidence boundary

Local Docker tests verified PostgreSQL persistence/private networking, hidden bindings, checksum-verified backup download, empty-target restore, nonempty-target rejection, HTTP volume persistence/restore, resource limits, observed metrics and custom routing. A separate local proxy verified HTTPS with an explicitly trusted test certificate. This is not a DNS-issued ACME certificate or a clean public VPS installation; those checks require the target host/domain.

References: [Docker Ubuntu installation](https://docs.docker.com/engine/install/ubuntu/), [Docker firewall behavior](https://docs.docker.com/engine/network/packet-filtering-firewalls/).
