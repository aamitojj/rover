=== hosting-accounts
title: Hosting accounts: connect Hetzner, DigitalOcean, Vultr and more
q: how do i connect my hosting account | add hosting provider | connect hetzner | connect digitalocean | link my vps provider account | multiple hosting accounts | api token for my provider
tags: hosting account provider hetzner digitalocean vultr linode akamai lightsail aws oracle contabo hostinger scaleway upcloud token oauth connect
hi: hosting account kaise jode | hetzner kaise connect kare | होस्टिंग अकाउंट कैसे जोड़ें
go: Hosting accounts -> #/hosting
do: open_page page=hosting
screen: hosting
---
Open **Hosting accounts** and click **Connect an account**. Pick your provider:

- **DigitalOcean** - click **Sign in with DigitalOcean** (your browser opens their real sign-in page; 2FA works).
- **Hetzner Cloud, Vultr, Linode/Akamai, Scaleway, UpCloud, Hostinger, Contabo** - Rover opens the provider's token page and shows the steps; paste the token and Rover checks it.
- **AWS Lightsail** - an access key pair; **Oracle Cloud** - an API key.
- **Any other VPS** - just its IP and SSH.

You can connect **several accounts**, even several for the same provider (e.g. one per customer). Rover never asks for your provider website password. Tokens are encrypted on this PC and never sent to any AI.
@hi
**Hosting accounts** खोलें और **Connect an account** दबाएँ, फिर प्रोवाइडर चुनें:

- **DigitalOcean** - **Sign in with DigitalOcean** (ब्राउज़र में असली साइन-इन पेज खुलेगा)।
- **Hetzner, Vultr, Linode, Scaleway, UpCloud, Hostinger, Contabo** - Rover टोकन पेज खोलेगा; टोकन पेस्ट करें।
- **Any other VPS** - सिर्फ़ IP और SSH।

कई अकाउंट जोड़ सकते हैं। Rover कभी वेबसाइट पासवर्ड नहीं माँगता।

=== hosting-providers-support
title: Which providers can Rover control?
q: which hosting providers are supported | can rover create a server | providers with api | ssh only providers | does it work with contabo | oracle cloud support | hostinger vps
tags: provider support api create list ssh oauth token hetzner digitalocean vultr linode lightsail oracle contabo hostinger scaleway upcloud
hi: kaun se provider support hai
go: Hosting accounts -> #/hosting
screen: hosting
---
- **List servers + create servers (cost shown first):** Hetzner Cloud, DigitalOcean, Vultr, Linode/Akamai, AWS Lightsail.
- **List servers (order new ones on their website):** Oracle Cloud, Contabo, Hostinger VPS, Scaleway, UpCloud.
- **Any other VPS:** IP + SSH only.
- **Sign-in method:** DigitalOcean uses browser sign-in (OAuth) or a token; Lightsail uses access keys; Oracle uses an API key; Contabo uses API client details; the rest use one API token.

For every server in the list, **Use this server for Rover** sets it up automatically.

=== create-server
title: Create a new server from Rover
q: create a new server | buy a vps | how much does a server cost | which server size | new droplet | cheapest server | server for 100 sites
tags: create server vps buy cost price size plan region droplet cheapest
hi: naya server kaise banaye | server kitne ka hai | नया सर्वर
go: Hosting accounts -> #/hosting
screen: hosting
---
1. Open **Hosting accounts**, choose a connected account and click **Create server**.
2. Pick a size and location. Rover suggests one that fits the hosting engine (at least 2 CPU / 2-4 GB RAM) and shows the **monthly price** before anything happens.
3. Click **Create and set up** and confirm. Rover creates the server with its own SSH key, waits for it to start, and installs everything (5-10 minutes).

Rough sizing: a few sites - 2 vCPU / 4 GB; about 100 static sites - 4 vCPU / 8 GB / 80 GB SSD. Turn on the provider's backups.

