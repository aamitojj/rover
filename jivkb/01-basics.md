=== what-is-dilreet
title: What Rover does
q: what is rover | what can this app do | how does rover work | what is coolify | explain how this works | getting started
tags: about overview start beginner coolify vps
hi: rover kya hai | yeh app kya karta hai | रोवर क्या है | यह ऐप क्या करता है
go: Open the dashboard -> #/dashboard ; Read the guide -> #/guide
screen: dashboard
---
Rover is a remote control on your PC for websites that live on **your own server** (a VPS you rent, e.g. from Hetzner or DigitalOcean).

1. **Connect a server** (Hosting accounts or Settings > Server). Rover installs the free, open-source hosting engine **Coolify** on it for you.
2. **Add a project**: drag a folder or .zip, paste a Git link, or import from Netlify. You get a working https address straight away.
3. **Add your domain** in the project's Domain tab. Rover shows the exact steps for your registrar (or does them itself with an API key).

Everything is kept on this PC too (Documents\Rover\Projects), so you can roll back any time. Nothing depends on GitHub or any other service.
@hi
Rover आपके PC पर एक रिमोट कंट्रोल है, जो आपके अपने सर्वर (VPS) पर वेबसाइट चलाता है।

1. **सर्वर जोड़ें** (Hosting accounts या Settings > Server)। Rover उस पर Coolify अपने-आप इंस्टॉल करता है।
2. **प्रोजेक्ट जोड़ें**: फ़ोल्डर या .zip खींचकर छोड़ें, Git लिंक दें या Netlify से लाएँ। तुरंत एक https पता मिलता है।
3. **डोमेन जोड़ें**: प्रोजेक्ट के Domain टैब में। Rover आपके रजिस्ट्रार के लिए सही स्टेप दिखाता है।

=== first-steps
title: First steps: from nothing to a live website
q: how do i start | i am new what do i do first | setup from scratch | how to put my website online | how to host my website | onboarding | first time
tags: start beginner onboarding host online live setup checklist
hi: shuru kaise kare | website online kaise kare | pehli baar | वेबसाइट ऑनलाइन कैसे करें | शुरू कैसे करें
go: Connect a hosting account -> #/hosting ; Add new project -> #/new
screen: setup
---
1. **Get a server.** If you have none, open **Hosting accounts**, connect a provider (Hetzner, DigitalOcean, Vultr...) and click **Create server** - Rover shows the monthly price first. Already have a VPS? Choose **Any other VPS** and type its IP.
2. **Let Rover set it up.** Click **Use this server for Rover**. It installs the hosting engine (about 5-10 minutes) - nothing to type.
3. **Add a project.** Click **Add new project** and drop your site folder or .zip. You get an https address right away.
4. **Add your domain** (optional) in the project's **Domain** tab.

The checklist on the Dashboard shows what is left. Press **Ctrl+K** any time to jump anywhere.
@hi
1. **सर्वर लें।** Hosting accounts खोलें, कोई प्रोवाइडर जोड़ें और **Create server** दबाएँ (पहले मासिक कीमत दिखेगी)। पहले से VPS है तो **Any other VPS** चुनें और IP लिखें।
2. **Use this server for Rover** दबाएँ - Rover सब कुछ इंस्टॉल करेगा (5-10 मिनट)।
3. **Add new project** दबाएँ और अपना फ़ोल्डर या .zip छोड़ें।
4. प्रोजेक्ट के **Domain** टैब में अपना डोमेन जोड़ें।

=== dashboard
title: The Dashboard
q: what does the dashboard show | explain the dashboard | what is needs attention | what is the setup checklist
tags: dashboard attention checklist overview home status
hi: dashboard kya dikhata hai | डैशबोर्ड
go: Open the dashboard -> #/dashboard
screen: dashboard
---
The Dashboard is the home screen:

- **Needs attention** lists anything broken (a failed deploy, a site that does not answer, a domain waiting for DNS, a certificate about to expire) with a one-click **Fix** button.
- **Setup checklist** shows what is left: server, first project, domain, backups, Jiv's offline brain.
- **Pinned projects** and **recent deploys** are one click away.

Ask Jiv "what needs my attention?" for a plain explanation of every item.

=== projects-screen
title: The Projects list
q: where are my projects | how do i find a project | how to search projects | what are groups and tags | how to pin a project | archive a project
tags: projects list search group tag pin archive filter
hi: mere projects kahan hai | project kaise dhundhe | प्रोजेक्ट कहाँ हैं
go: Open Projects -> #/projects
screen: projects
---
**Projects** lists every website. You can:

