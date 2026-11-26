# Rover automation: what runs by itself, what you still click

Rule for everything below: Rover only calls documented APIs, never sends
data anywhere you did not choose, and every state-changing action is shown as a
confirm card or button you click first. Background work only *reads* (status
polling, DNS checks, Git "is there a new commit?" checks) and applies changes you
already approved (for example "auto-deploy this repo").

## Server and Coolify

| Step | How | Manual? |
|---|---|---|
| Create a VPS | Hetzner Cloud `POST /v1/servers`, DigitalOcean `POST /v2/droplets` with your API token (DPAPI-encrypted on this PC) | You paste a token, pick a size, click **Create** |
| Install Coolify | SSH + the official `install.sh`, live log | One click |
| Coolify admin + API token | Created over SSH with `php artisan tinker` on the server (no browser step); token named `Rover`; the admin login is kept DPAPI-encrypted on this PC | None |
| Instant address | `https://<project>.<server-ip>.sslip.io` | None |
| Open ports on Oracle Cloud | Oracle's Ubuntu images reject incoming traffic in the server's own `iptables`, even when the cloud firewall allows it. The setup script inserts ACCEPT rules for 80 and 443 before the REJECT rule and saves them (`netfilter-persistent` or `/etc/iptables/rules.v4`) | None (you still open 80/443 in Oracle's Security List; Jiv's guide shows how) |
| Health checks | Server reachability and project status every 10 minutes while Rover runs | None (read-only) |

Server paths stay `/data/hostpanel/sites/<id>/` so servers set up by older
versions keep working; the name is internal and never shown as the app name.

## Projects on this PC

`Documents\Rover\Projects\<name>\` (location changeable in Settings → Storage):

```
current\            latest deployed files (edit and "Deploy from this folder")
deploys\<time>_<id>.zip  every deploy, kept per the retention setting
deploys\index.json  deploy list: id, time, source, file count, size, checks, commit
```

Existing sites from earlier versions are migrated on first start (folders are created,
nothing on the server changes). Retention: all, or last 5/10/20/50 (the live deploy
is always kept).

## Domains

Rover looks up the domain's nameservers over public DNS and recognises the
DNS host / registrar from them (e.g. `domaincontrol.com` = GoDaddy), then offers, in this order:

1. **Guided (default, any registrar).** Numbered steps for that registrar, a button
   that opens its DNS page, copy buttons for every value, warnings about
   conflicting A/AAAA/CNAME records and parking pages, and a live check every 15 s
   that notifies you and connects the domain when DNS is right.
2. **Registrar API (GoDaddy, Porkbun).** You paste a key once; Rover shows the
   records it will create and asks before it writes them.
3. **Fully automatic (Cloudflare).** Rover creates the zone in your Cloudflare
   account (`POST /client/v4/zones`), copies the existing records it found,
   adds the site records, and shows the two Cloudflare nameservers. With a GoDaddy
   personal access token it can also switch the nameservers at GoDaddy for you
   (separate confirm). Otherwise you paste the nameservers at your registrar once.

### GoDaddy

* **Who can use the API:** since April 2026, GoDaddy production DNS and
  Domains API keys work for any account with **at least one domain**. Before
  that, production access needed 10+ domains (or a Discount Domain Club / Pro
  plan), which is why many tools still say "GoDaddy API doesn't work for small
  accounts". The Availability API still needs 50+ domains. Sources:
  <https://www.godaddy.com/resources/news/godaddy-dns-api-now-works-with-a-single-domain>,
  <https://community.letsencrypt.org/t/godaddy-api-access-policy-update/246372>.
* **Auth:** v1 `Authorization: sso-key KEY:SECRET` (keys from
  <https://developer.godaddy.com/keys>, choose **Production**, not OTE), or a v3
  personal access token `Authorization: Bearer …` with `domains.dns:update`
  (and `domains.nameservers:update` for the nameserver switch). Source:
  <https://developer.godaddy.com/en/docs/api-users/auth>.
* **Endpoints used:** v1 `GET/PUT /v1/domains/{domain}/records/{type}/{name}`;
  v3 DNS records and nameservers endpoints as documented on developer.godaddy.com.
* The wizard shows this restriction plainly and falls back to Guided if the key
  is rejected (401/403 with the access message).

### Porkbun

API v3 (`https://api.porkbun.com/api/json/v3/`): `ping`, `dns/retrieve/{domain}`,
`dns/editByNameType/{domain}/{type}/{sub}` (upsert), `dns/create/{domain}`.
API access must be enabled **per domain** in the Porkbun dashboard. Source:
<https://porkbun.com/api/json/v3/documentation>.

