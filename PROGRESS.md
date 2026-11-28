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
| `3f3898e0f80017eff3cffcc1f4da54212faf21c4` | https://github.com/aamitojj/rover/commit/3f3898e0f80017eff3cffcc1f4da54212faf21c4 | First PROGRESS.md log (test status; CI not yet run) |
| `abef9e0c1ec1dc0841ea495d6f51fd43bba12c65` | https://github.com/aamitojj/rover/commit/abef9e0c1ec1dc0841ea495d6f51fd43bba12c65 | Run `build.yml` on this feature branch; default Windows artifacts to 0.9.0. Offline + publish jobs stay tag-only. |
| `92236af8e7810119e9de91e117bbe9d8c3b9fb29` | https://github.com/aamitojj/rover/commit/92236af8e7810119e9de91e117bbe9d8c3b9fb29 | Progress log with first green CI, SHA256 sums, screenshots; Integrations nav icon uses `plug` |
| `d66b584edb21a94814dd2c5f0c7c0f68e931f42c` | https://github.com/aamitojj/rover/commit/d66b584edb21a94814dd2c5f0c7c0f68e931f42c | Firestore REST helpers (`ToFirestoreFields`, `WriteSyncDocument`) and unit tests |
| `337f53c6388e2172c07bb346eab0b8457d0416ef` | https://github.com/aamitojj/rover/commit/337f53c6388e2172c07bb346eab0b8457d0416ef | Firebase metadata sync writes to `rover_sync/{device}` via official REST; secrets refused; mocked `/api/sync/now` test; INTEGRATIONS.md |
| `372d490dbbe69ae93e29a0c5013493a309d5294a` | https://github.com/aamitojj/rover/commit/372d490dbbe69ae93e29a0c5013493a309d5294a | Progress log for Firebase Firestore sync write |
| `c4fb014947a959652f85fa3823d76e9c8898154a` | https://github.com/aamitojj/rover/commit/c4fb014947a959652f85fa3823d76e9c8898154a | Proxy GA4 `runReport`, Search Console `searchAnalytics.query` and AdSense `reports:generate` through `/api/analytics/{slug}/google`; official URL encoding; mocks |
| `b66dd07b555a188560500f71f4d9210f578e4baf` | https://github.com/aamitojj/rover/commit/b66dd07b555a188560500f71f4d9210f578e4baf | Analytics UI tables for those reports, proxy tests, table styles, INTEGRATIONS.md |
| `713b279383d6c6aa8f261ce055a0d2277a2edde6` | https://github.com/aamitojj/rover/commit/713b279383d6c6aa8f261ce055a0d2277a2edde6 | Progress log for Google report proxy and Firebase CI |
| `35eed7fcd615a321efe57f2e5c21f1ea4ffe867b` | https://github.com/aamitojj/rover/commit/35eed7fcd615a321efe57f2e5c21f1ea4ffe867b | SMTP email alerts (DPAPI-stored password), webhook tests, Monitoring UI |
| `4f2c50a90a56174ad31a1fa11c29d5bed6822631` | https://github.com/aamitojj/rover/commit/4f2c50a90a56174ad31a1fa11c29d5bed6822631 | Document SMTP and https webhook alerts |
| `74709cd43a2ed5e0444e1c305aa55b75ccb20b80` | https://github.com/aamitojj/rover/commit/74709cd43a2ed5e0444e1c305aa55b75ccb20b80 | Progress log for SMTP alerts and Google report CI |
| `e0bb4a5853cd9dd4de49397858bf68d7214f8a33` | https://github.com/aamitojj/rover/commit/e0bb4a5853cd9dd4de49397858bf68d7214f8a33 | Optional MMDB city/region lookup via pure-Go `maxminddb-golang` (CGO off) |
| `cfe51b3dffd5507a7fb90fa375dc90268e5801f3` | https://github.com/aamitojj/rover/commit/cfe51b3dffd5507a7fb90fa375dc90268e5801f3 | MMDB tests, Analytics path settings, DB-IP Lite notes, test fixture |


A follow-up commit on this same branch updates this log with CI results, screenshot paths, SHA256 sums, and a nav-icon fix (`integrations` uses the existing `plug` glyph because `blocks` is not in `web/icons.js`).

### Test status (local, 28 Nov 2026, Asia/Calcutta)

