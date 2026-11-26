# Rover v0.8

<img src="web/icon.png" width="64" align="right" alt="">

Rover is a Windows app that works like Netlify, but on **your own server**.
Drop a folder, paste a GitHub link or import from Netlify, and your site is online
with HTTPS in a minute. Under the hood it sets up and drives
[Coolify](https://coolify.io) on a VPS for you. You don't need Docker, the
command line or GitHub.

**New in 0.8** (see [docs/RESEARCH.md](docs/RESEARCH.md#rover-08--research-notes))

* **Dilreet is now Rover.** New name, logo and icon (`rover.exe`, `Rover-Setup-0.8.0.exe`,
  Start menu "Rover"). Jiv keeps its name. On first start Rover moves `%APPDATA%\Dilreet`
  to `%APPDATA%\Rover`, `%LOCALAPPDATA%\Dilreet` to `%LOCALAPPDATA%\Rover` and
  `Documents\Dilreet` to `Documents\Rover`, rewrites saved paths and leaves a
  `MOVED-TO-ROVER.txt` note. If a folder is locked (OneDrive, antivirus), settings are copied and
  the other folders are simply used where they are. The installer upgrades an existing Dilreet
  in place (same AppId), and the updater still understands Dilreet 0.6/0.7 release assets.
  Servers are untouched: `/data/hostpanel`, `dilreet-*` tunnels and nginx files keep their names.
* **Jiv "Show me how".** Step-by-step walkthroughs that highlight the real button on screen,
  wait for you to click it, check the result (for example "the domain points to the server")
  and skip steps that are already done. 40+ built-in guides; with the offline brain Jiv can also
  write a guide for a question it has no guide for (only real, known buttons are used).
* **Jiv "Do it for me".** Jiv turns a request ("put bakery on joesbakery.com with https and a
  backup on my home pc") into a plan of named steps, shows it, asks once (and again for risky
  steps), runs it with live progress, and stops and explains on the first failure. Read-only checks
  run first. Plans come from built-in templates; the offline brain fills in the rest, limited to
  24 known tools by a grammar.
* **Offline pack and offline installer.** `Rover-Offline-Setup-0.8.0.exe` (split into parts under
  2 GB) includes the Light brain and both llama.cpp engines. `.dlpack` files carry the same on a USB
  stick: Settings > Jiv > Import offline pack, or Export to make one. Every file is checked against
  SHA-256 values built into Rover.
* **Graphics card offload.** A dedicated NVIDIA, AMD or Intel GPU with enough memory runs the
  offline brain with llama.cpp's Vulkan build; otherwise the CPU build is used. If the GPU run
  fails, Rover falls back to the CPU by itself.
* **New look.** Dark-first theme (Light and "Same as Windows" in Settings), the Inter font bundled
  offline, cards with consistent spacing, short animations that switch off with Windows'
  "reduce motion", and icons everywhere: sidebar, tabs, buttons, menus, statuses, providers
  (brand marks where their licence allows) and file types. Still no frameworks and no internet needed.

**New in 0.7** (see [docs/RESEARCH.md](docs/RESEARCH.md#rover-07--research-notes) and [docs/HOME-HOSTING.md](docs/HOME-HOSTING.md))

* **One project on several servers at once.** Project → **Hosting** → **Add a backup server**.
  Every deploy goes to all servers with the same version name: the files are sent everywhere
  first, then each server switches atomically. A deploy counts as live only when every server
  switched; otherwise the project shows *live on some servers*, which server failed and why,
  and a **Retry** button that resends the live version from the copy on this PC. **Check health
  & sync** asks every server which version it serves. Rollbacks apply on every server.
* **Automatic failover, free.** *Primary + backup* (default): Rover checks every server each
  minute; after 3 failed checks it points the domain at a healthy backup through the Cloudflare
  API (proxied record, live in seconds), notifies you, and switches back after 3 good checks
  (optional). *Together*: one Cloudflare Tunnel with a connector on every server, so Cloudflare
  routes around a dead server even while your PC is off. **Switch now** (with a confirm click)
  switches by hand. Without Cloudflare, Rover still checks and tells you what to change at
  your registrar.
* **Your own servers at home.** *Hosting accounts → My own server (at home)*: an old PC, mini PC
  or Raspberry Pi on your network. Password once over SSH, then Rover's own key. Rover reads
  memory and processor (ARM64, ARMv7, x86) and picks **Lightweight** mode (plain nginx, ~10 MB,
  several sites side by side) or the full Coolify engine. A **Cloudflare Tunnel** (official
  `cloudflared`, fixed version, sha256-checked, systemd service, token never on a command line)
  makes it reachable behind CGNAT and changing IPs, with no router changes. Port forwarding +
  dynamic DNS is the guided alternative.
* **Host on this PC.** A built-in web server that applies the same `_redirects`, `_headers`,
  `netlify.toml` rules, password protection and primary-domain redirect as the servers; a
  Cloudflare tunnel via `cloudflared.exe` (downloaded on demand, checksum-checked); **Start with
  Windows**, **Keep this PC awake** (SetThreadExecutionState), battery/power warnings, and a
  **connection monitor** that shows Wi-Fi vs. phone (USB tethering/hotspot) and alerts when the
  PC goes offline or falls back to mobile data. This PC can be a backup server for any project.
* Jiv knows all of it (8 new offline articles, English + Hinglish). Settings migrate automatically.

**New in 0.6** (see [docs/RESEARCH.md](docs/RESEARCH.md#rover-06--research-notes))

* **Each project is its own control room, like a Netlify site.** Open a project and the
  left menu shows **Overview** (live address, status, quick actions and a *Set up this project*
  checklist where each step is a button), **Deploys**, **Domains**, **Hosting**, **Configuration**
  (General, Build & deploy, Redirects & headers, Password protection, Notifications, Backups,
  Danger zone), **Files**, **Testing** and **Visitors & logs**. Jiv knows which project you are in.
* **Drop the newest build to deploy.** Drag a folder or `.zip` onto the big drop area, or
  anywhere on the project page. It goes live straight away, with progress, the live address
  and an **Undo (roll back)** button. Hold Shift (or tick *Save as draft*) to save a **draft** you
  can preview and publish later. **Lock publishing** turns automatic deploys (watched folder,
  Git) into drafts. Deploy history has download, compare, preview, publish and roll back.
* **Faster deploys.** Only files that changed are uploaded (the server copies the rest from
  the live version). Rover falls back to a full upload when that is safer.
* **Faster to use.** *Redeploy last folder*, paste a copied folder path (Ctrl+V), recent folders,
  keys `1`–`8` for the tabs, `D` deploy, `R` redeploy, `O` open the site, and an optional
  **Deploy with Rover** entry in File Explorer's right-click menu for folders and .zip files.
* **New settings per project:** build command and output folder, password protection
  (HTTP basic auth), primary domain with automatic www / non-www redirect, an in-app editor
  for `_redirects` and `_headers` with a *Check* button, per-project alerts, opt out of backups.
* **Automatic updates from GitHub Releases.** *Settings → Updates*: **Check now**, check
  automatically (daily and at start-up, on by default), ask first or install by itself,
  Stable or Beta channel, *Remind me later* and *Skip this version*. Downloads are checked
  against `SHA256SUMS.txt` (and an optional ed25519 signature) before anything is installed.
  No account or token is needed; with no internet Rover simply says so and carries on.
  Publishing a release is one button on GitHub: see [docs/RELEASING.md](docs/RELEASING.md).

**New in 0.5** (see [docs/RESEARCH.md](docs/RESEARCH.md#rover-05--research-notes))

* **Jiv works fully offline.** A built-in knowledge base (60 articles, English + Hindi,
  BM25 search with typo and Hinglish handling) answers questions about every screen and
  error. Jiv understands requests (*"roll back bakery"*, *"why is my site down?"*, *"connect
  my Hetzner account"*), checks your sites, domains and servers, and offers fix buttons.
  Every server, DNS or settings change is a card you confirm; read-only checks run directly.
* **Optional offline brain.** *Settings → Jiv* downloads llama.cpp (`llama-server`, MIT)
  and a small model (Qwen3 4B Instruct 2507 or Qwen3 1.7B, Apache-2.0), checked against
  fixed sha256 fingerprints, resumable, or imported from files you already have. It runs
  as a local sidecar on `127.0.0.1` with a random key, starts on demand and unloads when idle.
  Cloud AI is still optional and needs a one-time consent per provider.
* **Hosting accounts portal.** Connect several accounts (even several per provider):
  DigitalOcean (browser sign-in, OAuth + PKCE), Hetzner, Vultr, Linode/Akamai, AWS Lightsail,
  Oracle Cloud, Contabo, Hostinger VPS, Scaleway, UpCloud, or any VPS by IP + SSH.
  See each account's servers, **Use this server for Rover** (auto-setup), **Create server**
  (price first, confirm click), rename, remove. Tokens are DPAPI-encrypted and never sent to any AI.
* **Several servers.** A server switcher in the sidebar; each project remembers its server,
  can be moved to another one, and Rover offers to switch when you open a project on another server.
* **Easier:** Ctrl+K command palette, keyboard shortcuts (`?`), undo for archive/pin/bulk changes,
  *Explain this page*, *Ask Jiv why* on errors, suggested questions per screen, help offered
  when a job fails, chat history saved on this PC. 0.4 settings migrate automatically.

**New in 0.4**

* **Dashboard** — counts, a *Getting started* checklist, *Needs your attention* with
  **Fix it** buttons, pinned projects and recent deploys.
* **Organised** — groups (shown in the left menu), coloured tags, pins, search,
  filters (status, tag, built with), sort, card or list view, and archive.
* **Bulk actions** — select many projects to pin, move, tag, check, redeploy or archive.
* **Templates** — shop, portfolio, link-in-bio, restaurant menu, event, coming soon, blank.
* **Automatic** — health and padlock (SSL) checks with notifications, scheduled backups
  (*Settings → Backups & alerts*), duplicate a project, *Visitors & logs* from the server log.
* **Easier words** — plain language, tooltips for words like DNS and SSL, empty states,
  light/dark theme. Old settings are migrated automatically.
* Every server or DNS change still asks you to confirm first.

**New in 0.3**

* **A real app.** `rover.exe` opens its own window (WebView2, built into
  Windows 10/11), remembers its size and position, has a Start-menu entry and an
  installer (`Rover-Setup-0.3.0.exe`). `-browser` still opens it in your
  normal browser.
* **Projects**, like Netlify: a project list, **Add new project** (folder or
  zip, Git or link, Netlify), and per project **Overview / Files / Deploys /
  Domain / Settings**. Every project has a folder on your PC:
  `Documents\Rover\Projects\<name>\current` (the live files) and
  `deploys\<time>_<id>.zip` (every deploy) with `deploys\index.json`. Compare any two
  deploys, download one, publish an older one again, or open the folder in
  Explorer. **Settings → Storage** shows disk usage, moves the folder and sets how
  many deploys to keep.
* **Domains made easy.** Rover recognises where the domain is (GoDaddy,
  Namecheap, Porkbun, Hostinger, Squarespace, Cloudflare…) and shows exact steps
  with a button to that registrar's DNS page, copy buttons, warnings and a live
  check that notifies you when the domain works. With a **GoDaddy** or **Porkbun**
  API key it adds the records itself; **Fully automatic** moves DNS to a free
  Cloudflare account once. See [docs/AUTOMATION.md](docs/AUTOMATION.md).
* **Jiv**, the built-in assistant, is at the bottom of every screen ("Ask Jiv
  anything…"). It answers questions, explains errors, walks you through domains,
  and can do things for you after you click a confirm card. It connects to a free
  local AI on your PC (Ollama, LM Studio, Jan, GPT4All, llama.cpp, LocalAI, vLLM,
  text-generation-webui), or, only if you choose it, to a cloud AI (OpenAI,
  Anthropic, Gemini, xAI, OpenRouter, Mistral, Groq, DeepSeek or any
  OpenAI-compatible URL). Without AI it answers from the built-in guide.
* **Zero-touch server setup**: create a Hetzner or DigitalOcean server from the
  app, install Coolify and get an API token without opening the Coolify dashboard.
  Auto-deploy from Git, Netlify migration, and background checks.

---

## Quick start

1. Install with `Rover-Setup-0.3.0.exe` (or just run `rover.exe`). Windows
   SmartScreen may warn because the exe is unsigned: *More info → Run anyway*.
   If WebView2 is missing (rare on Windows 10/11), Rover offers to open the
   Microsoft download page or to use your browser.
2. **Connect a server.** Either paste a Hetzner/DigitalOcean API token and click
   **Create server**, or enter the IP and root password/SSH key of any fresh
   Ubuntu 22.04/24.04 VPS (2 vCPU / 2 GB minimum; 4 vCPU / 8 GB for ~100 sites).
   Open ports 22, 80, 443 (and 8000 for the Coolify dashboard). Click **Set up
   automatically**: Rover installs Coolify and creates its admin account and API
   token.
3. **Add new project** → drop a folder/zip, paste a Git link, or import from
   Netlify. You get `https://<name>.<server-ip>.sslip.io` straight away.
4. **Domain** tab → type your domain and follow the steps (or let Rover do it).
5. Edit files in the project's `current` folder and click **Deploy from this
   folder**, drop a new version, or push to Git with auto-deploy on.

Stuck? Ask Jiv.

---

## App Testing

1. **Load a site**: pick a `.zip`, a folder, or type a path on this PC (paths
   can be reloaded after you edit files). Rover copies it into a private temp
   workspace (`%TEMP%\dilreet-test-*`, deleted when you load another site,
   click **Stop**, or quit), using the same cleaning as deployments (zip-slip
   protection, no `__MACOSX`/`.git`/`node_modules`). The published folder is
   taken from `netlify.toml` (`[build] publish`), or auto-detected
   (`dist/`, `build/`, `out/`, `public/`... when it has `index.html`).
2. **Local preview server** on `http://localhost:<random port>`, only on the
   loopback interfaces (127.0.0.1 and ::1). It follows Netlify's rules:
   * `_redirects` and `[[redirects]]` / `[[headers]]` in `netlify.toml`, plus
     `_headers`;
   * status 200 (rewrite), 301, 302, 303, 307, 308, 404 and 410 (with a
     custom page), forced rules (`200!`);
   * splats (`/blog/*  /news/:splat`) and placeholders
     (`/users/:id  /profile.html?u=:id`);
   * "shadowing": an existing file wins over a non-forced rule, as on Netlify;
   * SPA fallback (`/*  /index.html  200`), custom `404.html`, clean URLs
     (`/about` → `about.html`, `/dir` → `/dir/`);
   * the rule files themselves are never served.
3. **Built-in browser**: address bar, back/forward/reload, home, open in your
   normal browser, sizes 375×667 (phone), 768×1024 (tablet), 1280×800
   (desktop), full width or a custom size, rotate and fit-to-window. A small
   script is injected **only into previewed HTML pages** (never into deployed
   files). It reports JavaScript errors, `console.error`/`warn`, unhandled promise
   rejections, failed images/scripts/styles, and failed `fetch`/XHR calls. The
   server also logs every 4xx/5xx response. Everything appears in the console
   panel ("Problems" or "All"). Links to other websites are not opened inside the
   preview; a notice offers to open them in your browser.
4. **Checks report**: every check is *pass*, *warn* or *fail*, with an
   explanation and how to fix it:
   home page, published folder, built output vs. unbuilt source (React/Vue/Vite/
   Angular/Next... detection), internal links, images/scripts/styles (incl.
   `srcset`, CSS `url()`, `@import`), upper/lower-case mismatches (work on
   Windows, 404 online), `C:\`/`file://` paths, `http://` mixed content,
   `#anchor` targets, large files, title/viewport/lang/charset/favicon,
   redirect/header rule problems, SPA routing, secret files (`.env`, keys,
   `.npmrc`...), Netlify-only features (Forms, Functions, Edge), and the custom
   404 page. The report also names missing files that an SPA rule hides by
   answering with `index.html`. **View** links open the affected page in the
   preview. **Export** downloads the report plus the browser log as JSON, and
   **nginx config** shows the exact server config the rules will produce.
5. **Deploy this version**: choose a project and click the button. If any check
   failed, Rover lists the failures and asks you to confirm. Otherwise it
   asks once, showing the build fingerprint. It then deploys the **exact files you
   tested** from the temp workspace through the normal deploy (SFTP, new
   release, atomic switch, rollback). If the workspace was changed or removed
   meanwhile, it refuses.

### The built-in browser

The built-in browser is an `<iframe>` inside the Rover window (WebView2, the
Edge engine), so it is a real browser engine, and the injected script captures
console and network problems. The preview runs on a different origin
(`localhost` vs. the UI's `127.0.0.1`), so the previewed site cannot read or
call Rover. Limits: the preview can't show other websites in the frame (most
forbid it), and sites that sent `X-Frame-Options`/`frame-ancestors` have those
headers relaxed **in the preview only** (logged as info). For full DevTools,
click **Open in browser**.

### Redirects and headers on the server

On every deployment Rover converts the rules into
`/data/hostpanel/sites/<id>/nginx/default.conf`. This folder is mounted into
the container at `/etc/nginx/conf.d`, with one config per release in
`nginx-releases/`, so rollbacks restore the matching rules. Rover runs
`nginx -t` before switching. If nginx rejects the config, the deployment stops
and the live site is unchanged. Then it reloads nginx (no restart). Sites created
with 0.1 get the mount added automatically on their first 0.2 deploy (one
container restart, about 30-60 s). A test suite runs the generated config in
a real nginx and compares every answer with the preview server.

---
## How deployments work without GitHub

Each site is **one Coolify project** containing **one "Docker Image"
application** (`nginx:stable-alpine`, port 80). These are created with the
documented API endpoints `POST /api/v1/projects` and
`POST /api/v1/applications/dockerimage`.

The site's files are on the server's disk at `/data/hostpanel/sites/<site-id>/`,
which is bind-mounted into the container at `/usr/share/nginx`
(`POST /api/v1/applications/{uuid}/storages`, type `file`, `is_directory: true`):

```
/data/hostpanel/sites/<site-id>/
├── html -> releases/20261023-101500      (symlink = what nginx serves)
└── releases/
    ├── 20261023-101500/   (newest, live)
    ├── 20261022-180312/
    └── initial/           (placeholder page)
```

When you deploy, Rover:

1. builds a cleaned `.tar.gz` on your PC. It blocks path tricks like `../`
   (zip-slip) and leaves out `__MACOSX`, `.DS_Store`, `Thumbs.db`, `.git`, and
   `node_modules`;
2. uploads it over SFTP;
3. unpacks it into a new release folder and switches the `html` symlink to it
   in one atomic step.

Nginx serves the new files immediately. No container rebuild or restart is
needed, and visitors never see a half-uploaded site.

Why this approach:

* It only uses API endpoints that exist in Coolify's official OpenAPI spec. A
  Dockerfile app with no git repository has no build context for your files,
  and pushing images would need a registry.
* Deploys take seconds and don't depend on Docker builds.
* Coolify still handles the parts it's good at: the container, the Traefik
  proxy, domains, and Let's Encrypt certificates.

Domains are set with `PATCH /api/v1/applications/{uuid}` (`domains`, optionally
`force_domain_override`), followed by `POST /api/v1/deploy?uuid=...` so the proxy
picks them up. For older Coolify versions where these actions are `GET`,
Rover falls back to `GET` automatically.

---
## Security

* **SSH passwords and key passphrases are never saved.** They stay in memory
  only while the app runs, so you enter them once per session. Use key
  authentication if you'd rather not type a password.
* The server's SSH host key is pinned on first connect (trust on first use).
  If it changes, Rover refuses to connect.
* The Coolify API token is saved encrypted with **Windows DPAPI**, which ties it
  to your Windows user account. On Linux and macOS development builds it is
  stored in a user-only (0600) file.
* The local web UI only listens on 127.0.0.1. It needs the random session key
  printed in the console (stored as an HttpOnly, SameSite=Strict cookie),
  rejects requests with a foreign `Host` header (DNS rebinding), and needs a
  custom header on every state-changing request (CSRF). In 0.2 it also
  rejects cross-site requests (`Sec-Fetch-Site`/`Origin` checks), sends a strict
  CSP, and refuses to be framed.
* The App Testing preview server only listens on loopback (127.0.0.1/::1),
  accepts only `GET`/`HEAD`, checks the `Host` header, runs on a different
  origin than the UI, and serves only files from the temp workspace. It has no
  route to the Rover API (`/api/...` on the preview is just a path of your
  site). Its only link back is a log endpoint on the preview's own port, which
  accepts browser-log messages from the preview page.
* If your SSH user isn't root, commands run through `sudo`, and the password
  you typed (memory only) is used for sudo.
* Self-updates are only accepted over HTTPS and only installed if the SHA-256
  matches.

* API keys for cloud AI, Git, Netlify, Cloudflare, Hetzner/DigitalOcean, GoDaddy
  and Porkbun are DPAPI-encrypted, never logged, and only shown again as a hint
  (`…a1b2`).
* Jiv uses AI on this PC by default. A cloud AI is used only after you add its
  key and accept a one-time consent per provider. SSH passwords, tokens and keys
  are never sent; secrets and IP addresses are redacted; "Include my status" can
  be switched off.
* Every change Jiv suggests appears as a card you must click. Nothing runs by itself.

Config file: `%APPDATA%\Rover\config.json` (an old `%APPDATA%\HostPanel`
folder is moved there automatically on first start). It holds the server IP, user,
key path, pinned host key, Coolify URL, encrypted tokens, the update URL and your
project list. It never contains your SSH password.

---
## Self-update

Rover updates itself from the **GitHub Releases** of its repository (default
`aamitojj/rover`; change it under *Settings → Updates → Advanced*). It uses GitHub's
public API, so no account or token is needed.

* **Check now** asks GitHub for the newest release. **Check automatically** (on by
  default) checks 45 seconds after start-up and then once a day.
  Offline? Rover says *"You're offline"* and tries again later; nothing else is affected.
* **When an update is found:** *Ask me first* (default) shows the release notes with
  **Install and restart**, **Remind me later** (1 day) and **Skip this version**.
  *Install by itself* downloads and installs without asking (never while a deploy runs).
  A red badge on *Settings* and an *Update available* button in the sidebar show it too.
* **Channel:** *Stable* uses only normal releases. *Beta* also offers pre-releases.
* **Safety:** downloads come only from github.com / githubusercontent.com over HTTPS.
  Rover downloads `SHA256SUMS.txt` from the same release and refuses the update if the
  file's SHA-256 does not match it (or GitHub's own digest for the file). Builds that carry a
  public key (`-X main.updatePublicKey=…`) also require a valid ed25519 signature
  (`SHA256SUMS.txt.sig`). A release without `SHA256SUMS.txt` is never installed.
* **How it installs:** installed copies (with `unins000.exe` next to the exe) run the new
  `Rover-Setup-x.y.z.exe` silently (`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART`), which
  closes Rover, updates it and starts it again. Portable copies download `rover.exe`,
  rename the running exe to `rover.exe.old`, move the new one into place and restart;
  if any step fails, the old exe is put back. Your settings and projects are never touched.

Set `ROVER_NO_UPDATE_CHECK=1` to switch off all update checks (for example on managed PCs).

**Updating Coolify:** *Settings → Update Coolify on the server* runs the official
installer again over SSH, which upgrades an existing installation to the latest
version. The installed version comes from `GET /api/v1/version`, and the latest
version from `https://cdn.coollabs.io/coolify/versions.json`. Coolify's API has
no "upgrade" endpoint. Your websites keep running while the dashboard restarts.

---
## Command-line flags

```
rover.exe [-browser] [-port 8787] [-no-browser] [-print-url] [-debug] [-deploy <folder or .zip>] [-autostart]
```

`-deploy` opens Rover (or the copy already running) and asks which project to deploy the
folder to; the installer's Explorer menu uses it. `-after-update` is internal (used by the updater).
`-autostart` is used by *Start with Windows* (runs in the background with `-browser -no-browser`).

Environment variables for testing: `ROVER_CONFIG_DIR` (settings folder),
`ROVER_PROJECTS_DIR` (projects folder), `ROVER_BRAIN_DIR` (offline brain folder),
`ROVER_NO_BACKGROUND=1`. `ROVER_*` is read first; the old `DILREET_*` and `HOSTPANEL_*`
names still work. The Light-model eval runs with
`ROVER_EVAL_GGUF=<model.gguf> ROVER_EVAL_LLAMA=<llama-server> go test -run JivEvalLightModel -v .`

## Building from source

Requires Go 1.26 or newer (pure Go, no cgo).

```
go vet ./... && go test -race ./...
go run github.com/tc-hib/go-winres@latest make --in winres/winres.json --arch amd64   # icon + manifest
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H=windowsgui -X main.Version=0.8.0" -o rover.exe .
iscc /DAppVersion=0.8.0 installer\rover.iss                 # -> dist\Rover-Setup-0.8.0.exe
iscc /DAppVersion=0.8.0 /DOffline=Light installer\rover.iss  # -> dist\Rover-Offline-Setup-0.8.0*.exe/.bin (needs offline\stage)
```

`.github/workflows/build.yml` runs the tests on every push and builds `rover.exe`, the
installer and `SHA256SUMS.txt`. A `v*` tag, or the one-click **Release** workflow
(`.github/workflows/release.yml`), also publishes them as a GitHub Release, which is what
the self-updater reads. See [docs/RELEASING.md](docs/RELEASING.md). `devtools/mockcoolify` is a
development-only fake Coolify API used for end-to-end tests.

## Source layout

| File | Purpose |
|---|---|
| `main.go` | HTTP server, routes, auth, startup |
| `desktop_windows.go` | WebView2 window, single instance, window placement |
| `projects.go` | project folders, deploy archive, index.json, files/deploys/storage API |
| `registrars.go`, `nameservers.go` | registrar detection, GoDaddy/Porkbun APIs, Cloudflare move |
| `cloudflare.go`, `dns.go` | Cloudflare API, required records and DNS checks |
| `automation.go`, `cloudservers.go` | zero-touch Coolify setup, Hetzner/DigitalOcean |
| `gitimport.go`, `imports.go`, `netlify.go`, `build.go` | Git/link/Netlify import, auto-deploy, builds |
| `ai.go`, `ai_providers.go` | Jiv: local AI detection, cloud providers, tools, consent |
| `sshclient.go`, `coolify.go`, `sites.go` | SSH/SFTP, Coolify API, create/deploy/rollback |
| `workspace.go`, `preview.go`, `checks.go`, `apptest.go`, `netlifyrules.go`, `nginxconf.go` | App Testing |
| `secret_windows.go` | DPAPI encryption |
| `update.go` | self-update from GitHub Releases (check, verify, install) |
| `project06.go` | per-project settings, password protection, drafts, incremental deploys, quick deploy |
| `multihost.go` | 0.7: one project on several servers, deploy to all, retry/sync, rollback everywhere |
| `failover.go` | 0.7: health checks, failover state machine, Cloudflare DNS switching |
| `tunnel.go` | 0.7: Cloudflare Tunnel API, ingress rules, cloudflared install (pinned + sha256) |
| `homeserver.go` | 0.7: home Linux servers, lightweight nginx mode, port-forwarding guide |
| `thispc.go`, `thispc_windows.go` | 0.7: hosting on this PC, keep-awake, autostart, power and network monitor |
| `rename.go` | 0.8: Dilreet → Rover folder migration, `ROVER_*`/`DILREET_*`/`HOSTPANEL_*` env lookup |
| `jiv_guides.go`, `jivguides/` | 0.8: "Show me how" walkthroughs (built-in and model-written) |
| `jiv_plan.go`, `jiv_plan_run.go`, `jiv_ask08.go` | 0.8: "Do it for me" plans, plan tools, runner |
| `jiv_pack.go`, `dlpack/` | 0.8: offline packs (`.dlpack`, split parts), import/export, offline installer brain |
| `gpu.go`, `gpu_windows.go`, `gpu_other.go` | 0.8: graphics card detection and offload |
| `web/rover.css`, `web/icons.js`, `web/ui08.js`, `web/fonts/` | 0.8: design system, icon sprite (Lucide, Simple Icons), icon decorator, Inter |
| `tools/signsums` | optional ed25519 signing of release checksums |
| `web/` | the embedded UI (no external assets) |

## Known limitations (v0.3)

* Static sites only (plus an optional local `npm run build`); no server-side code,
  Netlify Functions, Forms or SSR.
* The Files tab's HTML preview is sandboxed; pages that load images or CSS from
  the project may show without them there. Use App Testing for a full preview.
* GoDaddy/Porkbun/Cloudflare automation, Hetzner/DigitalOcean server creation,
  Netlify import and private Azure DevOps/Bitbucket repos were tested against mock
  APIs only, not live accounts.
* Namecheap, Hostinger and Squarespace use guided steps (no usable public API).
  Domain Connect is not supported (see docs/AUTOMATION.md).
* Project details live in the local config; projects created on another PC don't
  show up automatically.
* The exe and installer are unsigned (SmartScreen warning).
* You enter the SSH password once per session unless you use key auth (Rover
  can install a key for you during setup).
* Server paths are still `/data/hostpanel/sites/<id>` so servers set up by older
  versions keep working.

## Known limitations (v0.5)

* The offline brain is optional and small: the Light model (1.7B) sometimes mixes up steps;
  Jiv therefore always shows the source article's buttons and never acts without a click.
  The Standard model (4B) is recommended on PCs with 12 GB RAM or more.
* Downloading the brain needs internet once (GitHub release of llama.cpp + Hugging Face);
  after that nothing is fetched. Companies can point Rover at their own mirror or import files.
* Provider APIs were tested against local fakes built from their documentation, not live
  accounts. Creating servers is supported for Hetzner, DigitalOcean, Vultr, Linode and
  Lightsail; for Oracle, Contabo, Hostinger, Scaleway and UpCloud Rover lists servers and
  you order new ones on their website.
* DigitalOcean browser sign-in uses DigitalOcean's dynamic client registration for public
  apps; if it is unavailable, the token form still works.

## Known limitations (v0.6)

* Browsers don't reveal the path of a dropped folder, so **Redeploy last folder** knows a folder
  only after you deployed it by path (the path box, Ctrl+V, recent folders, a watched folder or
  the Explorer menu). Dropped folders still deploy normally.
* Notifications use the window's notification permission and a taskbar flash; there is no
  tray icon or Windows Action Center toast yet.
* The self-updater was tested against a fake GitHub API, not a real release, and the silent
  installer and Explorer menu were not run on a real Windows PC in this build environment.
* Password protection and the primary-domain redirect are written into the site's nginx
  config on the next deploy (or *Apply now*). Rolling back to a release made before you
  changed them restores that release's config until you apply again.
* The executables are unsigned, so Windows SmartScreen may warn on first run.

## Known limitations (v0.7)

* Cloudflare Tunnel creation, DNS switching and Coolify on a second server were tested against
  mock APIs and local scripts, not a live Cloudflare account or a real Raspberry Pi. The
  lightweight nginx configs were validated with a real nginx 1.26 (`nginx -t`, two sites side by side).
* *Primary + backup* switching needs Rover running (on this PC, or started with Windows).
  *Together* mode keeps working without Rover but cannot see a server whose web server is
  broken while its tunnel is up.
* This PC can be a backup or extra server, not a project's main server, and is skipped in
  *Together* mode (it does not answer HTTPS on port 443).
* Automatic switching needs the domain's DNS on Cloudflare (free plan). Other registrars get
  guided steps. DNS round robin is not used (browsers cache dead addresses).
* Fast (changed-files-only) deploys are turned off for projects on several servers.
* The Windows network monitor (adapter type, Wi-Fi name, USB tethering detection), keep-awake,
  autostart and power status were compiled and vetted for Windows but not run on a real PC here.

## Known limitations (v0.8)

* With the Light brain alone (Qwen3 1.7B, CPU), 12 of 24 eval tasks get exactly the right plan;
  the shipped planner (built-in templates first, brain second) gets 24/24. Requests no template
  covers depend on the model and may need rewording. Every plan is shown and confirmed first.
* Walkthroughs follow Rover's own screens only; they cannot point at buttons on other websites
  (registrars, Oracle Cloud), they give written steps there instead.
* GPU offload was tested with mocked hardware lists, not on a real graphics card here. The offline
  installer was assembled by CI scripts but not run on a real Windows PC here.
* The folder move cannot run while another program holds files in `Documents\Dilreet`; Rover then
  keeps using the old folder and tries again on the next start.