### Namecheap

Guided only. Its API requires a whitelisted public IPv4 address and account
eligibility (balance / number of domains), which does not fit a desktop app on a
changing home IP. Source: <https://www.namecheap.com/support/api/intro/>.

### Hostinger, Squarespace, others

Guided steps with a direct link to the DNS page.

### Domain Connect (documented, not implemented)

[Domain Connect](https://www.domainconnect.org/) lets a service apply a DNS
"template" at the user's DNS provider after one click. It needs the template to be
published and onboarded with every DNS provider (GoDaddy, IONOS, Cloudflare, …)
and a public service identity, so it is not usable by a self-hosted desktop app
today. If Rover ever gets a public template, the guided mode is where it would
appear.

## Git and Netlify

* GitHub / GitLab / Bitbucket / Azure DevOps / Gitea / Codeberg / Forgejo, plus
  plain `.git` links and `.zip`/`.tar.gz` URLs: archive download through the
  provider's documented API; optional read-only token (encrypted). Auto-deploy is
  available for the named providers (not for plain links or archive URLs). Auto-deploy polls the branch head (every 5 minutes by default,
  configurable per project), with limits on download size, file count and build time.
* Netlify: site list, files of the published deploy, domain and `_redirects`,
  with a personal access token.

## Jiv (assistant)

Local AI apps are found by probing well-known loopback ports only (Ollama 11434,
LM Studio 1234, Jan 1337, GPT4All 4891, llama.cpp 8080, LocalAI, vLLM 8000,
text-generation-webui 5000). Nothing leaves the PC unless you connect a cloud
provider and accept its one-time consent; passwords, tokens and keys are never
sent, and the "Include my status" switch can be turned off.

## What still needs you

* Renting the server (or pasting a cloud API token).
* Buying a domain; for Guided mode, adding 1–2 records (or nameservers) at the registrar.
* Approving each change (confirm cards).
* The first Coolify admin password is shown once for you to save.

## New in 0.4

* **Health checks** — with background checks on, every site is opened every 10
  minutes; you get a notification if it stops answering or its padlock (SSL)
  certificate ends within the days set in *Settings → Backups & alerts* (default 14).
* **Scheduled backups** — daily or weekly zip of every project's live files (and
  optionally all saved deploys) to a folder you choose, keeping the last N. No
  passwords or keys are ever written to a backup.
* **Fix it** — the dashboard lists problems (failed deploy, stopped site, server
  not connected, domain not working) with one button each. Buttons that change the
  server (restart, redeploy) ask you to confirm first.
* **Bulk redeploy** asks for a confirm and runs as one job with a log.

## 0.5: Hosting accounts

