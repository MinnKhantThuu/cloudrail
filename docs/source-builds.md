# GitHub source deployments

Open a service's **Source** tab. Enter an accessible `owner/repository`, choose its branch and root directory, then choose Dockerfile or Railpack. Save the source and select **Build & deploy**. Build history records the commit, bounded logs, image digest and resulting deployment. A successful image build is separate from a healthy deployment.

Runtime variables are applied when the image is queued for deployment. This version does not inject runtime variables or installation tokens into builds. For public build configuration, use a Dockerfile, checked-in Railpack configuration or the build/start command overrides. Private package build secrets are not yet supported.

## GitHub App

Create an App in the intended account/organization with Contents (read), Metadata (read), and push events. Set its webhook to `https://<dashboard-domain>/webhooks/github` and use a random secret of at least 32 characters. In Cloudrail's **GitHub** dialog, save the App ID, slug, PEM private key and webhook secret; then follow **Install app on GitHub**. Choose the repositories that installation should expose. Back in the service, load installations, choose the installation, select/enter the repository and branch, and enable push deployments.

The setup does not publish code or send messages. Account installation must be performed in the intended GitHub account. Repository and branch suggestions show the first page (100 entries); any accessible repository/branch can also be entered directly. The API supports `page` for paginated clients.

## Execution and recovery

The same trusted execution node handles Dockerfile and Railpack builds sequentially. BuildKit listens on a private UNIX socket. The registry listens only on the Docker host's loopback, at port 5001. It must never be published publicly without authentication/TLS. BuildKit is privileged, which is why this release is for an owner and trusted team, not arbitrary customer code.

A build attempt is capped at 15 minutes; interrupted execution can resume up to three claims within a 45-minute job deadline. Cancel stops the build command and retains the previous serving deployment. Request IDs and signed webhook delivery IDs prevent duplicate jobs. Source paths are bounded and links/special files are rejected. Build scratch is removed after execution and stale scratch is removed on agent startup.

The image reference is `127.0.0.1:5001/cloudrail/<service-id>@sha256:<manifest>`. Keep the registry volume in VPS backups. Runtime deployment limits and build resource limits are separate.

## Local verification

- `python3 scripts/phase4_acceptance.py` — public GitHub Dockerfile and Railpack samples, registry publication, HTTP routing and failed-build preservation.
- `npm --prefix apps/web run test:browser -- source.spec.ts` — source/history, hidden App credentials, desktop/mobile UI. Run acceptance first to create the sample project.
- `TestPostgresSignedPush` — isolated schema, HMAC rejection, replay deduplication, stale/cancelled reports and bounded retries. The sample signature is the public [GitHub validation example](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries).

GitHub App account installation and a GitHub-delivered webhook require a real installation/domain. Local signed-payload tests do not prove those account-bound steps.
