# Rover 0.9 progress log

Work is on `feature/v0.9-cloud-analytics` only. No merge to main, no tag, no GitHub release.

Branch: https://github.com/aamitojj/rover/tree/feature/v0.9-cloud-analytics

## Push log

Commits from this implementation (newest last). Each URL is `https://github.com/aamitojj/rover/commit/<SHA>`.

| SHA | URL | What was pushed |
| --- | --- | --- |
| `cac427d58868d4d79ae58fc2fab95c164fefca53` | https://github.com/aamitojj/rover/commit/cac427d58868d4d79ae58fc2fab95c164fefca53 | `prov` types, HTTP transport, registry, SigV4 / OCI / Google JWT signing |
| `157e3da1d5ce0cf4403757e550024c7a3987cd52` | https://github.com/aamitojj/rover/commit/157e3da1d5ce0cf4403757e550024c7a3987cd52 | Coolify, nginx, Netlify, Vercel, Supabase and Firebase providers |
| `0ae90f0567e72236a25e5774a3ed6ae4c81df4d4` | https://github.com/aamitojj/rover/commit/0ae90f0567e72236a25e5774a3ed6ae4c81df4d4 | AWS, GCP, Azure, OCI, Cloudflare, GitHub/GitLab import, Google API providers and `prov` tests |
| `e2a53fa4ec8d233420bddbd2b17517c7d9b748ca` | https://github.com/aamitojj/rover/commit/e2a53fa4ec8d233420bddbd2b17517c7d9b748ca | Integration Centre API, config v9 migration, nginx-direct installer, first-party analytics |
| `36727a6d51e403c6c9917c1a32a6ae6a0dde86a1` | https://github.com/aamitojj/rover/commit/36727a6d51e403c6c9917c1a32a6ae6a0dde86a1 | Monitoring alerts, Google Sign-In (loopback + PKCE), metadata-only sync, S3/Supabase backup upload |
| `f4fe723253037c2d4a6283c9493d818aed9b589d` | https://github.com/aamitojj/rover/commit/f4fe723253037c2d4a6283c9493d818aed9b589d | Per-site deploy locks, v0.9 tests, config v9 fields |
| `fe4cc93919ea0afc55678f81f283badf31aeea55` | https://github.com/aamitojj/rover/commit/fe4cc93919ea0afc55678f81f283badf31aeea55 | Route wiring, monitor tick, cloud backup destinations, `Version = 0.9.0` |
| `56f84de80396d7739de0ff6bf4773505a1486c69` | https://github.com/aamitojj/rover/commit/56f84de80396d7739de0ff6bf4773505a1486c69 | Catalog wrap, `sbp_` redaction, config-migration tests bumped to v9 |
| `46c85c3ce1a6130a8e6380da94a4f478ab2f349a` | https://github.com/aamitojj/rover/commit/46c85c3ce1a6130a8e6380da94a4f478ab2f349a | Integration Centre, Analytics and Monitoring UI |
| `05b359d12bf703dbb15a63313a45f26c6f75a922` | https://github.com/aamitojj/rover/commit/05b359d12bf703dbb15a63313a45f26c6f75a922 | `docs/INTEGRATIONS.md`, README “New in 0.9”, RESEARCH notes |

This log file is committed after those pushes. Its own SHA is recorded in the finish report (and in a follow-up line if a later push updates this file).

`build.yml` was not changed and was not pushed.

### Test status (local, 2026-11-28, Asia/Calcutta)

- `gofmt -l .` clean
- `go vet ./...` and `GOOS=windows go vet ./...` clean
- `node --check web/*.js` clean
- `go test -count=1 ./...` pass — **151** tests, 0 fail (`hostpanel` + `hostpanel/prov`)
- `go test -race -count=1 ./...` pass
- Linux `go build` and `GOOS=windows GOARCH=amd64 go build` succeed

### CI status

Not yet green at the time this log was first written. Actions should start on the branch after these commits; the run URL and artifact links will be added here once the workflow finishes.

## Spec checklist (all 10 sections)

### 1. Modular provider architecture
- [x] Go `prov.Provider` + registry, official API bases, injectable transport
- [x] Capabilities: hosting, DNS, database, auth, storage, analytics, monitoring, cost
- [x] Providers in catalog: Coolify, nginx direct, This PC, Supabase, Firebase, Netlify, Vercel, Oracle Cloud, AWS, Google Cloud, Azure, Cloudflare, GitHub, GitLab, GA4, Search Console, AdSense, Rover Analytics
- [x] httptest mocks for Test() / signing (SigV4, OCI, cost confirm)
- [ ] Live credential click-through for each cloud account
- [ ] Main-package API + tests for every optional capability method (Netlify ZIP deploy, Cloudflare GraphQL, live GA/GSC/AdSense report proxy still thin)

