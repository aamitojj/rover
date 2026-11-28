# Rover 0.9 — Integrations

How to connect each provider, which credentials you need, and what Rover
can actually do (and cannot do) through official APIs. **No provider is
required.** Where a capability has no public API, the Integration Centre
marks it *unsupported* instead of faking it.

Credentials are stored on this PC with Windows DPAPI (or a user-only file
on the Linux development build) and are never returned to the UI, written
to logs, included in backups, or synced to the cloud.

> Live verification against a real account is marked **needs live credentials**.
> Everything else is implemented and covered by httptest mocks of the official
> request shapes.

## Least privilege and paid APIs

Rover never creates a paid resource or calls a paid *query* API without an
explicit confirm in the UI. AWS Cost Explorer is the main example: AWS
charges about $0.01 per request, so Rover refuses the call until you confirm.

## Providers

### Coolify (existing)
- **Capabilities:** hosting (supported), monitoring (partial).
- **Credentials:** Coolify URL + API token with deploy permission.
- **Setup:** already used by Rover when you set up a server. The Integration
  Centre shows the active server's token as read-only.
- **Live check:** already used in production-style local tests with a mock Coolify.

### Nginx direct (existing engine, expanded in 0.9)
- **Capabilities:** hosting, monitoring.
- **Credentials:** none (SSH to a server you already added).
- **Setup:** on a server, choose engine *Nginx* (API `POST /api/servers/{id}/engine`).
  Rover installs nginx if it is missing (Ubuntu, Debian, Oracle Linux; amd64 and
  arm64), reuses it if present, tunes worker counts for about 1 GB RAM, and
  prints swap advice. It never disables SELinux or the firewall; it only opens
  80/443 when ufw or firewalld is already active.
- **Deploy:** the same ZIP/folder drop as Coolify, using the 0.7 lite release
  machinery (nginx -t, restore on failure, history, rollback).
- **TLS:** after the domain points at the server, *Enable HTTPS* can run
  certbot (HTTP-01) with allowlisted host names. Cloudflare remains an option
  for tunnels / DNS-01 as in 0.7.
- **Needs live infrastructure** to confirm a first public URL on a real
  Oracle Linux 9 / Ubuntu / Debian VM.

### This PC (existing)
- **Capabilities:** hosting, monitoring.
- Built in. No account.

### Supabase
- **Capabilities:** database, auth, storage (supported). Hosting is *unsupported*.
- **Credentials:** Management API personal access token (`sbp_…`). Optional
  project ref + service role key for REST/Auth/Storage on one project.
- **Scopes:** a personal access token for the Management API; the service role
  key bypasses Row Level Security — treat it like a root password. It stays
  device-local and encrypted.
- **Setup:** Account → Access Tokens. Project Settings → API for the service
  role key.
- **Needs live credentials** for a final click-through on a real project.
- Sync: optional metadata sync uses the SQL in `cloud/schema.sql` (apply it
  in a dedicated Supabase project). SSH keys and secrets are never synced.

### Firebase
- **Capabilities:** hosting, database (Firestore), auth, storage.
- **Credentials:** a service account JSON (or an OAuth access token) and a
  project ID.
- **Scopes:** `firebase.projects` read, Firebase Hosting, Datastore/Firestore
  viewer, Identity Toolkit, Storage object viewer. No write unless you later
  confirm a deploy.
- **Needs live credentials.**
- Sync: metadata is upserted with the official Firestore REST API
  (`PATCH /v1/projects/{id}/databases/(default)/documents/rover_sync/{device}`).
  Grant the service account permission to write that collection (Firestore
  rules or IAM Datastore User). SSH keys and tokens are never included; the
  write is refused if the payload looks like it contains a secret.

### Netlify
- **Capabilities:** hosting (supported). Analytics is *unsupported* (paid add-on,
  no public export API Rover will call).
- **Credentials:** personal access token (User settings → Applications).
- **Needs live credentials** for a real ZIP deploy. Request shape is mocked.

### Vercel
- **Capabilities:** hosting (supported). Web Analytics *unsupported*.
- **Credentials:** access token (Settings → Tokens). Optional team ID.
- **Scopes:** user read, project read, deployment read/create if you deploy.
- **Needs live credentials.**

### Oracle Cloud
- **Capabilities:** hosting (partial — list instances; Rover does not create
  paid shapes), monitoring (partial), cost (partial).
- **Credentials:** tenancy OCID, user OCID, fingerprint, region, API signing
  key (PEM). Official signed requests.
- **Needs live credentials** to list a real tenancy. Signing is unit-tested.

### AWS
- **Capabilities:** hosting (partial: Lightsail + EC2 DescribeInstances),
  storage (S3 ListBuckets), cost (Cost Explorer, **paid**, confirm required).
- **Credentials:** access key ID, secret, region.
- **Scopes:** `lightsail:Get*`, `ec2:DescribeInstances`, `s3:ListAllMyBuckets`,
  optional `ce:GetCostAndUsage`. Do not attach AdministratorAccess.
- **Needs live credentials.** SigV4 is unit-tested; API shapes are mocked.

### Google Cloud
- **Capabilities:** hosting (partial: list Compute instances), storage
  (partial), cost *unsupported* (Billing export lives in BigQuery).