- **Search** by name, domain or tag, and filter by group.
- **Pin** important projects (they show first and on the Dashboard).
- **Archive** old ones - they keep running, they just leave the main list. Use **Undo** in the message that appears if you clicked by mistake.
- Put projects in **groups** (e.g. per customer) and add coloured **tags** in Settings > Groups & tags.

Click a project to open its Files, Deploys, Domain, Logs and Settings tabs.

=== new-project
title: Add a new project (deploy a website)
q: how do i deploy | how to upload my website | how to publish my site | add new project | drag and drop folder | deploy a zip | create a site | make a new website
tags: deploy upload publish new project create zip folder drop
hi: website upload kaise kare | deploy kaise kare | naya project kaise banaye | वेबसाइट अपलोड कैसे करें | नया प्रोजेक्ट
go: Add new project -> #/new ; Start from a template -> #/new/template
do: create_site
screen: new
---
1. Click **Add new project** (or press **N**).
2. Type a name - this becomes the address, e.g. *bakery*.
3. **Drag your site folder or .zip** onto the drop area, or click to choose it. You can also paste a Git link, import from Netlify, or start from a template.
4. Rover checks the files, uploads them and shows the live address (https://bakery.<server-ip>.sslip.io) in about a minute.

Tip: your site needs an **index.html** in its top folder. React/Vue/Vite sources can be built for you with one click (needs Node.js), or drop the built *dist* folder.
@hi
1. **Add new project** दबाएँ (या **N**)।
2. नाम लिखें, जैसे *bakery*।
3. अपनी साइट का **फ़ोल्डर या .zip** ड्रॉप एरिया पर छोड़ें। Git लिंक या Netlify से भी ला सकते हैं।
4. लगभग एक मिनट में लाइव पता मिल जाएगा।

ध्यान दें: ऊपर वाले फ़ोल्डर में **index.html** होना चाहिए।

=== redeploy-files
title: Change files and deploy again
q: how do i update my website | edit my site files | how to change the website content | redeploy | upload a new version | deploy from folder
tags: update edit files current folder redeploy version change
hi: website update kaise kare | file kaise badle | वेबसाइट अपडेट कैसे करें
go: Open Projects -> #/projects
screen: files
---
Every project has a folder on this PC: **Documents\Rover\Projects\<name>\current**.

1. Open the project's **Files** tab and click **Open folder** (or open the files in your editor).
2. Change what you need and save.
3. Click **Deploy from this folder**.

Or just drop a new folder/zip on the project page. Turn on **Auto-deploy when this folder changes** in the project's Settings to publish every time you save.

=== project-folder
title: Where my files are stored on this PC
q: where are my files saved | where is the project folder | local copy of my site | storage location | deploy history zips
tags: folder storage documents local copy zip history disk location
hi: files kahan save hoti hai | फ़ाइलें कहाँ सेव होती हैं
go: Storage settings -> #/settings/storage
screen: storage
---
Each project lives in **Documents\Rover\Projects\<name>**:

- **current** - the latest files (edit them and deploy again).
- **deploys** - a .zip of every deploy, so you can download, compare or publish any of them again.

Change the location or how many old deploys to keep in **Settings > Storage**. Your files never depend on the server: if the server breaks, everything can be published again from here.

=== rollback
title: Roll back to an earlier version
q: how do i undo a deploy | go back to previous version | restore the old website | rollback | revert my site | the new version is broken
tags: rollback undo revert previous restore older version deploys
hi: purana version wapas kaise laye | rollback kaise kare | पुराना वर्ज़न वापस कैसे लाएँ
go: All deploys -> #/deploys
do: rollback_site
screen: deploys
---
1. Open the project and go to the **Deploys** tab.
2. Find the version that worked and click **Publish**.

If the server still has it, the switch is instant; otherwise Rover uploads it again from the copy on this PC. You can also just ask me: *"roll back bakery"* - I will show a confirm card first.
@hi
1. प्रोजेक्ट खोलें और **Deploys** टैब पर जाएँ।
2. जो वर्ज़न ठीक चल रहा था उस पर **Publish** दबाएँ।

या मुझसे कहें: *"bakery rollback karo"* - मैं पहले पुष्टि माँगूँगा।

=== deploys-screen
title: The Deploys screen
q: what does the deploys page show | deploy history | why did my deploy fail | see all deploys | deploy log
tags: deploys history failed log status publish compare
hi: deploy history kahan hai | डिप्लॉय हिस्ट्री
go: All deploys -> #/deploys
screen: deploys
---
**Deploys** lists every upload of every project, newest first, with its source (drop, folder, Git, Netlify, watch), file count and result.

- **Failed** deploys have a **Why did it fail?** button - I read the error and tell you the fix in plain words.
- **Publish** puts any older deploy live again.
- **Download** gives you the exact zip that was published; **Compare** shows which files changed.

=== templates
title: Start from a template
q: i don't have a website yet | use a template | starter site | create a website without files | sample website
tags: template starter sample blank portfolio landing
hi: template se website kaise banaye | मेरे पास वेबसाइट नहीं है
go: Templates -> #/new/template
screen: new
---
No files yet? Open **Add new project > Start from a template**, pick one (landing page, portfolio, coming soon, docs...) and give it a name. Rover publishes it straight away and the files land in the project's folder on this PC, ready to edit.

=== git-import
title: Import from GitHub, GitLab or a Git link
q: deploy from github | import a repository | connect github | private repo | auto deploy on push | git branch | gitlab bitbucket
tags: git github gitlab bitbucket repository repo branch commit auto private token
hi: github se deploy kaise kare | गिटहब से डिप्लॉय
go: Import from Git -> #/new/git ; Git settings -> #/settings/git
do: pull_and_deploy
screen: git
---
1. **Add new project > Import from Git or link**.
2. Paste the repository URL, pick the branch and (optionally) a subfolder.
3. Private repo? Click **Connect account** and create a read-only token with the link shown. Rover stores it encrypted on this PC.
4. Turn on **auto-deploy** to publish new commits while Rover is open (it checks every few minutes - no webhooks needed).

GitHub is optional: you can always drag and drop instead.

=== netlify-migrate
title: Move a site from Netlify
q: migrate from netlify | import my netlify site | move away from netlify | netlify redirects headers | netlify to rover
tags: netlify migrate move import redirects headers toml forms functions
hi: netlify se kaise shift kare | नेटलिफ़ाई से साइट लाना
go: Import from Netlify -> #/new/netlify
screen: netlify
---
1. **Add new project > Import from Netlify**.
2. Paste a Netlify personal access token (the link opens the right Netlify page), pick the site.
3. Rover copies the published files, `_redirects`, `_headers` and `netlify.toml` rules, and offers to move the custom domain.

Not available on your own server: Netlify Functions, Forms and Identity. Everything else (redirects, headers, SPA fallbacks, HTTPS) works the same.

=== app-testing
title: Test a site before publishing (App Testing)
q: how do i test my site before deploy | preview my site | check for broken links | app testing | mobile view | check report
tags: test testing preview check broken links mobile report before
hi: deploy se pehle test kaise kare | टेस्ट कैसे करें
go: App Testing -> #/testing
screen: testing
---
1. Open **App Testing** and drop the same folder or zip you would deploy.
2. Click through the private preview (only this PC can open it). Use **Phone / Tablet / Desktop** to check other screens.
3. Read the **Check report**: *fail* = visitors see something broken, *warn* = worth fixing. Each line says how to fix it.
4. Click **Deploy this version** to publish exactly what you tested.

=== redirects-headers
title: Redirects and headers (_redirects, _headers, netlify.toml)
q: how do i add a redirect | single page app routing 404 | react router refresh 404 | custom headers | cache headers | _redirects file | redirect old page | add security headers | edit _redirects | edit _headers | redirects editor
tags: redirects headers spa react router 404 rules nginx toml cache
hi: redirect kaise lagaye | रीडायरेक्ट
go: App Testing -> #/testing
screen: rules
---
Rover understands Netlify's `_redirects`, `_headers` and `netlify.toml`. Put the file in your site's top folder, or edit the rules in the app: open the project > **Configuration** > **Redirects & headers** (ready-made examples, a **Check** button and **Save and publish**).

Single-page app (React/Vue) - every unknown address shows index.html:
`/*  /index.html  200`

Moved page: `/old-page  /new-page  301`

An existing file always wins over a rule unless you add `!` (e.g. `301!`). If a rule can't be converted, the live site is not changed and App Testing shows which line to fix.

=== build-node
title: "This project must be built first" (React, Vue, Vite, Astro)
q: this project must be built first | npm build | node js required | react app deploy | vite build | dist folder
tags: build npm node react vue vite astro dist framework
hi: build kaise kare | npm install error | बिल्ड
go: Add new project -> #/new
screen: build
---
Your folder has source code (package.json) but no built site yet.

- Click **Build it for me** - Rover runs `npm install` and `npm run build` on this PC. It needs **Node.js LTS** from nodejs.org (install it once).
- Or build it yourself and drop the output folder (usually **dist** or **build**).

Rover only runs npm when you click the button.

=== backups-alerts
title: Backups and alerts
q: how do i back up my sites | automatic backup | where are backups | get notified when a site is down | email alerts | backup now
tags: backup alerts notify notification email windows restore schedule
hi: backup kaise le | बैकअप कैसे लें
go: Backups & alerts -> #/settings/backups
do: backup_now
screen: backups
---
**Settings > Backups & alerts**:

- **Backups** copy every project (its files and deploy history) to a folder you pick - an external drive or a synced folder like OneDrive. Choose daily or weekly, or click **Back up now**.
- **Alerts** show a Windows notification when a site goes down, a deploy fails, a domain goes live or a certificate is about to expire.

Your server provider's snapshots are still a good idea for the server itself.

=== shortcuts
title: Keyboard shortcuts and the command palette
q: keyboard shortcuts | command palette | ctrl k | how to jump quickly | search everything | hotkeys
tags: shortcut keyboard palette ctrl k search quick jump hotkey
hi: shortcut keys | कीबोर्ड शॉर्टकट
go: Show shortcuts -> #/shortcuts
screen: any
---
- **Ctrl+K** - command palette: type anything (a project, a page, an action like "deploy bakery", or a question) and press Enter.
- **Ctrl+J** - ask Jiv.
- **N** - add a new project.
- **?** - show all shortcuts.
- **Esc** - close a window.

Every action that changes your server or DNS still asks you to confirm.

=== undo
title: Undo
q: undo | i clicked by mistake | restore archived project | undo archive | undo rollback
tags: undo mistake restore archive unpin revert
hi: galti se click ho gaya | undo kaise kare | गलती से
go: Projects -> #/projects
screen: any
---
Small changes (archive, pin, group, rollback) show a message with an **Undo** button for a few seconds - click it to put things back.

For deploys you can always publish an earlier version from the **Deploys** tab. Deleting a project asks you to type its name first, and the local copy stays in Documents\Rover\Projects.

=== delete-project
title: Delete a project
q: how do i delete a site | remove a project | delete website | stop hosting a site
tags: delete remove project site stop destroy
hi: project delete kaise kare | प्रोजेक्ट डिलीट
go: Projects -> #/projects
screen: project-settings
---
Open the project > **Settings** > **Delete project**. Type the project name to confirm. Rover removes it from the server (the domain stops working); the local folder on this PC is kept unless you tick *also delete local files*.

Just want to hide it? Use **Archive** instead - the site keeps running.

=== updates
title: Update Rover (automatic updates from GitHub) and the hosting engine
q: how do i update rover | new version | check for updates | automatic updates | auto update | update available | skip this version | beta version | pre-release | upgrade coolify | update the hosting engine
tags: update upgrade version release github auto automatic beta channel skip remind coolify engine
hi: update kaise kare | rover update kaise kare | naya version | अपडेट | नया वर्जन
go: Updates -> #/settings/updates
screen: updates
---
Open **Settings > Updates**.

1. Click **Check for updates now** any time. Rover asks GitHub for the newest release (no account needed).
2. **Check for updates automatically** is on by default: at startup and once a day. When a new version appears you see a small **Update available** button in the sidebar and a window with **what's new**.
3. In that window choose **Install and restart**, **Remind me later** (tomorrow) or **Skip this version**.
4. Prefer no questions? Choose **Download and install automatically**. Rover installs when nothing else is running and restarts by itself.
5. **Update channel**: *Stable* (recommended) or *Beta* (also offers pre-releases).

Every download is checked against the release's **SHA256SUMS.txt** (and GitHub's own checksum); if anything doesn't match, nothing is installed. Your projects and settings are never touched. Offline? Rover simply checks again later.

The same page can upgrade the hosting engine (Coolify) on your server with one click.
@hi
**Settings > Updates** में **Check now** दबाएँ, फिर **Install**। Rover सिग्नेचर जाँचकर अपडेट करता है; आपकी साइटें चलती रहती हैं। अपने-आप अपडेट के लिए **automatic updates** चालू करें।

=== privacy-security
title: Privacy and security: what is stored and where
q: is my password stored | where are my tokens saved | is it safe | what is sent to the internet | security | privacy | encryption
tags: privacy security password token encrypted dpapi cloud safe key
hi: password safe hai kya | kya data bahar jata hai | पासवर्ड सुरक्षित है
go: Settings -> #/settings
screen: any
---
- **Server passwords are never stored.** They are used once to install Rover's own SSH key, then the key is used.
- **Tokens and keys** (hosting accounts, Cloudflare, GoDaddy, Git, AI) are encrypted with Windows (DPAPI) so only your Windows account on this PC can read them.
- **Jiv works offline.** The built-in answers and the offline brain run on this PC. Cloud AI is only used if you connect it yourself, and secrets, tokens and passwords are never sent to it.
- Every action that changes a server, DNS or money asks you to confirm.
