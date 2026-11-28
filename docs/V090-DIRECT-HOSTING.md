# Rover v0.9.0 — Direct Cloud Hosting
Status: planned; no release until acceptance gates pass.

## Engines
Explicit per-server selection: direct nginx (recommended for 1 GB RAM), Coolify, existing local PC. No Docker or Coolify prerequisite for direct static hosting. Reuse existing lightweight nginx, SSH runner, archive validation and release machinery; preserve existing server modes and migrations.

## Deployment
ZIP/folder drag and drop, detect index.html/dist/build/public, prebuilt React/Vite/static sites, SSH/SFTP progress, public IP preview (unique routing per project), redeploy, enable/disable, rename/delete, history with size/date/status, download ZIP, atomic rollback. No GitHub requirement. Validate archive paths, symlinks and extraction limits; serialize conflicting operations on the server; test nginx config and restore previous config on failure.

## Oracle and Linux
Discover existing Oracle instances with authorized API access; no paid creation without explicit approval. SSH private key, host fingerprint verification, Ubuntu/Debian/Oracle Linux 9.9, AMD64/ARM64. Detect installed nginx and reuse it. Handle package manager, SELinux, firewall and OCI ingress separately; preserve SSH and unrelated websites. Never silently disable SELinux or firewall. No mandatory builds or unnecessary services.

## Domains
Custom domains/subdomains, DNS instructions and verification, IP preview, Let's Encrypt after DNS validation, HTTPS redirect, automatic renewal and expiry alerts. Avoid altering unrelated nginx virtual hosts.

## Analytics and monitoring
Privacy-conscious first-party analytics: pageviews, estimated unique visitors, country, referrers, popular pages, browser/device, daily/weekly/monthly reports, bandwidth, errors, bot filtering, retention. No mandatory GA. Lightweight periodic CPU/RAM/swap/disk/network/uptime/nginx/site/SSL checks and alerts. Avoid retaining raw IPs unnecessarily.

## Accounts and backups
Existing local authentication remains intact; cloud account authentication is not yet implemented. Supabase + optional Google sign-in, owner-isolated project/history/hosting metadata, per-device SSH authorization; no unencrypted private keys or device-bound credentials in cloud. Local scheduled/manual backups, restore, ZIP download, retention and storage limits. Existing local mode remains usable.

## Release gates
- Regression tests for existing features and onboarding.
- Direct deploy, redeploy, rollback, interrupted upload, SSH failures, archive attacks, concurrent deployment, nginx failure and unrelated-site preservation.
- Real Oracle Linux 9.9 and Ubuntu/Debian tests; AMD/ARM and 1 GB RAM verification.
- Domain, SSL issuance and renewal tests with authorized infrastructure.
- Cross-account isolation and multi-device sync tests.
- Windows installer smoke test, release notes and SHA256 checksums.
- Primary acceptance: connect existing Oracle server, drop static folder, open public URL without GitHub, Docker or Coolify.
- Do not publish v0.9.0 as complete until gates pass; document any unverified environments.
