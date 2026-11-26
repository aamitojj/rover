=== settings-general
title: Settings > General
q: general settings | change theme | dark mode | background checks | start with windows | language
tags: settings general theme dark light background start windows language
hi: settings kahan hai | dark mode kaise kare
go: Settings -> #/settings
screen: settings
---
**Settings > General** holds the look (light/dark), **background checks** (domains, HTTPS, health, auto-deploys while Rover is open) and start-up options. Jiv's language and the offline brain are in **Settings > Jiv**.

=== organise
title: Groups and tags
q: how do i make groups | add tags to projects | organise projects by customer | tag colours
tags: group tag organise customer colour label
hi: group kaise banaye
go: Groups & tags -> #/settings/organise
screen: organise
---
**Settings > Groups & tags**: create groups (for example one per customer) and tag colours. Then in a project's **Settings** pick its group and tags, or select several projects in the Projects list and use the bulk bar. Filter the list by group or tag.

=== logs-tab
title: Logs: see what the site and server did
q: where are the logs | see deploy log | error log | access log | explain this log
tags: logs log error output container deploy explain
hi: logs kahan dikhte hai | लॉग
go: Projects -> #/projects
do: explain_logs
screen: logs
---
Open the project's **Logs** tab for the container's recent output and the last deploy log. Click **Explain** (or ask me *"explain the logs of bakery"*) and I point out the important lines in plain English.
@hi
**Logs** टैब सर्वर से आपकी साइट की ताज़ा लाइनें दिखाता है। समझ न आए तो Jiv से कहें *"bakery ke logs samjhao"* - वह ज़रूरी लाइनें चुनकर आसान भाषा में बताएगा।

=== project-settings
title: Project configuration (name, build settings, rules, password, alerts, backups)
q: project settings | project configuration | change project name | publish folder | build command | auto deploy when folder changes | watch folder | lock publishing | where are the settings of my project
tags: project settings configuration general build deploy publish folder command watch lock rename notes archive
hi: project settings kaha hai | auto deploy kaise on kare
go: Projects -> #/projects
do: toggle_auto_deploy
screen: project-settings
---
Open a project and click **Configuration**. It has sections like Netlify's:

- **General**: name, group, tags, notes, pin, duplicate, archive.
- **Build & deploy**: publish folder (the one with index.html), build command for Git projects, **Lock publishing**, **Fast deploys**, a **watched folder** that redeploys by itself, Git auto-deploy and your recent folders.
- **Redirects & headers**: edit `_redirects` and `_headers` with checks and ready-made examples.
- **Access & password**: protect the site with a user name and password.
- **Notifications** and **Backups** for this project.
- **Danger zone**: archive, restart or delete.

The server that hosts the project (and moving it) is in the **Hosting** tab.

=== health-checks
title: Health checks (is my site answering?)
q: health check | uptime monitoring | is my site up | check all sites | ping my site | response time
tags: health uptime monitor check ping response time status certificate
hi: site chal rahi hai ya nahi kaise check kare
go: Dashboard -> #/dashboard
do: run_health
screen: dashboard
---
While Rover is open it checks every site every few minutes (like Uptime Kuma): does it answer, how fast, and how long until its HTTPS certificate expires. Problems appear under **Needs attention** and as a Windows notification. Ask me *"check all my sites"* to run it now.

=== domains-screen
title: The Domains screen
q: list of all domains | domains page | which domains are live | domain status
tags: domains list status live pending https
hi: sab domains kahan dikhte hai
go: Domains -> #/domains
screen: domains
---
**Domains** lists every custom domain with its status: *live*, *waiting for DNS*, *HTTPS pending*. Click one to open its project's Domain tab with the exact next step.

=== server-settings
title: Settings > Server
q: server settings | change server ip | reconnect server | set up server again | coolify url
tags: server settings ip connect reconnect setup coolify ssh
hi: server settings
go: Server settings -> #/settings/server
screen: server
---
**Settings > Server** shows the active server: IP, SSH user, connection status and the hosting engine. Here you can **Connect**, **Set up automatically** (install/repair Coolify), **Forget host key**, or use an existing Coolify with a token. For several servers and provider accounts, use **Hosting accounts**.
