# 0.1.0-alpha.1 release and recovery

This is the first alpha source publication; it is not a production-ready beta. It includes Phase 1–5 implementation, VPS/AWS preparation and release tooling. See [verification](verification-current.md) for what was actually exercised.

## Packages

```sh
# Source only; no host compiler or installed frontend dependencies needed.
python3 scripts/package-release.py

# Go 1.26.8, Node 22.20+ and npm required; produces source and both Linux architectures.
bash scripts/build-release.sh
```

Output: `.data/releases/cloudrail-0.1.0-alpha.1-{source,linux-amd64,linux-arm64}.tar.gz` and `SHA256SUMS`. Source archives include a per-file `SOURCE-MANIFEST.json` and exclude installation data, keys, environment files and dependencies. Runtime archives contain API/agent binaries, dashboard assets and dependency notices. They are developer artifacts, not a replacement for the Compose services, BuildKit tools or installer. Use the **source archive** for a standard VPS install.

Archive metadata is normalized for repeatable packaging. Go builds use trimpath and no VCS/build ID. Identical staged inputs produce identical archives. Docker base tags and OS package repositories are not frozen snapshots, so bit-identical Docker rebuilds are not promised. Verify `SHA256SUMS` before extraction; no signing identity has been selected yet.

GitHub Actions definitions run Go race/vet/database checks, npm build/audit, local packaging and a separate Docker/browser journey. See the repository [Actions page](https://github.com/MinnKhantThuu/cloudrail/actions/workflows/verify.yml) for hosted results; local verification is recorded separately. Artifacts are uploaded to CI only when that repository workflow actually runs; there is no release publishing step.

## Update an existing installation

Updates are an operator maintenance action. First export application backups off-server, verify a restore, read schema/image changes and retain the previous source release. Do not change the control PostgreSQL major version with this procedure.

Copy only a reviewed source release over the installation, preserving `deploy/local/.env`, `.data` and Docker volumes. Then run from that installation:

```sh
bash scripts/update.sh
```

The script tags the current API/agent images for recovery, builds replacements while the old containers serve, stops job writers, backs up control database/identity, then starts the new API/agent. The public-overlay marker is respected. Application containers keep serving during control-plane maintenance, except an already-running deployment may need agent reconciliation. Run this when no deployment/build/action is pending.

Recovery material is in `.data/updates/TIMESTAMP/`. The control backup contains secrets; encrypt and export it. After update verify API health, owner sign-in, node online state, existing routes, a new deployment and backup download. This script has been syntax-reviewed; a cross-version clean-host upgrade remains a release gate because no earlier public version exists.

## Recover a failed update

Do not automatically restore a production database or start an old binary against a changed schema. Applied migration checksums are immutable. An image rollback cannot reverse schema/data changes.

If the new server **did not apply any migration** (compare `schema_migrations` with the saved dump), the retained `previous-images.yaml` selects the prior API/agent images:

```sh
bash scripts/compose.sh -f .data/updates/TIMESTAMP/previous-images.yaml \
  up -d --no-build --wait server agent
```

When schema changed, keep the failed installation and volumes intact. Restore the control dump with the **previous source/images and matching installation.env/encryption key/CA/node identity** into a separate recovery installation; do not overwrite the working database. Before starting its agent, restore the required application volumes and registry, prevent both agents from controlling the same Docker host, and reconcile the app state captured at the backup timestamp. ACME state and off-server application backups need their own copies. The original registry/volumes are preserved until recovery is verified. Full host disaster recovery needs the independent-host rehearsal below; a database-only rehearsal is not a complete host restore.

To verify dump integrity without changing the running workspace:

```sh
bash scripts/verify-control-restore.sh .data/updates/TIMESTAMP/control-backup
```

## Known limitations and release gates

- One owner/node, trusted builds; no hostile multi-tenancy, RBAC, scheduling or HA.
- Tested source matrix: public Go Dockerfile (`traefik/whoami`) and Node/Express Railpack template. Other languages/private dependencies are not yet verified.
- Real GitHub App install/webhook delivery, public DNS/ACME and AWS application pilot are pending account/domain access.
- Public installer declares Ubuntu 24.04/26.04 amd64/arm64 targets; neither a clean public host nor both architectures have received full installer/runtime proof. Local Linux arm64 runtime and a fresh Ubuntu CI amd64 Compose/browser journey have passed; public installer and cross-version host recovery are still unverified.
- State is retained deliberately: registry/backups/history need operator retention. No scheduled off-server backup or automatic database major upgrade.
- A node certificate eventually needs operator re-enrollment; keep identity recovery material. Backup archives include private application data.
- Final owner UX walkthrough, independent fresh install → update → recover, dependency/image security review, signed release assets remain beta gates. Source hosting and private vulnerability reporting use [the GitHub repository](https://github.com/MinnKhantThuu/cloudrail).

Source repository: [MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail). This source publication does not deploy an AWS resource or create a versioned GitHub Release. Local alpha archives can be built with the commands above.