=== use-server
title: Use an existing server for Rover
q: use this server | set up my vps | install coolify on my server | connect my existing vps | server setup automatically | ssh password
tags: use server setup install coolify vps ssh password key existing connect
hi: server kaise connect kare | vps setup kaise kare | सर्वर कैसे जोड़ें
go: Hosting accounts -> #/hosting ; Server settings -> #/settings/server
screen: server
---
- From **Hosting accounts**: click **Use this server for Rover** next to a server.
- Any other VPS: **Settings > Server**, type the IP.

If Rover's key is not on the server yet, type the root (or *ubuntu*) password **once** - Rover installs its own key and forgets the password. Then click **Set up server**: it checks the system, adds swap if needed, installs the hosting engine (Coolify) and connects. Use a fresh Ubuntu 22.04/24.04 server with nothing else on it.

=== multiple-servers
title: Several servers: choose where a project lives
q: multiple servers | switch server | use another server for a project | use a different server | move a project to another server | which server is my site on | deploy to a different server | active server
tags: multiple servers switch move active profile project choose
hi: dusre server par project kaise le jaye | सर्वर बदलें
go: Hosting accounts -> #/hosting
do: open_page page=hosting
screen: hosting
---
Rover can manage several servers. One is **active** at a time (shown at the bottom of the sidebar - click it to switch).

