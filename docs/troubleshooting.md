# Troubleshooting

Start with the selected service's deployment/build error and bounded logs. For platform problems, run these commands from the installation directory:

```sh
bash scripts/compose.sh ps
bash scripts/compose.sh logs --tail=100 server agent proxy
```

Remove credentials and private application output before posting a public issue. Use [private vulnerability reporting](../SECURITY.md) for security problems.

## Setup or login fails

- A fresh installation shows **Create your account**: enter email, password and confirmation. No token is required. Once configured, the page shows **Sign in**; use the existing owner email/password.
- **Passwords do not match** means the two setup password fields differ. **Show** makes both visible so you can correct them.
- Passwords must be 12–72 bytes. There is no public default password or email-reset flow in this alpha.
- If local acceptance scripts created the owner, that installation's `.data/test-owner.json` holds the test credentials. It is not part of the repository.
- Too many failed attempts cause a 15-minute rate limit. Do not repeatedly submit credentials while limited.
- Public installations require HTTPS for the session cookie. Check the hostname/certificate; do not disable secure cookies to work around public routing errors.
- Keep a verified control backup before any operator account-recovery work. Deleting the database to fix login also deletes deployment records and is not a recovery procedure.

## Docker or startup fails

Check `docker info` and `docker version`. Start the Linux runtime first on macOS. The Docker Engine API must support 1.47+, and Compose must be installed. Check RAM/free disk on the Docker VM, not just the host computer. The initial image build needs network access to upstream image/package registries.

Ports 8080, 8088 and 5001 must be available locally. A public VPS also needs 80/443. Inspect the conflicting service before changing ports or stopping it. Cloudrail assumes one installation per Docker host.

## Node offline

The agent needs its persistent identity, the server's matching CA/database identity and access to the private management network. Check server/agent logs and confirm the control API is healthy. A recent API/agent restart can briefly show offline until the heartbeat resumes.

Do not delete identity volumes to make the indicator green. If the node was revoked or its identity was lost, an operator can use `scripts/node-reenroll.sh --replace-node` after reviewing [the operations runbook](vps-operations.md). A revoked agent cannot manage applications even if existing containers are still serving traffic.

## Image deployment fails

- Mutable tags such as `nginx:latest` are rejected. Supply a repository and full SHA-256 digest.
- Verify that the image supports the node architecture and can be pulled by the Docker host. Arbitrary private external registry credentials are not configurable in this alpha.
- Check container port and `0.0.0.0` binding. The host port is not the same as the internal application port.
- Readiness must return 2xx without redirect/login at the configured path. The bundled quickstart uses a short readiness timeout; slow-starting applications need an appropriate `READINESS_TIMEOUT` agent setting.
- Review variables and application logs. Saving variables/settings requires a new deployment; restart retains the old snapshot.

If a previous release remains active, investigate before retrying. Don't delete it to clear a failed badge.

## Source build fails

Check the exact repository, branch, root and Dockerfile path saved under **Source**. Dockerfile paths are relative to the selected source root. Railpack support depends on the actual application; the recorded matrix currently covers a public Node/Express sample and a separate Go Dockerfile sample.

Private repositories need an installed GitHub App with Contents/Metadata read access. Private package dependencies need build-time credentials, which are not supported yet. Runtime variables are deliberately excluded from builds.

Inspect BuildKit/registry logs if source retrieval succeeded but building/pushing failed. Builds use the dedicated UNIX socket and loopback registry, not a public BuildKit TCP endpoint. Keep runtime and build resource limits separate. See [source limits](source-builds.md#execution-and-recovery).

## Push does not deploy

Check the App installation/repository access, configured branch, automatic-deploy setting, webhook URL and matching secret. In GitHub's App settings, inspect the delivery result. Webhooks must reach the **public HTTPS dashboard**, not a localhost URL. Nonmatching branches/deleted pushes do not trigger deployment, and repeated delivery IDs are deduplicated.

A manual public-repository build does not prove that the GitHub App webhook has been installed/configured.

## Application URL or HTTPS fails

Local apps use `<service-id>.localhost:8088`. A client without wildcard localhost support can send that hostname as the HTTP `Host` header to `127.0.0.1:8088`.

For VPS domains, check A records and wildcard app DNS against the server IPv4, public 80/443 reachability, the public Compose overlay and proxy certificate logs. A healthy deployment's private readiness check is separate from public DNS/ACME verification. Keep ACME state persistent and correct certificate errors before entering owner credentials over a public connection.

## Database connection fails

The database and HTTP application must be in the same project/environment. Connect the application from the database's Settings, then redeploy the application. Its private hostname will not resolve from your laptop. The database has no public port. Managed template credentials cannot be changed through ordinary variable editing.

For persistent data, verify the existing named volume instead of creating another service and assuming data was copied. New environments/services are independent.

## Backup or restore fails

Download/check the recorded checksum and inspect the action error. PostgreSQL restoration requires an empty database. Volume restoration requires an empty, stopped target; volume backups require a stopped source and are limited to 1 GB. The UI restores backups retained in the installation; it does not import arbitrary external archives.

Do not bypass empty-target checks. Create a fresh target and retain the original data. Partial volume extraction can leave files behind; inspect that target before deciding whether to use another empty one. A backup stored only on the same VPS does not protect against loss of the VPS.

## Disk is low

New builds reserve 2 GB free disk, new image pulls 1 GB, and container creation 512 MB. The Docker VM can run out of disk while the host still has free space. Existing apps may continue serving while new work is rejected.

Follow [disk/retention guidance](vps-operations.md#disk-and-retention). Only clear Cloudrail's dedicated build cache if appropriate; don't run a global Docker prune on a shared host. Keep referenced registry images for rollback and verified off-server copies of backups. Persistent data is not automatically deleted to free space.

## Reporting a bug

Include your Cloudrail version, host OS/architecture, Docker/Compose versions, reproduction steps, expected/actual behavior and relevant **redacted** errors. State whether the problem occurs locally or on a public VPS. Do not attach `.env`, private keys, owner credentials, backups or unreviewed logs.
