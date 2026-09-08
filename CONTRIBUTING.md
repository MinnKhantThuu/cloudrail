# Contributing to Cloudrail

Cloudrail is an early alpha for one owner, one Linux node and trusted repositories. Read [the roadmap](docs/ROADMAP.md), [architecture](docs/architecture.md) and [user guide](docs/user-guide.md) before proposing a change.

## Bugs and proposals

Use [GitHub Issues](https://github.com/MinnKhantThuu/cloudrail/issues) for reproducible bugs and feature proposals. Include expected/actual behavior and the affected version. For substantial features, explain the user problem and how the proposal fits the single-node scope before implementing it. Security issues belong in [private reporting](SECURITY.md), not a public issue.

## Development setup

```sh
git clone https://github.com/MinnKhantThuu/cloudrail.git
cd cloudrail
npm ci --prefix apps/web
go test -race ./...
go vet ./...
npm run build --prefix apps/web
```

Use Go 1.26.8 and Node 22.20+ with the committed lockfile. The normal local runtime is `bash scripts/dev-up.sh`; after editing code, `bash scripts/dev-rebuild.sh` can rebuild using host Go/Node. A running Linux Docker runtime is required for deployment verification.

Database integration tests need `CLOUDRAIL_INTEGRATION=1`, an expendable PostgreSQL database URL and a 64-hex test encryption key. Each test creates a separate schema. See [verification](docs/verification-current.md) for database, real Docker and browser prerequisites. Do not run destructive/restart acceptance journeys on production.

## Code and review expectations

- Keep SQL migrations append-only; checksums reject edits to applied migrations.
- Preserve immutable deployment/source/variable snapshots, attempt-token checks and route recovery semantics.
- Add meaningful failure/recovery coverage when changing queues, persistence or deployment behavior.
- Inspect UI changes in a browser on desktop and mobile, including loading/error states.
- Keep secrets, `.env`, `.data`, keys, backups, generated dependencies and local account details out of patches.
- Test only owned projects/containers; never use a global Docker prune in development scripts.

For a pull request, describe the concrete problem, resulting behavior and verification. Separate local test evidence from remote deployment. Run `python3 scripts/verify-release.py` when changing package contents and keep linked documentation consistent.

Contributions are under Apache-2.0; dependencies retain their own licenses. Maintainers review proposals within the project's current scope; there is no support SLA.
