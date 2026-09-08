# 0.1.0-alpha.2 release and recovery

This alpha adds simpler owner registration and guarded maintenance; it is not a production-ready beta. It includes Phase 1–5 implementation, VPS/AWS preparation and release tooling. See [verification](verification-current.md) for what was actually exercised.

## Packages

```sh
# Source only; no host compiler or installed frontend dependencies needed.
python3 scripts/package-release.py

# Go 1.26.8, Node 22.20+ and npm required; produces source and both Linux architectures.
bash scripts/build-release.sh
```

Output: `.data/releases/cloudrail-0.1.0-alpha.2-{source,linux-amd64,linux-arm64}.tar.gz` and `SHA256SUMS`. Source archives include a per-file `SOURCE-MANIFEST.json` and exclude installation data, keys, environment files and dependencies. Runtime archives contain API/agent binaries, dashboard assets and dependency notices. They are developer artifacts, not a replacement for the Compose services, BuildKit tools or installer. Use the **source archive** for a standard VPS install.

Archive metadata is normalized for repeatable packaging. Go builds use trimpath and no VCS/build ID. Identical staged inputs produce identical archives. Docker base tags and OS package repositories are not frozen snapshots, so bit-identical Docker rebuilds are not promised. Verify `SHA256SUMS` before extraction. Trusted push builds also generate GitHub artifact attestations in the same CI job that builds the packages. Verify the expected repository, workflow and release commit:

```sh
sha256sum -c SHA256SUMS
# Substitute the release commit recorded in the release notes.
gh attestation verify cloudrail-0.1.0-alpha.2-source.tar.gz \
  --repo MinnKhantThuu/cloudrail \
  --signer-workflow MinnKhantThuu/cloudrail/.github/workflows/verify.yml \
  --source-digest RELEASE_COMMIT
```

Use the same command for each runtime archive. Provenance identifies the CI workflow and source commit; it does not prove production readiness. See [GitHub artifact attestation documentation](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations).

GitHub Actions definitions run Go race/vet/database checks, npm build/audit, local packaging and a separate Docker/browser journey. See the repository [Actions page](https://github.com/MinnKhantThuu/cloudrail/actions/workflows/verify.yml) for hosted results; local verification is recorded separately. [v0.1.0-alpha.2](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.2) publishes the exact packages from [verified CI run 34205416387](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34205416387) at commit `b4c5bd2`. All three jobs passed; archive checksums, expected workflow/source-commit attestations and source-manifest correspondence were independently verified before publication. CI does not automatically publish a release on every push.

## Update an existing installation

Updates are an operator maintenance action. First export application backups off-server, verify a restore, read schema/image changes and retain the previous source release. Do not change the control PostgreSQL major version with this procedure.

Copy only a reviewed source release over the installation, preserving `deploy/local/.env`, `.data` and Docker volumes. Then run from that installation:

```sh
bash scripts/update.sh
```

The script rejects pending deployments/builds/actions and concurrent maintenance in the installation directory. It tags the current API/agent images for recovery, builds replacements while the old containers serve, stops both job writers, checks the queue again, backs up control database/identity, then starts the new API/agent. The public-overlay marker is respected. Application containers keep serving during control-plane maintenance. If a check or backup fails before starting the new runtime, the previous API/agent images are restarted. Once the new runtime might have applied migrations, no automatic downgrade is attempted.

Recovery material is in `.data/updates/TIMESTAMP-ID/`: a private `record.json` with exact image identities, installation identity, migration ledger and schema fingerprint, plus the control backup. The control backup contains secrets; encrypt and export it. After update verify API health, owner sign-in, node online state, existing routes, a new deployment and backup download. A hosted cross-version rehearsal installs the previously published alpha, updates to this version, restores a backup, rejects incompatible rollback, restores exact prior images and deploys again while monitoring app HTTP traffic. [CI run 34205416387](https://github.com/MinnKhantThuu/cloudrail/actions/runs/34205416387) passed this rehearsal, including preserved encrypted variables and new deployments after both update and rollback. Public-host upgrade remains a separate gate.

## Recover a failed update

Do not automatically restore a production database or start an old binary against a changed schema. Applied migration checksums are immutable. An image rollback cannot reverse schema/data changes.

For image-only rollback, run:

```sh
bash scripts/rollback.sh .data/updates/TIMESTAMP-ID
```

The rollback command verifies the completed-backup record, same Docker host and installation environment, unchanged migration ledger and actual schema, and the exact retained image identities. It rejects active jobs and rechecks after freezing the API/agent. A changed environment (including credentials) also blocks rollback; follow the recovery runbook instead of editing the guard record. Older installation tokens are retained only to allow the old alpha server to start during rollback; current registration never uses them. Run maintenance from the original installation directory and do not remove its recovery image tags before the update is accepted.

When schema changed, keep the failed installation and volumes intact. Restore the control dump with the **previous source/images and matching installation.env/encryption key/CA/node identity** into a separate recovery installation; do not overwrite the working database. Before starting its agent, restore the required application volumes and registry, prevent both agents from controlling the same Docker host, and reconcile the app state captured at the backup timestamp. ACME state and off-server application backups need their own copies. The original registry/volumes are preserved until recovery is verified. Full host disaster recovery needs the independent-host rehearsal below; a database-only rehearsal is not a complete host restore.

To verify dump integrity without changing the running workspace:

```sh
bash scripts/verify-control-restore.sh .data/updates/TIMESTAMP-ID/control-backup
```

## Known limitations and release gates

- One owner/node, trusted builds; no hostile multi-tenancy, RBAC, scheduling or HA.
- Tested source matrix: public Go Dockerfile (`traefik/whoami`) and Node/Express Railpack template. Other languages/private dependencies are not yet verified.
- Real GitHub App install/webhook delivery, public DNS/ACME and AWS application pilot are pending account/domain access.
- Public installer declares Ubuntu 24.04/26.04 amd64/arm64 targets; neither a clean public host nor both architectures have received full installer/runtime proof. Local Linux arm64 runtime and a fresh Ubuntu CI amd64 Compose/browser journey have passed; public installer and complete host disaster recovery are still unverified.
- State is retained deliberately: registry/backups/history need operator retention. No scheduled off-server backup or automatic database major upgrade.
- A node certificate eventually needs operator re-enrollment; keep identity recovery material. Backup archives include private application data.
- Final owner UX walkthrough, public installation, complete host recovery and deployment-image security review remain beta gates. CI-signed alpha packages and an independent Linux Compose update/rollback rehearsal are available. Source hosting and private vulnerability reporting use [the GitHub repository](https://github.com/MinnKhantThuu/cloudrail).

Source repository: [MinnKhantThuu/cloudrail](https://github.com/MinnKhantThuu/cloudrail). [Versioned alpha release](https://github.com/MinnKhantThuu/cloudrail/releases/tag/v0.1.0-alpha.2) is public. Publishing it created no AWS resources. The commands above also build local development archives.
