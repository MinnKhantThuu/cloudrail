# Recover an installation onto a new Docker host

Available from alpha.3. A cold backup and recovery between two separate Ubuntu CI runners passed; the alpha.2 archive does not contain these commands. Use the source matching the installed runtime's VERSION and deployment configuration. This is a cold backup of Cloudrail's persistent application/platform state, not an operating-system disk image.

## What is retained

- Control PostgreSQL data, owner account/sessions, encrypted configuration, deployment history and node records.
- Application PostgreSQL data and attached named volumes, including stopped applications.
- Internal registry data and application backup downloads.
- Platform CA, node certificate/key, installation environment/encryption key and optional ACME certificate storage.
- Exact platform Docker images and the public-installation marker.

BuildKit cache/socket and generated proxy routes are recreated. Application container writable layers, host files outside managed volumes, OS configuration and external databases/storage are not included. Persist required application files in an attached volume. External volume drivers and unsafe/nonportable archive links are rejected. Only restore private backups from trusted storage; checksums detect damage but are not an independent authenticity signature.

## Cold backup on the existing installation

Allow a maintenance window: the command stops Cloudrail's API/agent, applications and platform writers while snapshotting their volumes, then resumes the original running containers. No application/build/action may be pending. It uses the same operator lock as update/rollback and refuses unrelated containers sharing these volumes.

```sh
python3 scripts/host-recovery.py backup .data/backups/host-20260908
python3 scripts/host-recovery.py verify .data/backups/host-20260908
```

Export the entire directory to encrypted off-server storage. It contains passwords, private keys and application data. A partial backup is marked incomplete and cannot be restored. Normal exceptions/signals attempt to resume the source platform. After machine/process loss, inspect its state and start the platform with `bash scripts/compose.sh up -d --no-build --wait`; verify application recovery before further changes. Keep the original volumes and backup until the restored installation is accepted.

## Restore on a new host

The destination must have a Linux Docker daemon of the same CPU architecture, Docker Compose and Python 3, sufficient free Docker disk, the matching Cloudrail source, and no containers, volumes or installed `deploy/local/.env`. Restore refuses the still-running source daemon/kernel and existing installations. Docker IDs can be cloned in VM images, so Linux kernel boot identity and empty-storage checks disambiguate the destination; run the commands on the Docker host itself. The exact PostgreSQL image is restored; this procedure does not perform database-major or CPU-architecture migrations.

Fence the old host first: power it down or otherwise prevent both its agent and application writers from running. Keep it fenced through cutover. Copy the private backup to the new host, outside the destination source directory if convenient:

```sh
python3 scripts/host-recovery.py restore /secure-backups/host-20260908 --source-fenced
```

For a public installation, prepare dashboard/app DNS and ports 80/443 for the new destination, then also pass `--public-ip NEW_PUBLIC_IPV4`. Domains and saved certificate state are retained. Actual DNS/ACME issuance and renewal still need verification on that public host.

Restore verifies the inventory, checksums, tar paths, architecture and source/configuration match before loading anything. It imports platform images and volumes, restores the private environment, starts the platform, fetches active application image digests, then starts the restored agent. Internal source-build images come from the retained loopback registry. Public/upstream images require registry access; perform an operator `docker login` first if needed. This is not an offline application-image mirror.

A failed destination is retained for inspection, with `.data/host-restore.json` recording its stage. Do not point the command at existing production data or manually bypass its empty-host guard. Use a new empty destination for a retry after correcting the cause.

## Acceptance before cutover

1. Sign in with the original owner; verify projects/environments and node heartbeat.
2. Open application routes and validate meaningful PostgreSQL rows and persistent files.
3. Verify private database bindings, application backup downloads and stopped-service state.
4. Deploy a new image/source revision and verify its route and data.
5. Verify public HTTPS and DNS separately; then perform the planned traffic cutover while keeping the original host fenced.

The CI rehearsal uses two separate GitHub-hosted Ubuntu runners. The source job creates the fixture workloads, takes the cold backup and fences its workloads before the destination job starts. A dedicated repository CI secret encrypts/authenticates the synthetic fixture transfer with standard-library AES-256-GCM and random nonces; only ciphertext is uploaded, and the destination removes that transfer artifact afterward (one-day retention is the fallback). The key and plaintext private fixtures are never uploaded. The helper refuses transfers above its bounded test-fixture size; it is CI tooling, not a production backup-encryption product.

The destination has its own VM/kernel, Docker daemon and storage, and checks owner/session, identity, PostgreSQL rows, application volumes, registry artifacts, encrypted bindings, backup downloads, certificate-volume bytes, stopped state and a new deployment. This still does not substitute for the public HTTPS installer, real ACME or the AWS pilot. See [verification](verification-current.md) for recorded results.

Recorded independent-host proof: [CI run 34211555644](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34211555644) at `85bbdb3`. All five jobs passed, including the source-side envelope byte comparison, destination encrypted-file checksum/authentication and post-restore application/data checks. The earlier isolated envelope authentication failure has no confirmed root cause; integrity checks remain mandatory.
