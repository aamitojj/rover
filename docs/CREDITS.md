# Credits

Rover 0.4 was inspired by these projects and products. **No source code was
copied from them**; only ideas and general UI patterns (see [RESEARCH.md](RESEARCH.md)).
If code is ever copied in future, it must be MIT or Apache-2.0 and credited here
with its license text.

* Netlify — interface layout, project sub-menu, templates, deploy list (product, proprietary; ideas only).
* Vercel and Cloudflare Pages — dashboard and onboarding checklist (products; ideas only).
* Coolify (github.com/coollabsio/coolify) — Rover drives Coolify v4 through its API; project/environment grouping idea.
* Dokploy (github.com/Dokploy/dokploy) — project grouping idea.
* CapRover (github.com/caprover/caprover) and Dokku (github.com/dokku/dokku) — one-click fixes and backup ideas.
* Uptime Kuma (github.com/louislam/uptime-kuma) — health and certificate-expiry monitoring and tags.
* GoAccess (github.com/allinurl/goaccess) — log-based visitor stats without a tracking script.
* Portainer (github.com/portainer/portainer) — bulk actions and list/table filters.
* nginx (nginx:stable-alpine) — the web server image each site runs on.

Please check each project's own repository for its current license.

## 0.5

* llama.cpp (github.com/ggml-org/llama.cpp, MIT) — `llama-server` runs the optional offline brain.
  Not bundled: downloaded on request from the official release (build b12360) and sha256-checked.
* Qwen3 4B Instruct 2507 and Qwen3 1.7B (Qwen team, Apache-2.0), GGUF files by Unsloth
  (huggingface.co/unsloth) — the optional offline models. Not bundled; downloaded on request.
* Okapi BM25 ranking (Robertson & Zaragoza) — Jiv's offline search (own implementation).
* Command palette idea: VS Code, GitHub, Linear (ideas only).

## 0.6

* softprops/action-gh-release (MIT) — publishes GitHub Releases from the build workflow (CI only, not bundled).
* Inno Setup (Jordan Russell & Martijn Laan) — installer, used for silent self-updates.
* APR1 password hashing (Apache httpd `htpasswd -m` format) — own implementation for password-protected sites.
* Ideas only: Netlify (project configuration, Drop, locked deploys), Vercel (instant rollback,
  promote), Cloudflare Pages (direct upload, changed-file uploads), Coolify and Dokploy
  (per-resource tabs, notifications, basic auth).

## 0.7

* cloudflared (github.com/cloudflare/cloudflared, Apache-2.0) — Cloudflare Tunnel connector.
  Not bundled: downloaded on request from the official release 2026.11.2 and sha256-checked.
* nginx (Debian/Raspberry Pi OS package) and `ssl-cert` — lightweight mode on home servers.
* Ideas only: Cloudflare Load Balancing and Tunnel replicas (failover design), Uptime Kuma
  (3-strike health checks), Tailscale/ngrok (reaching machines behind CGNAT).

## 0.8

* Lucide icons (lucide.dev, `lucide-static` 1.80.0, ISC license, parts derived from Feather, MIT)
  — the icons in the app, bundled in `web/icons.js`. License text below.
* Simple Icons (simpleicons.org, CC0 1.0) — brand marks for providers and registrars. The marks
  themselves are trademarks of their owners; Rover uses them only to name the service and does
  not imply endorsement. Services without a usable mark get a generic icon.
* Inter (rsms.me/inter, `@fontsource-variable/inter` 5.3.0, SIL Open Font License 1.1) — the UI
  font, bundled in `web/fonts/` with its license (`Inter-OFL.txt`).
* llama.cpp (MIT) and Qwen3 1.7B GGUF (Apache-2.0) — now **bundled** in the offline installer
  and `.dlpack` packs (still downloaded on request in the standard installer). Their license
  texts are installed with them (`{localappdata}\Rover\Brain\licenses`, from `installer/licenses/`).
* Ideas only: Intro.js, Shepherd.js, Driver.js (guided tours), Raycast and Linear (dark UI).

### Lucide license

```
ISC License

Copyright (c) 2026 Lucide Icons and Contributors

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

---

The following Lucide icons are derived from the Feather project:

airplay, alert-circle, alert-octagon, alert-triangle, aperture, arrow-down-circle, arrow-down-left, arrow-down-right, arrow-down, arrow-left-circle, arrow-left, arrow-right-circle, arrow-right, arrow-up-circle, arrow-up-left, arrow-up-right, arrow-up, at-sign, calendar, cast, check, chevron-down, chevron-left, chevron-right, chevron-up, chevrons-down, chevrons-left, chevrons-right, chevrons-up, circle, clipboard, clock, code, columns, command, compass, corner-down-left, corner-down-right, corner-left-down, corner-left-up, corner-right-down, corner-right-up, corner-up-left, corner-up-right, crosshair, database, divide-circle, divide-square, dollar-sign, download, external-link, feather, frown, hash, headphones, help-circle, info, italic, key, layout, life-buoy, link-2, link, loader, lock, log-in, log-out, maximize, meh, minimize, minimize-2, minus-circle, minus-square, minus, monitor, moon, more-horizontal, more-vertical, move, music, navigation-2, navigation, octagon, pause-circle, percent, plus-circle, plus-square, plus, power, radio, rss, search, server, share, shopping-bag, sidebar, smartphone, smile, square, table-2, tablet, target, terminal, trash-2, trash, triangle, tv, type, upload, x-circle, x-octagon, x-square, x, zoom-in, zoom-out

The MIT License (MIT) (for the icons listed above)

Copyright (c) 2013-present Cole Bemis

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