- `gofmt -l .` clean
- `go vet ./...` and `GOOS=windows go vet ./...` clean
- `node --check web/*.js` clean
- `go test -count=1 ./...` pass — **160** tests, 0 fail (`hostpanel` + `hostpanel/prov`) after optional MMDB lookup (was 158 after SMTP alerts)
- `go test -race -count=1 ./...` pass
- Linux `go build` and `GOOS=windows GOARCH=amd64 go build` succeed

### CI status (green)

- Run: https://github.com/aamitojj/rover/actions/runs/46108749616
- Head: `abef9e0c1ec1dc0841ea495d6f51fd43bba12c65`
- Started 10:11:59 AM IST, finished 10:13:32 AM IST (28 Nov 2026, Asia/Calcutta)
- `test` (ubuntu-latest): **success** — gofmt, go vet (linux + windows), `go test -race ./...`, `node --check` — https://github.com/aamitojj/rover/actions/runs/46108749616/job/140867314373
- `windows` (windows-latest): **success** — `rover.exe` + `Rover-Setup-0.9.0.exe` — https://github.com/aamitojj/rover/actions/runs/46108749616/job/140867450782
- `offline` and `publish`: **skipped** (tag / explicit release inputs only). No GitHub release was published.
- Artifact: `rover-0.9.0` (id `15248631494`) — https://github.com/aamitojj/rover/actions/runs/46108749616#artifacts
  - Artifact archive digest (GitHub): `sha256:2ffc648f6ca330ce18dc260fcd2e5c325d8f5a83d6a72118a3a6179ef87ee767`
  - `rover.exe` (14,099,968 bytes) SHA256: `072adf08d3677637068cfca52a7e505220613342044a4380868c43c3631f4f5c`
  - `Rover-Setup-0.9.0.exe` (6,515,679 bytes) SHA256: `23feb5461682e5ad14abbd4148920b6ea2c88cb51ff306d8a367be71467006f0`

A later push of this log will start another Actions run; the green verification run above is the implementation tip `abef9e0`.


### Item 1 — Firebase metadata sync write (28 Nov 2026, Asia/Calcutta)

- Local: gofmt, `go vet` (linux + windows), `go test -race`, Linux and Windows builds all passed (**154** tests at that tip).
- CI (head `372d490`, includes the log): **success** — https://github.com/aamitojj/rover/actions/runs/46109611694
  - Started 10:23:35 AM IST, finished 10:25:21 AM IST (28 Nov 2026, Asia/Calcutta)
  - test: https://github.com/aamitojj/rover/actions/runs/46109611694/job/140869983350
  - windows: https://github.com/aamitojj/rover/actions/runs/46109611694/job/140870134970
  - Artifact `rover-0.9.0` id `15249331578` — https://github.com/aamitojj/rover/actions/runs/46109611694#artifacts
  - archive digest `sha256:7fa5858eb58a94815e37f8e7c4bd32fd2af20badb0738f933e5cf48b46de8ee5`
  - `rover.exe` SHA256 `b6e40bd2d14f090c49f6eb49417efacf261111476ce7bbd0d2b74dad42276033` (14,109,184 bytes)
  - `Rover-Setup-0.9.0.exe` SHA256 `b8ed7a3b7609325048ba5a144f5cf618d78800500055ebc81586004cb27afcd8` (6,518,817 bytes)
