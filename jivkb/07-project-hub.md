=== project-hub
title: The project page: everything for one site in one place
q: project page | project overview | what is on the project page | setup checklist | where do i manage my site | project tabs | quick actions
tags: project overview hub checklist tabs quick actions netlify
hi: project page par kya hai | project kaise manage kare | प्रोजेक्ट पेज
go: Projects -> #/projects
screen: project
---
Open a project from **Projects**. Its menu on the left works like Netlify's:

- **Overview**: the live address, quick actions (Redeploy last folder, Open site, Roll back), a big **drop area** for new versions and the **Set up this project** checklist (hosting, first deploy, domain, HTTPS, Git, alerts, backups). Each step has a button that does it right there.
- **Deploys**: drop a new build, see the history, compare, download, publish an older one (rollback) or delete it. Save **drafts** and publish them later.
- **Domains**: add a domain, the registrar wizard, HTTPS and the **primary domain** (www or not).
- **Hosting**: which servers run the project, their health, add a backup server (automatic failover), move to another server.
- **Configuration**: build settings, redirects & headers, password, notifications, backups, danger zone.
- **Files**, **Testing** and **Visitors & logs**.

Keys: **1-8** switch tabs, **D** deploy a folder, **R** redeploy the last folder, **O** open the site.

=== drop-deploy
title: Deploy a new version by drag and drop
q: how do i deploy a new build | drag and drop new version | upload new build | drop folder to deploy | deploy zip | update my website files | redeploy | deploy from clipboard | paste folder path
tags: deploy drop drag upload build zip folder redeploy new version paste clipboard
hi: naya build kaise upload kare | drag drop se deploy | नया वर्जन कैसे डालें
go: Projects -> #/projects
screen: deploys
---
1. Open the project. The **Deploy a new version** area is on the Overview and Deploys tabs.
2. **Drag your build folder** (for example `dist` or `build`) or a **.zip** onto it. You can drop **anywhere on the project page**.
3. Rover checks the files, sends them and shows the live address when it's done (usually seconds). Only changed files are sent (**fast deploys**).

More ways:
- **Browse for a folder** or **Choose a .zip** buttons.
- Copy a folder in File Explorer (*Shift + right-click > Copy as path*) and press **Ctrl + V** on the project page.
- **Redeploy last folder** publishes the same folder again in one click.
- Right-click a folder in File Explorer > **Deploy with Rover** (when chosen in the installer).
- Tick **Save as a draft** to preview first and publish later. Hold **Shift** while dropping for the same.

Your previous version stays on the server: **Roll back** brings it back instantly.
@hi
नया वर्ज़न डालने के लिए प्रोजेक्ट खोलें और अपना फ़ोल्डर या .zip **Deploys** (या Overview) पर छोड़ें। Rover जाँचकर पब्लिश करता है; पुराना वर्ज़न रखा जाता है ताकि आप वापस जा सकें।

=== drafts-lock
title: Drafts and locked publishing (publish later)
q: draft deploy | preview a draft deploy | publish later | lock publishing | stop auto publishing | auto deploy as draft | publish this deploy
tags: draft preview lock publish later auto publishing stop netlify locked
hi: draft kaise banaye | draft deploy
go: Projects -> #/projects
screen: deploys
---
- **Draft**: tick **Save as a draft** under the drop area (or hold Shift while dropping). The live site doesn't change. In **Deploys** the draft has **Preview** (opens it in App Testing) and **Publish this deploy**.
- **Lock publishing** (Deploys tab or Configuration > Build & deploy): watched-folder and Git deploys are saved as drafts instead of going live, like Netlify's *Lock to stop auto publishing*. Unlock to go back to automatic publishing.

=== password-protect
title: Protect a site with a password
q: password protect my site | private site | basic auth | hide site from public | client preview password | remove password from site
tags: password protect private access basic auth login hide htpasswd
hi: site par password kaise lagaye | वेबसाइट पर पासवर्ड
go: Projects -> #/projects
screen: project-settings
---
1. Open the project > **Configuration** > **Access & password**.
2. Turn on **Protect this site with a password**, type a user name and a password (or click **Generate**).
3. Click **Save and publish** and confirm. Visitors now see a login box from the browser.

Rover keeps only a scrambled copy (hash) of the password, encrypted on this PC. To remove it, turn the switch off and publish again.

=== primary-domain
title: Primary domain: www or not
q: www redirect | redirect www to non www | primary domain | canonical domain | both www and without www
tags: www primary canonical redirect domain apex
hi: www wala ya bina www
go: Domains -> #/domains
screen: domain
---
When both `example.com` and `www.example.com` are connected, open the project's **Domains** tab > **Primary domain** and pick the one visitors should end up on. The other forwards there (a 301 redirect, good for search engines). Click **Save and publish**.

=== project-hosting
title: Which server hosts this project
q: which server hosts my project | move project to another server | add hosting to project | server health | change hosting
tags: hosting server move health vps project
hi: project kis server par hai | server badalna
go: Hosting accounts -> #/hosting
screen: project-hosting
---
Open the project > **Hosting**. You see the server it runs on, whether Rover is connected, the hosting engine's version, **Restart the site**, **+ Add a hosting account**, and **Move this project** to another server (Rover publishes the last deploy there; then point your domain to the new IP).
