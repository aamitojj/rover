> Rover was called **Dilreet** (and before that HostPanel) up to version 0.7.

# Rover 0.4 — research notes

Goal for 0.4: easier to use, more automatic, better organised for people with many
sites, with a Netlify-like interface and plain language.

**Rule we followed:** we took ideas only. No code was copied from any project
below, so Rover's own license is unchanged (see [CREDITS.md](CREDITS.md)).

| Idea in 0.4 | Where it came from |
|---|---|
| Dark left sidebar, "Projects" list, per-project sub-menu (Overview / Deploys / Files / Domains / Logs / Settings) | [Netlify app](https://app.netlify.com) and its [docs on the site overview](https://docs.netlify.com/) |
| Dashboard with counters, "Recent deploys", "Getting started" checklist that can be hidden | Netlify team overview; [Vercel dashboard](https://vercel.com/docs/dashboard-features); [Cloudflare Pages](https://developers.cloudflare.com/pages/) onboarding |
| Start from a template, with live thumbnails | [Netlify templates](https://www.netlify.com/integrations/templates/), [Vercel templates](https://vercel.com/templates) |
| Groups (one per client/business) shown in the sidebar | Coolify "Projects → Environments" ([coolify.io](https://coolify.io), [GitHub](https://github.com/coollabsio/coolify)); [Dokploy](https://github.com/Dokploy/dokploy) projects; Netlify teams |
| Tags with colours, pins/favourites, search, filters, sort, grid/list toggle | [Portainer](https://github.com/portainer/portainer) tables; [Uptime Kuma](https://github.com/louislam/uptime-kuma) tags; GitHub/Vercel "starred" projects |
| Select many → bulk actions (redeploy, move, tag, archive) | Portainer bulk container actions; Gmail-style selection bar |
| Archive instead of delete | GitHub repository archiving; Netlify "hide" of old sites |
| Health check every few minutes + padlock (SSL) expiry warning + notification | [Uptime Kuma](https://github.com/louislam/uptime-kuma) (monitors, certificate expiry notification) |
| "Needs your attention" list with a **Fix it** button | Vercel/Netlify failed-deploy banners; [CapRover](https://github.com/caprover/caprover) one-click fixes; Coolify notifications |
| Visitors & logs from the web server's access log (no tracking script) | [GoAccess](https://github.com/allinurl/goaccess) (log-based analytics); Netlify Analytics (server-side, no cookies) |
| Scheduled local backups with "keep the last N" | Coolify scheduled backups; [Dokku](https://github.com/dokku/dokku) backup plugins |
| Duplicate a project | Netlify "duplicate site" / Vercel "clone" |
| Plain words + tooltips for jargon (DNS, SSL, VPS) | [GOV.UK content design guide](https://www.gov.uk/guidance/content-design/writing-for-gov-uk), [plainlanguage.gov](https://www.plainlanguage.gov/guidelines/) |
| Light/dark theme following the system | Netlify, GitHub, Coolify |

## What we did not copy
* Netlify Forms, functions and env vars: not useful for static nginx sites.
* Real-time multi-user features (teams, roles): Rover is single-user on one PC.
* Visitor counts are approximate: the proxy hides some real IPs.

## Safety rule kept from 0.3
Anything that changes the server or DNS (redeploy, restart, domain, DNS records,
bulk redeploy, Fix-it actions that touch the server) always asks for a confirm click.
Backups, health checks and log reading are read-only.

---

# Rover 0.5 — research notes

Goal for 0.5: make Rover easier still, and build **Jiv** into a helper that can answer
every question about the app and do the work for you, **offline**. Then let people connect
their hosting accounts instead of copying IP addresses around. Same rule as before: ideas
only, no copied code, every state-changing action needs a click.

## How people get stuck (and what 0.5 does about it)

| Where people get stuck | 0.5 answer | Source / inspiration |
|---|---|---|
| "Where is that setting?" | **Ctrl+K command palette** for pages, projects, servers and help articles; `?` lists shortcuts; `G` then a letter jumps | [VS Code command palette](https://code.visualstudio.com/docs/getstarted/userinterface), [GitHub command palette](https://docs.github.com/en/get-started/accessibility/github-command-palette) |
| Too many "Are you sure?" boxes for small things | **Undo** for archive, pin and bulk changes. Confirm boxes remain for anything that touches the server, DNS or money | [NN/g: Confirmation dialogs](https://www.nngroup.com/articles/confirmation-dialog/) (use undo for reversible actions, confirm for consequential ones) |
| Not knowing what a page is for | **? Explain this page** on every page; suggested questions for the current screen | Contextual help patterns from Netlify/Vercel docs links |
| Errors in technical words | **Ask Jiv why** on every red error box and failed job; Jiv explains about 20 common error patterns in plain words and links a fix | [GOV.UK content design](https://www.gov.uk/guidance/content-design/writing-for-gov-uk) |
| "Why is my site down?" | Jiv runs read-only checks (server reachable, container running, DNS points here, HTTPS, health) and lists findings with fix buttons | Uptime Kuma, Coolify status (ideas) |
| Copying IPs and tokens between websites | **Hosting accounts portal** with each provider's official connection method | Provider API docs below |

## Jiv offline: how it works

* **Built-in knowledge (always on, no download):** 60 articles (English with Hindi
  versions) covering every screen, domains/registrars, servers, errors and privacy. Search is
  Okapi **BM25** ([Robertson & Zaragoza](https://www.staff.city.ac.uk/~sbrp622/papers/foundations_bm25_review.pdf))
  with field weights (title > questions/tags > body), simple stemming, synonyms, Hinglish and
  Devanagari words, typo tolerance (edit distance 1–2) and a small boost for the current screen.
  Test set: 41 real questions answered with the right article first (100 %).
* **Intent detection:** rules turn requests into tool calls (*"roll back bakery"* →
  `rollback_site`), questions into answers, and "why/not working" into a diagnosis.
* **Optional offline brain:** [llama.cpp `llama-server`](https://github.com/ggml-org/llama.cpp/tree/master/tools/server)
  ([release b12360](https://github.com/ggml-org/llama.cpp/releases/tag/b12360), MIT) runs a GGUF model
  as a sidecar on `127.0.0.1`, random port, random API key, OpenAI-compatible API, tool calling via `--jinja`.
  Jiv sends it the top 3 help articles plus a redacted summary of the app state (RAG), so answers stay grounded.

### Model choice

| | Standard | Light |
|---|---|---|
| Model | [Qwen3-4B-Instruct-2507](https://huggingface.co/Qwen/Qwen3-4B-Instruct-2507) Q4_K_M ([GGUF](https://huggingface.co/unsloth/Qwen3-4B-Instruct-2507-GGUF)) | [Qwen3-1.7B](https://huggingface.co/Qwen/Qwen3-1.7B) Q4_K_M |
| Download | 2.5 GB (+25 MB engine) | 1.1 GB (+25 MB engine) |
| Memory while chatting | ~3.5 GB | ~2 GB |
| Picked automatically when | PC has ≥ 11 GB RAM and ≥ 4 CPU threads | otherwise |
| License | Apache-2.0 | Apache-2.0 |

Why Qwen3: Apache-2.0 (free for any use, no sign-up gate), strong tool calling at small sizes,
100+ languages including Hindi, and good CPU speed in Q4. The 2507 Instruct build has no
"thinking" phase, so answers start quickly. Files are fetched from fixed URLs and checked
against fixed sha256 values; downloads resume; you can also import files from a USB stick or set a company mirror.

## Hosting providers: which APIs are usable

| Provider | Official connection method | List | Create | Docs |
|---|---|---|---|---|
| DigitalOcean | **OAuth 2.0 browser sign-in** (public client, PKCE S256, loopback redirect, dynamic client registration) or API token | ✔ | ✔ | [OAuth API](https://docs.digitalocean.com/reference/api/oauth/), [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252), [RFC 7636](https://www.rfc-editor.org/rfc/rfc7636) |
| Hetzner Cloud | API token (per project) | ✔ | ✔ | [docs.hetzner.cloud](https://docs.hetzner.cloud/) |
| Vultr | API key | ✔ | ✔ | [vultr.com/api](https://www.vultr.com/api/) |
| Linode / Akamai | Personal access token | ✔ | ✔ | [Linode API v4](https://techdocs.akamai.com/linode-api/reference/api) |
| AWS Lightsail | Access keys, SigV4-signed | ✔ | ✔ | [Lightsail API](https://docs.aws.amazon.com/lightsail/2016-11-28/api-reference/Welcome.html), [SigV4](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv.html) |
| Oracle Cloud | API signing key (RSA), signed requests | ✔ | order in console | [OCI request signing](https://docs.oracle.com/en-us/iaas/Content/API/Concepts/signingrequests.htm) |
| Contabo | OAuth2 client credentials + API user | ✔ | order on website | [api.contabo.com](https://api.contabo.com/), [credentials page](https://my.contabo.com/api/details) |
| Hostinger VPS | API token | ✔ | order on website | [developers.hostinger.com](https://developers.hostinger.com/) |
| Scaleway | Secret key | ✔ | order on website | [Instance API](https://www.scaleway.com/en/developers/api/instance/) |
| UpCloud | API token | ✔ | order on website | [UpCloud API 1.3](https://developers.upcloud.com/1.3/) |
| Any other VPS | IP + SSH (password once, then Rover's key) | — | — | — |

Of these, DigitalOcean is the one with a documented browser sign-in for desktop apps that needs
no client secret, so it is the OAuth provider; the rest use their documented tokens or keys.
**Rover never stores or types website passwords and never scrapes login pages** — providers
do not support that and two-step login would break it. Tokens are encrypted with Windows DPAPI
([CryptProtectData](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata))
and are never put into any AI prompt (Jiv only sees account nicknames and server names).
Creating servers was limited to providers whose API can create a server with an SSH key and
report its price; for the others, buying a server stays on their website (Rover links to it).
The minimum size offered matches [Coolify's requirements](https://coolify.io/docs/get-started/installation) (2 CPU, 2 GB RAM).

# Rover 0.6 — research notes

Goal: when you open a project, everything about it is in one place, deploying a new build is
a single drop, and Rover keeps itself up to date. We looked at how Netlify, Vercel,
Cloudflare Pages, Coolify and Dokploy organise a project and what makes them quick.

## What they do

| Product | Project page | Deploying a new build | Undo | Other ideas |
|---|---|---|---|---|
| Netlify | *Project configuration* split into General, Build & deploy, Domain management, Notifications, Access & security | Drag a folder onto the Deploys page (Netlify Drop) | *Publish deploy* on any older deploy | *Lock to stop auto publishing*: new deploys wait until you publish them; deploy previews; password protection (paid) |
| Vercel | Overview, Deployments, Settings (Domains, Git, Environment, Deployment Protection) | CLI or Git | *Instant Rollback*, *Promote* a preview to production | Password protection is a Pro feature; checklist on new projects |
| Cloudflare Pages | Deployments, Custom domains, Settings | *Direct Upload*: drag a folder or zip, choose *Production* or *Preview* | Rollback from the Deployments list | Only changed files are uploaded (asset hashing) |
| Coolify | Resource page with tabs: General, Advanced, Environment, Domains, Logs, Rollback, Danger zone | Git or Docker image | Image-based rollback | Per-channel notifications (deploy succeeded/failed, status changes), HTTP basic auth |
| Dokploy | Application tabs: General, Environment, Domains, Deployments, Logs, Monitoring, Advanced | Git, Docker, or drop a zip | Registry rollback; auto rollback on failed health check | Notifications for deploys, build errors, backups and resource limits |

## What 0.6 takes from them

* **One page per project with a sub-menu** (all five): Overview, Deploys, Domains, Hosting,
  Configuration (General, Build & deploy, Redirects & headers, Access, Notifications, Backups,
  Danger zone), Files, Testing, Visitors & logs. Configuration has its own second-level menu,
  as in Netlify.
* **A drop area right on the project** (Netlify Drop, Cloudflare Direct Upload, Dokploy zip):
  drop a folder or zip on the Overview or Deploys page, or anywhere on the project page. It
  deploys at once and shows progress, the live address and **Undo**.
* **Draft vs live** (Cloudflare preview / production, Vercel promote): *Save as draft*
  (or hold Shift while dropping). Drafts can be previewed and published later.
* **Lock publishing** (Netlify): automatic deploys from a watched folder or Git become drafts.
* **One-click rollback** (all): *Undo* right after a deploy, *Roll back* in history, key `R`
  redeploys and the Deploys list publishes any older deploy.
* **Only upload what changed** (Cloudflare): Rover keeps a list of file fingerprints per
  project; the server copies the live version and only changed files are sent. Full upload
  when the live version is not the one Rover expects, or when more than 70% changed.
* **Setup checklist with buttons** (Vercel, Netlify onboarding): deploy, test, add a domain,
  HTTPS, primary domain, backups, optional Git.
* **Password protection** (Netlify / Vercel paid, Coolify basic auth): HTTP basic auth in the
  site's nginx. Free here because it is your server.
* **Primary domain** (Netlify): choose www or not; the other redirects with 301.
* **Per-project notifications** (Coolify, Dokploy): site down and deploy failed, on or off per
  project, plus a desktop notification and taskbar flash when a deploy finishes.
* **Redirects & headers editor** with examples and a *Check* button that uses the same parser
  as the deploy, so mistakes are found before publishing.

## What we added for speed

*Redeploy last folder*, paste a folder path (Ctrl+V) anywhere on a project, recent folders,
keyboard keys 1–8 / D / R / O, a *Deploy with Rover* Explorer right-click menu (asks which
project), and the command palette knows every new page.

## Self-update: how other desktop apps do it

* Most Windows apps that publish on GitHub (e.g. those using Squirrel, electron-updater,
  Velopack) read the GitHub Releases API, download an asset and verify a checksum or
  signature before installing. We follow the same pattern without adding a framework.
* `GET /repos/{owner}/{repo}/releases/latest` returns the newest non-draft, non-pre-release;
  `/releases` lists pre-releases too (used for the Beta channel). No token is needed; the
  unauthenticated limit is 60 requests per hour per IP, plenty for a daily check.
* Since June 2025 GitHub shows a `digest` (`sha256:…`) for every release asset. Rover uses
  it as a second check next to `SHA256SUMS.txt`, never instead of it.
* Inno Setup installers update in place with `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART
  /CLOSEAPPLICATIONS`; a `[Run]` entry with `skipifnotsilent` restarts the app afterwards.
* Publishing: `softprops/action-gh-release` creates the tag and release from a workflow,
  which lets us offer a one-button *Release* workflow (`workflow_dispatch` with a version box).
* Optional ed25519 signature over `SHA256SUMS.txt` (same idea as minisign / Tauri updater keys)
  for people who want protection even against a compromised repository.

## What we did not copy

* Serverless functions, forms, edge middleware and build minutes: Rover stays a static host
  on your own server.
* Automatic rollback on failed health checks (Dokploy): static sites rarely fail that way,
  and silent rollbacks are confusing. Rover tells you instead and offers *Undo*.
* Background "silent" updates without any choice: the default is *Ask me first*.

# Rover 0.7 — research notes

Goal: one project on two or more servers, staying up by itself when one goes down, and
servers that people build at home (old PCs, Raspberry Pi, this PC). Free by default.

## Ways to fail over a static site

| Option | Cost | Who checks health | Switch time | Works while this PC is off | Notes |
|---|---|---|---|---|---|
| **Cloudflare Load Balancing** | $5/month for 2 origins, +$0.50 per extra 500k DNS queries | Cloudflare, from many locations | ~1 check interval + TTL (proxied: seconds) | yes | The "real" product. Documented as an option, not the default (paid). |
| **Cloudflare Tunnel replicas** ("Together") | free | Cloudflare sees which connectors are connected | seconds | yes | Run the same tunnel on several hosts; each replica opens 4 connections; a visitor goes to the closest replica and Cloudflare retries another if a connection fails. No traffic steering, and if the web server dies while `cloudflared` stays up, visitors get errors. |
| **Cloudflare DNS + health checks by Rover** ("Primary + backup") | free | Rover, every 60 s, 3 strikes | 3 min + seconds (proxied record, no client TTL) | no | Rover patches the A/CNAME through the API. Proxied (orange cloud) means visitors are not stuck on cached DNS. |
| **DNS round robin** (two A records) | free | nobody | never (clients cache) | – | Browsers and resolvers keep using a dead address for minutes; Rover would have to remove dead IPs and still wait for TTL. Rejected. |
| **Registrar DNS (GoDaddy etc.)** | free | Rover | TTL ≥ 600 s plus resolver caching | no | GoDaddy's API can change A records (Rover already uses it for setup) but there are no health checks and the minimum TTL is long. Offered as guided manual steps. |

**Chosen default: Primary + backup** with Rover's own health checks and Cloudflare's free DNS API,
because it works with any server type (rented, home behind a tunnel, This PC), costs nothing and
the switch is visible and reversible. **Together** is offered for people who want it to keep
working while their PC is off. Both need the domain's DNS on Cloudflare (free); without it
Rover still checks and tells you what to change.

Details that matter:
* Health check = HTTP request to the server's IP with the site's Host header (2xx/3xx/401 = up),
  falling back to `curl` over SSH for machines without a direct way in. 3 failures mark a server
  down, 3 successes mark it up again (no flapping). Rover never fails over when *this PC* is
  offline (it first checks that it can reach the internet itself).
* The switched record is **proxied**: the change is live in seconds, Cloudflare serves HTTPS,
  and Let's Encrypt on the backup is not in the critical path. Cloudflare SSL mode should be
  *Full* (the origin certificate is not verified by Cloudflare in that mode).
* Deploys to several servers use a **two-phase** upload: send to every server, then switch every
  server that received the files, all with the same release name. "Live" only when all switched.

## Home servers

* Most home connections (and every mobile connection) are behind CGNAT and change IP. A
  Cloudflare Tunnel needs only an outgoing connection, so it works everywhere, including over a
  phone's mobile data. Port forwarding + dynamic DNS (DuckDNS, No-IP, router DDNS) is the
  advanced path; it fails behind CGNAT (WAN address 100.64.0.0/10).
* Tunnel API (remotely managed): `POST /accounts/{acc}/cfd_tunnel {"name","config_src":"cloudflare"}`
  returns the id and token; `PUT .../configurations` sets the ingress rules; DNS is a proxied CNAME
  to `<id>.cfargotunnel.com`. Token permissions: *Account › Cloudflare Tunnel › Edit* and
  *Zone › DNS › Edit*. `cloudflared tunnel --no-autoupdate run` reads `TUNNEL_TOKEN` from a
  root-only environment file, so the token never appears in `ps`.
* `cloudflared` is pinned to **2026.11.2** with sha256 fingerprints for windows-amd64,
  linux-amd64, linux-arm64, linux-arm and linux-armhf, downloaded from Cloudflare's GitHub release
  only when needed.
* Coolify needs ~2 GB of memory and a 64-bit OS. For a Raspberry Pi 3/4 with 1 GB, or 32-bit
  Raspberry Pi OS, Rover uses a **lightweight** mode: Debian's nginx with one include per site,
  the same generated rules (with per-site variable names so several sites can share one nginx),
  HTTPS on 443 with the machine's snakeoil certificate for the tunnel, `nginx -t` before every
  switch and an automatic rollback if nginx refuses a config.
* This PC: Rover's own server (the App Testing rule engine), `cloudflared.exe` as a child
  process with automatic restart, `SetThreadExecutionState(ES_CONTINUOUS|ES_SYSTEM_REQUIRED)` to
  keep the PC awake, the per-user Run key to start with Windows, `GetSystemPowerStatus` for
  battery, and `GetAdaptersAddresses` to tell Wi-Fi (IfType 71), mobile broadband (243/244) and
  USB tethering ("Remote NDIS" adapters) apart.

## Trade-offs we tell people plainly

The machine must stay on and awake; upload speed is the site's speed; power and internet cuts take
it down (a UPS helps); best used as a backup next to a rented server or for small sites.

Sources: Cloudflare docs (Load Balancing pricing and health monitors; Tunnel availability and
replicas; Tunnel API; proxied records and TTL), cloudflared release notes 2026.11.2, Coolify
installation requirements, Raspberry Pi OS documentation, Microsoft docs for
SetThreadExecutionState, GetSystemPowerStatus and GetAdaptersAddresses.

# Rover 0.8 — research notes

Goal: Jiv should be able to *show* people where things are and *do* multi-step jobs for them,
fully offline, and the app should look like a modern tool.

## Guiding people through an app

* Product tours (Intro.js, Shepherd, Driver.js, Appcues, WalkMe) all use the same pattern: dim
  the page, cut out the target element, a short bubble, Next/Back. What helps beginners most is
  *waiting for the real click* and *checking the result*, not a slideshow. Rover's guides do that
  (`done` = click, view or a state check such as `dns_points`) and skip steps already done.
* Guides are data (`jivguides/*.json`) with stable anchors (`data-anchor` / selectors), and a test
  checks every anchor exists in the UI, so a UI change cannot silently break a guide.
* A model may only use anchors from a list; made-up anchors are dropped.

## Letting an assistant act

* Agent frameworks (OpenAI/Anthropic tool use, LangGraph, Open Interpreter) show that small
  models do best with few, well-named tools, a JSON grammar, and a plan shown before running.
  Rover uses 24 tools, a llama.cpp `json_schema` grammar where tool names, project slugs and
  server ids are enums, and built-in templates for common jobs.
* Safety: read-only checks first, one confirmation for the plan and one for each risky step,
  stop on the first failure with a plain explanation. Passwords and tokens never enter the prompt.

## Offline packs and GPU

* GitHub release assets are limited to 2 GiB, FAT32 USB sticks to 4 GiB: packs are split into
  `.dlpack.001/.002…` parts and the offline installer uses Inno Setup disk spanning.
* llama.cpp's Vulkan build runs on NVIDIA, AMD and Intel cards with one binary; `--fit` places as
  many layers on the card as fit. CUDA builds are faster on NVIDIA but ~500 MB larger.

## Rename and look

* Renaming desktop apps: keep the installer AppId so the upgrade is in place, move data folders
  with a fallback, keep reading old settings and environment names, and publish old asset names
  for one or two releases so older updaters still find the new version.
* Icons: Lucide (ISC) and Tabler (MIT) are both permissive; Lucide was chosen for its consistent
  1.5–2 px stroke. Brand marks come from Simple Icons (CC0) and are used only to name the service.
* Font: Inter (SIL OFL 1.1), bundled as two variable woff2 files (~120 KB), no CDN.