- Also green: implementation commits `d66b584` (https://github.com/aamitojj/rover/actions/runs/46109578478) and `337f53c` (https://github.com/aamitojj/rover/actions/runs/46109589236). Offline/publish skipped. No release published.

### Item 2 — GA4 / Search Console / AdSense report proxy (28 Nov 2026, Asia/Calcutta)

- Local: gofmt, `go vet` (linux + windows), `go test -race`, Linux and Windows builds all passed (**156** tests).
- Analytics UI screenshot refreshed: `/workspace/rover-shots-v09/04-analytics.png` (full page, includes Google report cards).
- CI (head `713b279`, includes the log): **success** — https://github.com/aamitojj/rover/actions/runs/46110192830
  - Started 10:31:30 AM IST, finished 10:32:47 AM IST (28 Nov 2026, Asia/Calcutta)
  - Artifact `rover-0.9.0` id `15249566615` — https://github.com/aamitojj/rover/actions/runs/46110192830#artifacts
  - archive digest `sha256:df838574f7cea545eaecb15c53b6aebd734947bbfec0da73f809466c048bdcb7`
- Also green: `c4fb014` (https://github.com/aamitojj/rover/actions/runs/46110126584) and `b66dd07` (https://github.com/aamitojj/rover/actions/runs/46110164443). Offline/publish skipped.

### Item 3 — SMTP email alerter + webhook tests (28 Nov 2026, Asia/Calcutta)

- Local: gofmt, `go vet` (linux + windows), `go test -race`, Linux and Windows builds all passed (**158** tests).
- Monitoring screenshot refreshed: `/workspace/rover-shots-v09/05-monitoring.png` (full page, SMTP fields).
- CI (head `74709cd`, includes the log): **success** — https://github.com/aamitojj/rover/actions/runs/46110648130
  - Started 10:37:23 AM IST, finished 10:38:49 AM IST (28 Nov 2026, Asia/Calcutta)
  - Artifact `rover-0.9.0` id `15248638884` — https://github.com/aamitojj/rover/actions/runs/46110648130#artifacts
  - archive digest `sha256:d37bf5b16078fa72529a629fdb0d11e05ca1f2b983afe58ed505376752dce7ee`
- Also green: `35eed7f` (https://github.com/aamitojj/rover/actions/runs/46110615298) and `4f2c50a` (https://github.com/aamitojj/rover/actions/runs/46110624367). Offline/publish skipped.

### Item 4 — Optional MMDB city/region lookup (28 Nov 2026, Asia/Calcutta)

- Local: gofmt, `go vet` (linux + windows), `go test -race`, `CGO_ENABLED=0` Linux and Windows builds all passed (**160** tests).
- Reader: `github.com/oschwald/maxminddb-golang` v1.13.1 (pure Go, no CGO). User supplies the file (e.g. DB-IP City Lite, CC-BY). Nothing is bundled.
- Analytics screenshot refreshed: `/workspace/rover-shots-v09/04-analytics.png` (geolocation path card).
- CI: recorded after this log is pushed. Implementation tip is `cfe51b3` (code in `e0bb4a5`).

### Screenshots and source zip

- Headless Chromium shots (Linux build, 1440×900): `/workspace/rover-shots-v09/`
  - `01-dashboard.png`, `02-projects.png`, `03-integrations.png`, `04-analytics.png`, `05-monitoring.png`, `06-project-overview.png`, `07-hosting-servers.png`, `08-settings.png`
- Source zip of the branch (written after this log is committed): `/workspace/rover-v0.9.zip`

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
- [x] Firebase sync write via official Firestore REST (`rover_sync/{device}`), secrets refused, httptest mocks
- [ ] Two-device live sync test after `cloud/schema.sql` is applied

### 5. Analytics
- [x] First-party beacon `/__rover/r.js` and nginx combined-log ingest
- [x] Visitors, pageviews, daily-salt uniques (no cookies, no raw IPs), CF-IPCountry, devices, browsers, referrers, top pages, bandwidth, engagement, 4xx/5xx, bot filter
- [x] Optional GA4 / Search Console / AdSense catalog entries and Data API helpers
- [x] Optional user-provided MaxMind-format MMDB (city/region/country); CF header remains the fallback; no database is bundled
- [x] `/api/analytics/{slug}/google` proxies `GA4RunReport`, `GSCQuery` and `AdSenseReport` (httptest mocks); tables shown on the Analytics page. Live credentials still needed for real numbers.

### 6. Monitoring
- [x] CPU/RAM/disk/swap/network over SSH `/proc`, uptime, site availability, SSL expiry
- [x] In-app + Windows flash alert; optional HTTPS webhook
- [x] Multiple servers; AWS cost behind confirm
- [x] SMTP alerter (STARTTLS / TLS / local none), DPAPI-stored password never returned; https webhook tested
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
- [x] Headless Chromium UI screenshots in `/workspace/rover-shots-v09/`
- [x] Windows CI green on this branch (run + artifact URLs above)

### 10. Release prep
- [x] `Version = "0.9.0"`
- [x] `docs/INTEGRATIONS.md` and README “New in 0.9”
- [x] SHA256 of CI Windows exe / installer (above)
- [x] Release notes live in the README “New in 0.9” section and `docs/INTEGRATIONS.md` (no GitHub release body published)
- [x] Source zip `/workspace/rover-v0.9.zip` (packed after this log lands on the branch)
- [x] **No GitHub release, draft, prerelease, or `v0.9.0` tag until the user explicitly says so**

## Honest leftover (do not claim gates pass)

Still open (live credentials / infrastructure only): live nginx/certbot on a public VPS; live tokens for each cloud (two-device Firebase sync, live GA/GSC/AdSense numbers, a real SMTP send, and a user-supplied MMDB file). All four code leftovers are implemented and mocked. Do not tag or publish until the user says so.