### 2. Integration Centre
- [x] Page in 0.8 visual style: connect / disconnect / edit / test
- [x] Multiple accounts per provider; credentials never returned to the UI
- [x] DPAPI (Windows) / user-only file (dev) via existing `protectString`
- [x] Health via Test(); cost behind confirm; assign engine/services per project
- [x] States: connected / disconnected / unsupported / failed
- [x] Legacy HostingAccounts / GitAccounts / Cloudflare / Coolify / This PC merged into the list
- [ ] Live usage/plan fields where each vendor exposes them (Test covers reachability; plan/usage is best-effort)

### 3. Universal hosting
- [x] Coolify unchanged
- [x] Independent nginx over SSH: install or reuse on Ubuntu/Debian/Oracle-like, amd64 and arm64, 1 GB RAM tune, swap advice only, never disable SELinux/firewall
- [x] ZIP/folder deploy, preview, history, rollback (existing lite engine + site lock)
- [x] Certbot HTTP-01 with host allowlist; Cloudflare still available
- [ ] Live first public URL + certbot on a real Oracle Linux / Ubuntu / Debian (and ARM / 1 GB) VPS

### 4. Supabase, Firebase, Google Sign-In, sync
- [x] Connect/manage Supabase and Firebase projects (Management / SA)
- [x] Google Sign-In loopback + PKCE; user must create the Desktop OAuth client
- [x] Optional metadata sync; payload refused if it looks like it contains secrets
- [x] SSH keys and tokens never sync
- [ ] Firebase sync write (documented later step)
- [ ] Two-device live sync test after `cloud/schema.sql` is applied

### 5. Analytics
- [x] First-party beacon `/__rover/r.js` and nginx combined-log ingest
- [x] Visitors, pageviews, daily-salt uniques (no cookies, no raw IPs), CF-IPCountry, devices, browsers, referrers, top pages, bandwidth, engagement, 4xx/5xx, bot filter
- [x] Optional GA4 / Search Console / AdSense catalog entries and Data API helpers
- [ ] Optional user-provided MMDB (country today is CF header or documented gap)
- [ ] Wire Integration Centre “Google reports” through to live `GA4RunReport` / `GSCQuery` / `AdSenseReport` (currently a connected note)

### 6. Monitoring
- [x] CPU/RAM/disk/swap/network over SSH `/proc`, uptime, site availability, SSL expiry
- [x] In-app + Windows flash alert; optional HTTPS webhook
- [x] Multiple servers; AWS cost behind confirm
- [ ] Full email alerter (not a mailer yet)
- [ ] Live multi-server soak

### 7. Backups
- [x] Existing manual/scheduled downloadable ZIP, history, restore, retention
- [x] Optional S3-compatible and Supabase Storage upload
- [ ] Live upload to a real bucket

### 8. Security
- [x] Confirm cards kept; Cost Explorer requires `confirm_paid`
- [x] SSH host-key TOFU still present (source test)
- [x] No shell injection; certbot host allowlist
- [x] Existing zip-slip / size limits; atomic deploys; per-project locks
- [x] Least-privilege scopes in `docs/INTEGRATIONS.md`
- [x] Redaction tests; credentials stripped from API responses
- [x] No paid resource is created from the Integration Centre

### 9. Dev requirements
- [x] Backward-compatible project data; config version 9 + `migrateV09` + tests
- [x] Unit / integration (httptest) / regression; gofmt, go vet, `go test -race`
- [x] Production Windows + Linux builds compile locally
- [ ] Headless Chromium UI screenshots in `/workspace/rover-shots-v09/`
- [ ] Windows CI green on this branch (run + artifact URLs)

### 10. Release prep
- [x] `Version = "0.9.0"`
- [x] `docs/INTEGRATIONS.md` and README “New in 0.9”
- [ ] SHA256 of CI Windows exe / installer (after CI)
- [ ] Detailed release notes (draft only — do not publish)
- [ ] Source zip `/workspace/rover-v0.9.zip`
- [x] **No GitHub release, draft, prerelease, or `v0.9.0` tag until the user explicitly says so**

## Honest leftover (do not claim gates pass)

Live nginx/certbot on a public VPS, live tokens for each cloud, Firebase sync write, optional MMDB, email alerts, full Google report proxy, UI screenshots, source zip, and green Windows CI are still open. Release gates have **not** been claimed.