| Provider | Connect with | List servers | Create server | Notes |
|---|---|---|---|---|
| DigitalOcean | Browser sign-in (OAuth 2.0 code + PKCE, loopback redirect) or API token | Yes | Yes | Token refreshed automatically |
| Hetzner Cloud | API token (Read & Write), one per project | Yes | Yes | |
| Vultr | API key (allow this PC's IP if access control is on) | Yes | Yes | |
| Linode / Akamai | Personal access token (Linodes read/write) | Yes | Yes | |
| AWS Lightsail | Access key ID + secret (IAM user with Lightsail access), region | Yes | Yes | Requests signed with AWS SigV4 |
| Oracle Cloud (OCI) | API signing key (tenancy, user, fingerprint, private key, region) | Yes | No (order in the console) | Requests signed per OCI spec |
| Contabo | Client ID/secret + API user/password (from the API page, not the login) | Yes | No | |
| Hostinger VPS | API token | Yes | No | |
| Scaleway | Secret key | Yes | No | |
| UpCloud | API token | Yes | No | |
| Any other VPS | IP + root password once (then Rover's own SSH key) | — | — | |

"Use this server for Rover" saves a server profile, connects with Rover's key (asks for
the password once if the key is not there yet) and runs the usual automatic setup.
"Create server" shows sizes and the monthly price, asks for a confirm click, then creates the
server with Rover's SSH key, waits for its IP and sets it up. Removing an account deletes the
saved token from this PC only; servers keep running.

## 0.5: Jiv actions

| Action | Runs | Confirm click? |
|---|---|---|
| Open a page/project, explain this page, search help | In the app | No |
| Check a site / domain / all sites, explain logs, health check | Read-only on the server/DNS | No |
| Redeploy, restart, roll back, pull from Git and deploy, deploy folder, set up DNS, auto-deploy on/off, backup now, switch server, create a project | On the server or in settings | **Yes** (server-issued id, runs once, expires after 1 h) |
| Install the offline brain, add a hosting account, install Ollama | Opens the right screen | You finish there |

## 0.6: Project hub, quick deploys and updates

Runs by itself:

* **Deploy on drop.** Dropping a folder or .zip on a project deploys it straight away (no
  confirm, like Netlify Drop; it is undoable with one click). Paste-a-path and the Explorer
  menu ask first, because the folder could be the wrong one.
* **Only changed files** are uploaded; the full upload is used automatically when needed.
* **Locked projects:** automatic deploys (watched folder, Git) are saved as drafts.
* **Update checks** at start-up and daily, if *Check automatically* is on (default). With
  *Install by itself* chosen, verified updates install when no job is running.
* **Desktop notification and taskbar flash** when a deploy finishes while Rover is in the
  background (if you allowed notifications).

Needs your click (confirm):

* Publish a draft, roll back, delete a deploy, *Apply now*, saving password protection,
  primary domain or redirects that change the live site, moving a project to another server,
  installing an update (unless you chose *Install by itself*), and everything in the Danger zone.

## 0.7: Several servers, home servers, This PC

| Task | Automatic? | Needs |
|---|---|---|
| Set a project up on another rented server (Coolify) | yes | that server connected once with Rover's key |
| Deploy to every server, same version, atomic switch on each | yes | – |
| Retry a server that missed a deploy, sync check, rollback everywhere | yes (one click) | the deploy saved on this PC |
| Health checks every minute, notifications | yes | Rover running |
| Switch the domain to a backup and back (Primary + backup) | yes | domain DNS on Cloudflare (free) + token with DNS: Edit |
| Together mode (one tunnel, connector on every server) | yes | Cloudflare token with Cloudflare Tunnel: Edit + DNS: Edit |
| Switching without Cloudflare | guided | change the A record at your registrar |
| Home Linux server: login, key, engine choice, nginx or Coolify install | yes | SSH enabled on the machine, password once |
| Home server reachable from the internet | yes (tunnel) / guided (port forwarding + DDNS) | Cloudflare, or router access |
| This PC: web server, tunnel, start with Windows, keep awake, network/power monitor | yes | Cloudflare for internet access |
| Cloudflare Load Balancing (paid) | not used | – |

## 0.8: Jiv "Do it for me" and "Show me how"

Jiv can now run a whole job as a **plan**: a list of named steps from 24 tools. The plan is shown
first; you confirm it once, and each risky step (marked) asks again. Read-only checks run first; the
first failure stops the plan with a plain explanation and what to try.

| Tool | What it runs | Risky? |
|---|---|---|
| `create_project`, `deploy_folder`, `retry_deploy`, `rollback`, `restart_site` | the same code as the buttons in the project | no |
| `host_on_this_pc` | turns on the built-in web server on this PC | no |
| `add_backup_server`, `move_project` | adds a server to a project / moves it (deploys there, then switches) | move: yes |
| `set_domain`, `dns_records`, `wait_dns`, `enable_https`, `renew_ssl` | domain wizard steps (Cloudflare/registrar API or guided steps); `wait_dns` polls up to 45 minutes | renew: yes |
| `diagnose`, `check_dns`, `explain_logs`, `why_deploy_failed`, `disk_usage`, `check_services` | read-only checks (SSH `df`, `du`, `systemctl`/`docker ps`, logs) | no |
| `fix_web_config` | rebuilds the site's web server settings (redirects, headers, password) from Rover's copy, the same as *Apply now*, then checks the site answers | yes |
| `free_disk` | removes unused Docker images and build cache older than 3 days, shrinks system logs to 200 MB, `apt-get clean`, old compressed logs; never touches running containers, volumes or site files; always preceded by `disk_usage` | yes |
| `restart_service` | restarts nginx, Coolify's proxy, Docker, Coolify or the tunnel | yes |
| `backup_now` | backs up all projects to the backup folder | no |

Plans come from built-in templates for common requests (domains, backups, failover, moves, disk,
stuck services, deploy errors, HTTPS); otherwise the offline brain writes one under a grammar that
only allows these tools and your real project and server names. Passwords, tokens and keys are never
put in the prompt. **Show me how** guides only point at Rover's own buttons and never click for you.