- New projects go to the active server.
- Each project shows its server in **Settings > Server**. Click **Move to another server** to republish it there (the last deploy is uploaded again; then move the domain's A record - Rover tells you the new IP).
- Opening a project on another server asks you to switch first.
- To run one project on **several servers at once** (a backup that takes over if one goes down), open the project > **Hosting** > **Add a backup server**.

=== firewall-ports
title: Firewall and ports
q: which ports to open | firewall settings | port 8000 | port 22 blocked | cannot reach coolify | 6001 6002
tags: firewall ports 22 80 443 8000 6001 6002 blocked open security group
hi: kaun se port khole | पोर्ट
go: Server settings -> #/settings/server
screen: server
---
Allow incoming: **22** (SSH), **80** and **443** (websites), **8000** (the hosting engine dashboard; you can close it after giving it its own HTTPS address), **6001-6002** (real-time updates).

If port 8000 is blocked, tick **Send API calls through the SSH connection** in Settings > Server. When Rover creates the server itself (e.g. on Lightsail) it opens these ports for you.

=== ssh-failed
title: SSH connection failed
q: ssh connection failed | cannot connect to server | connection refused | i/o timeout connecting | permission denied publickey | wrong password server | unable to authenticate
tags: ssh connect failed refused timeout password key denied authenticate port 22 ppk
hi: server connect nahi ho raha | ssh error | सर्वर कनेक्ट नहीं हो रहा
go: Server settings -> #/settings/server
screen: server
---
1. **Check the IP** and that the server is running in your provider's panel.
2. **Port 22** must be open in the provider firewall.
3. **Password rejected?** Some providers disable password login - use the key file they gave you (OpenSSH format; convert a PuTTY .ppk with PuTTYgen > Conversions > Export OpenSSH key). On AWS/Oracle the user is *ubuntu*, not root.
4. **Timeout** usually means a firewall or a wrong IP.

Then click **Connect** again in Settings > Server.
@hi
Rover सर्वर से नहीं जुड़ पा रहा। जाँचें: सर्वर चालू है, IP सही है, पोर्ट 22 खुला है, और यूज़र/पासवर्ड सही है (Oracle/AWS पर यूज़र **ubuntu**)। फिर Settings > Server में **Connect** दबाएँ।

=== host-key
title: "Host key changed"
q: host key changed | host key mismatch | remote host identification has changed | forget host key
tags: hostkey host key changed mismatch reinstall forget
hi: host key change error
go: Server settings -> #/settings/server
screen: server
---
Normal if you **reinstalled** the server or got a new one with the same IP: click **Forget host key** in Settings > Server and connect again. If you did **not** change anything, do not continue - someone could be pretending to be your server. Ask your provider.

=== coolify-token
title: Hosting engine error 401 / invalid token / 403
q: coolify api error 401 | invalid token | unauthenticated | coolify 403 | cannot create site 403 | api token coolify
tags: coolify token 401 403 invalid unauthenticated api permission engine
hi: coolify token error
go: Server settings -> #/settings/server
screen: server
---
- **401 / invalid token**: if Rover set up the server, run **Settings > Server > Set up automatically** again - it makes a new token. Otherwise create a token in Coolify (Keys & Tokens) and paste it under *Use an existing Coolify*. Make sure the API is enabled (Coolify Settings > Advanced).
- **403 when creating a site**: the token needs the **deploy** (or root) permission.

=== server-full
title: Server disk full or out of memory
q: no space left on device | disk full | server out of memory | server is slow | ram full | too many sites
tags: disk space full memory ram slow swap storage server resize
hi: server full ho gaya | disk full | सर्वर धीमा है
go: Server settings -> #/settings/server
screen: server
---
- **No space left**: old Docker images pile up. In Coolify, Servers > localhost > *Cleanup*; or resize the disk at your provider. Rover keeps your deploys on this PC, so you can lower the keep-limit on the server.
- **Out of memory / slow**: Rover adds swap during setup. For many sites use at least 4 GB RAM; resize the server in the provider panel (most allow it in a few clicks).
@hi
सर्वर की डिस्क भर गई है। Jiv से कहें *"server ki disk khali karo"* - वह पहले दिखाएगा कि जगह किसने भरी, फिर पुष्टि के बाद पुराने Docker images और लॉग हटाएगा। बार-बार भरे तो बड़ी डिस्क लें।

=== site-down
title: My site is down / not loading
q: why is my site down | website not working | site not loading | site shows error | 502 bad gateway | site offline | exited degraded
tags: down offline not-working broken 502 503 exited degraded restart status health
hi: meri site band hai | website nahi chal rahi | site down kyun hai | मेरी वेबसाइट बंद है | साइट नहीं खुल रही
go: Dashboard -> #/dashboard
do: diagnose
screen: dashboard
---
Ask me *"why is bakery down?"* - I check the server connection, the site's container, the last deploy, the health check and the domain, and tell you the fix.

Common fixes:
1. **Status exited/degraded** - click **Restart** on the project (or let me do it).
2. **Last deploy failed** - roll back to the last good deploy.
3. **Server unreachable** - check it is running in your provider panel.
4. **Only the domain fails** but the instant address works - it's DNS; see *Domain is still waiting for DNS*.
@hi
मुझसे पूछें *"bakery down kyun hai?"* - मैं सर्वर, कंटेनर, आखिरी डिप्लॉय और डोमेन जाँचकर बताऊँगा।

आम उपाय:
1. **exited/degraded** - प्रोजेक्ट पर **Restart** दबाएँ।
2. **आखिरी डिप्लॉय फेल** - पिछले अच्छे डिप्लॉय पर rollback करें।
3. **सर्वर नहीं मिल रहा** - प्रोवाइडर पैनल में देखें कि सर्वर चालू है।

=== old-content
title: I see old content / 404 after deploying
q: still old version after deploy | 404 after deploy | changes not showing | page not found | cache
tags: 404 old content cache not-found index hard refresh changes
hi: changes dikh nahi rahe | पुराना कंटेंट
go: Deploys -> #/deploys
screen: deploys
---
1. Make sure **index.html is in the top folder** of your zip (not inside another folder). Rover can fix this automatically on upload.
2. Hard-refresh the browser (**Ctrl+F5**) - browsers cache pages.
3. Addresses are **case-sensitive** online (About.html is not about.html).
4. A single-page app needs a `/* /index.html 200` rule.

=== nginx-rejected
title: "nginx rejected the generated config"
q: nginx rejected the generated config | redirect rule error | headers rule failed
tags: nginx rejected config rules redirect headers error
hi: nginx error
go: App Testing -> #/testing
screen: rules
---
One of your `_redirects`/`_headers` rules could not be converted. **The live site was not changed.** Open App Testing with the same files: *Check report > Redirect and header rules* shows which line to fix. Then deploy again.
