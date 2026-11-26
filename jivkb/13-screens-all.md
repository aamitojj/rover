=== setup-wizard
title: The server set-up screen
q: set up my server | setup wizard | use this server for rover | install coolify automatically | server setup stuck
tags: setup wizard server install coolify automatic lite engine prepare
hi: server setup kaise kare | सर्वर सेटअप
go: Set up server -> #/setup
screen: setup
---
**Set up** prepares a fresh Ubuntu/Debian server in 5-10 minutes: updates, firewall (22/80/443, including Oracle's iptables), swap, Docker and the hosting engine (Coolify), or Rover's **Lite** engine (nginx only) for small or home servers. You type the password once; Rover installs its own key. Progress is shown step by step, and it's safe to run again if something stopped halfway.
@hi
**Set up** नए Ubuntu सर्वर को 5-10 मिनट में तैयार करता है: अपडेट, फ़ायरवॉल, swap, Docker और Coolify (या छोटे सर्वर के लिए Lite इंजन)। पासवर्ड एक बार डालें। बीच में रुके तो फिर से चला सकते हैं।

=== screen-overview
title: Project Overview tab
q: what is on the overview tab | project overview | restart button overview | quick deploy area
tags: overview project tab status restart rollback drop quick
hi: overview tab
go: Projects -> #/projects
screen: project
---
The **Overview** shows the project's live address, status (online, last check), the live deploy, quick buttons (**Restart**, **Roll back**, open site) and a drop area to publish a new version. Problems found by health checks appear here with a fix button.

=== screen-deploys
title: Deploys tab (history)
q: deploys tab | deploy history | publish an old version | lock deploys button | what do deploy statuses mean
tags: deploys history versions publish roll back draft lock status
hi: deploys tab | purane version
go: Projects -> #/projects
screen: deploys
---
Every upload is kept here with time, source, files and size. Statuses: **live** (visitors see it), **published** (was live), **draft** (saved, not published), **partial** (worked on some servers), **failed**. Press **Publish (roll back)** on any row to make it live again. **Lock** turns automatic deploys into drafts.

=== screen-domain-tab
title: Project Domain tab
q: domain tab | how does the domain tab work | dns step domain tab | check domain button
tags: domain tab project steps dns check https primary www
hi: domain tab
go: Domains -> #/domains
screen: domain
---
Three steps: **1.** type the domain (www is added automatically), **2.** point the DNS - automatically with Cloudflare or GoDaddy/Porkbun API keys, or by hand with copy buttons and a guide for your registrar, **3.** checks: DNS → server, HTTPS certificate. **Check** re-runs them. Ask Jiv *"show me how to add a domain"* for a walkthrough.

=== screen-hosting
title: Hosting accounts page
q: hosting accounts page | add a hosting account | connect provider | servers list page
tags: hosting accounts providers servers list add create home this pc
hi: hosting accounts page
go: Hosting accounts -> #/hosting
screen: hosting
---
Connect cloud providers (Hetzner, DigitalOcean, Oracle, AWS Lightsail and more) to create servers with the price shown first, add **any other VPS** by IP, add a **home server**, or turn on **This PC**. Each server card shows status and lets you make it the active server or open it.

=== screen-thispc
title: This PC page
q: this pc page | host from my computer settings | keep pc awake hosting | lan access this pc
tags: this pc page port lan awake tunnel start with windows
hi: this pc page
go: This PC -> #/thispc
screen: thispc
---
Turn **Host on this PC** on to serve projects with Rover's built-in web server. Options: port (8080), allow other devices at home (LAN), start with Windows, keep the PC awake. **Connect tunnel** makes it reachable from the internet through Cloudflare without port forwarding. Best used as a backup server or for testing.

=== screen-home-server
title: Home server page
q: home server page | add raspberry pi page | home server connect button | reach home server from internet
tags: home server page raspberry pi old laptop connect tunnel lan
hi: home server page
go: Home server -> #/hosting/home
screen: home-server
---
Type the home machine's local address (like 192.168.1.20), user and password, then **Connect**. Step 2 chooses how visitors reach it: **Cloudflare tunnel** (recommended, no port forwarding) or your router's port forwarding. Rover installs the Lite engine (nginx).

=== screen-jiv-settings
title: Settings > Jiv
q: jiv settings page | change jiv language | turn off the brain | jiv graphics card setting
tags: jiv settings language brain gpu offline pack nudges
hi: jiv settings
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Choose Jiv's language (auto, English, Hindi), download or remove the **offline brain** (Standard or Light), see your **graphics card** and whether Jiv uses it, **import or export an offline pack**, and turn tips on or off. Everything here works without internet except the optional download.

=== screen-updates
title: Settings > Updates
q: updates page | check for updates button | install update button
tags: updates page check install version release signature
hi: update kaise kare
go: Updates -> #/settings/updates
screen: updates
---
**Check now** looks for a new version, **Install** downloads it, verifies its signature and restarts Rover (your projects keep running). Turn on **automatic updates** to install them when Rover is idle. Ask *"show me how to update the app"*.

=== screen-connect-ai
title: Settings > Connect AI
q: connect ai page | add openai key | use ollama | which ai providers | cloud ai privacy
tags: connect ai openai anthropic gemini ollama key cloud local providers
hi: ai kaise jode
go: Connect AI -> #/settings/ai
screen: ai
---
Optional: connect a cloud AI (OpenAI, Anthropic, Google...) with your own key or a local one (Ollama, LM Studio) for Jiv's free-form answers. Keys are encrypted with DPAPI on this PC. Secrets, tokens and passwords are removed before anything is sent to a cloud AI, and you approve each company once. Jiv's offline brain needs none of this.

=== screen-guides
title: Walkthroughs page
q: walkthroughs page | guides page | where are the tutorials
tags: guides page walkthroughs tutorials list start
hi: guides page
go: All walkthroughs -> #/guides
screen: guides
---
All guided tasks grouped by topic: servers, projects, domains, backups and the app. Press **Start** and Jiv highlights each step. Steps on other websites show as checklists with pictures and copy buttons.

=== screen-review
title: Review page before publishing
q: review page | what is the publish button | project name before publish | check before publishing
tags: review publish name checks before deploy cancel build
hi: review page
go: Add new project -> #/new
screen: review
---
After you drop files Rover shows what it found (pages, size, any problems like a missing index.html or a project that needs building), a **name** for the project and the **Publish** button. Nothing goes online until you press Publish.
