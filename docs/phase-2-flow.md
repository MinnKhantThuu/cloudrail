# Phase 2 — workspace flow and implementation contract

2026-09-08. The user authorized consecutive implementation of the complete roadmap. This phase is implemented before Phase 3; user visual acceptance remains distinguishable from browser verification.

## Screen map

1. **First visit:** owner email/password/confirmation → workspace, signed in immediately. Setup is available once; concurrent registrations cannot create a second owner. No setup token is required. Later visits use email/password login.
2. **Returning owner:** email/password → HttpOnly session → workspace; sign out invalidates the server-side session.
3. **Workspace:** project list → project overview → environment selector (production/staging/custom). Creating a project creates production. Environment selection filters services and never copies secrets automatically.
4. **Service:** deployments / runtime logs / variables / settings. Variables display names only, with replace/delete actions. Changed variables apply to the next deployment. Each deployment retains its own encrypted configuration snapshot.
5. **Deploy:** pinned image + container port + readiness path → persisted activity → application URL. Historical redeploy explicitly uses current service variables (the form explains this).
6. **Service actions:** stop removes its route and stops the current container; start/restart brings the same release back and checks readiness before publishing the route. Actions are durable, serialized against deploys and visible in settings. Stop preserves data/configuration/history.
7. **Server/settings:** owner session details and local node scope; later phases add node identity and operating metrics. No pretend metric values.

## State behavior

- Empty projects/services prompt the next concrete action. Forms keep their input after API errors.
- Loading and disconnected states never imply a deployment is healthy.
- Mobile supports project/environment navigation, all service tabs and sign-out, without horizontal document overflow.
- Account/setup/session requests are rate-limited. Browser writes require a same-origin custom header; session cookies are HttpOnly, SameSite Strict and Secure when HTTPS is configured.
- Secrets use AES-256-GCM with random nonces and service/deployment-bound associated data. The encryption key is stored outside PostgreSQL; backups require both database and key.

## Acceptance

Upgrade existing M1 data; reject a second owner setup; authenticate/sign out with a real cookie session; reject cross-origin/unauthenticated writes; isolate services with identical names in staging and production; prove encrypted values are absent from database plaintext and normal state responses; deploy with variables; stop/start/restart without changing another environment; exercise these operations through the real dashboard.
