# Third-party components

Cloudrail's original code is Apache-2.0. It does not include Railway/Coolify source or branding assets. The following components remain under their respective upstream licenses; this summary does not replace their notices.

| Component | Source and license |
| --- | --- |
| Go | https://go.dev/LICENSE — BSD-style |
| pgx, pgpassfile, pgservicefile, puddle | https://github.com/jackc — upstream MIT licenses |
| golang.org/x packages | https://go.googlesource.com — BSD-style |
| React, Vite, TypeScript | Upstream repositories — MIT |
| Lucide icons | https://github.com/lucide-icons/lucide — ISC |
| Playwright | https://github.com/microsoft/playwright — Apache-2.0 (development only) |
| PostgreSQL | https://www.postgresql.org/about/licence/ — PostgreSQL License |
| Traefik | https://github.com/traefik/traefik — MIT |
| BuildKit and Distribution registry | https://github.com/moby/buildkit and https://github.com/distribution/distribution — Apache-2.0 |
| Railpack | https://github.com/railwayapp/railpack — MIT |
| Docker/Alpine images and included OS packages | Individual package licenses; inspect the actual image distribution |

The release packager includes the installed npm dependency LICENSE/NOTICE files alongside the compiled dashboard and Go module LICENSE/PATENTS files alongside binaries. No dependency source is vendored in the source archive. Container images are built or obtained separately and retain their upstream contents and notices. Verify notices and image inventories before publishing a release.