- **Credentials:** service account JSON (or OAuth access token) + project ID.
- **Scopes:** `cloud-platform.read-only` (Resource Manager, Compute list).
- **Needs live credentials.** JWT exchange is implemented.

### Microsoft Azure
- **Capabilities:** hosting (partial: list VMs), cost (Cost Management when
  your enrollment allows it), monitoring (partial).
- **Credentials:** tenant ID, application (client) ID, client secret,
  optional subscription ID. Client-credentials flow against
  `login.microsoftonline.com` and ARM.
- **Scopes:** Reader on the subscription; Cost Management Reader to see spend.
- **Needs live credentials.** Token + ARM list is mocked.

### Cloudflare
- **Capabilities:** DNS, tunnels, analytics (GraphQL `httpRequests1dGroups`).
- **Credentials:** a *scoped API token* (not the Global API key). Optional
  account ID for tunnels and account analytics.
- **Scopes:** Zone:Read, DNS:Edit (only if Rover writes records), Tunnel:Edit
  if you use tunnels, Account Analytics:Read for GraphQL.
- **Needs live credentials** for GraphQL on a real zone. Token verify + zone
  list is mocked. Existing Rover DNS/tunnel code is unchanged.

### GitHub and GitLab
- **Capabilities:** import only. Hosting is *unsupported*. Optional; Rover
  never depends on them at runtime to publish a site.
- **Credentials:** a read-only personal access token (Contents: Read /
  `read_repository`). Self-managed GitLab: paste the base URL.
- Already used for import in earlier versions.

### Google Analytics (GA4)
- **Capabilities:** analytics (Data API `runReport`, Admin account summaries).
- **Credentials:** Sign in with Google (loopback + PKCE) or a service account
  added to the property. Scope: `analytics.readonly` only.
- **Needs live credentials** for a live `runReport`. The Analytics page proxies
  `properties/{id}:runReport` (activeUsers, sessions, screenPageViews by date)
  through `/api/analytics/{slug}/google`. Assign the account (or a property ID)
  on the project if you have more than one.

### Google Search Console
- **Capabilities:** analytics / SEO (`sites` + `searchAnalytics.query`).
- **Scope:** `webmasters.readonly` only.
- **Needs live credentials.** A service account must be added as a user in
  Search Console. Rover proxies `searchAnalytics.query` (top queries and pages)
  through `/api/analytics/{slug}/google`.

### Google AdSense
- **Capabilities:** revenue reports (read-only). Rover never changes
  settings or payments.
- **Scope:** `adsense.readonly` only.
- **Needs live credentials.** Rover proxies `accounts/{id}/reports:generate`
  (estimated earnings, impressions, clicks by date) through
  `/api/analytics/{slug}/google`. Rover never changes AdSense settings or payments.

### Rover Analytics (first-party)
- **Capabilities:** analytics, built in.
- **Credentials:** none.
- A tiny beacon (`/__rover/r.js`) and/or nginx access-log ingest. Metrics:
  visitors, pageviews, estimated uniques (daily salted hash, no cookies),
  country (Cloudflare `CF-IPCountry` or an optional MMDB you provide — MaxMind
  does not allow Rover to bundle GeoLite), region/city when the log provides
  them, devices, browsers, referrers, top pages, bandwidth, engagement,
  4xx/5xx, bot filtering. Retention is configurable (default 90 days). Raw IPs
  are not stored.

## Google Sign-In (personal Rover account)

You must create an OAuth client in Google Cloud Console:

1. Open APIs & Services → Credentials → Create credentials → OAuth client ID.
2. Application type: **Desktop app** (public client, PKCE, no secret).
3. Add an authorised redirect URI `http://127.0.0.1` (loopback; the port is
   chosen at sign-in time). Some clients need you to allow custom loopback ports.
4. Paste the client ID (it ends in `.apps.googleusercontent.com`) in
   Settings / Integration Centre → Sign in with Google.
5. Rover opens Google's own page and listens on `127.0.0.1` only.

Enable the APIs you actually use (Analytics Data, Search Console, AdSense
Management) on that Google Cloud project. Extra scopes are requested only when
you connect those products.

## Cloud sync

Optional. Syncs project *metadata*, settings that are not secret, and deploy
history summaries to **your** Supabase or Firebase project. SSH private keys,
API tokens and other secrets stay on the device and remain DPAPI-encrypted.
The sync is refused if the payload looks like it contains a key.

Apply `cloud/schema.sql` in the destination Supabase project first (RLS is on).
For Firebase, Rover upserts one document per device in the `rover_sync`
collection of the `(default)` database. Give the service account write access
to that collection (or Datastore User on the project).

**Needs live credentials** (and a completed schema / Firestore rules) for a
two-device test.

## Cloud backups

Scheduled/manual ZIP backups stay local by default. You can also upload the
same ZIP (still with no secrets inside) to an S3-compatible bucket or Supabase
Storage, using an Integration Centre account. **Needs live credentials.**

## What still needs live infrastructure

See the per-provider notes above. In short: a real Oracle/Ubuntu/Debian VM
for the first nginx-direct public URL and certbot; real API tokens for a
final click-through of each cloud account; a real Google OAuth client for
Sign-In; a second PC for sync. Mocks cover the official HTTP shapes in CI.
