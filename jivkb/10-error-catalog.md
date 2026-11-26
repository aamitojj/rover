=== error-redirect-loop
title: "Too many redirects" / redirect loop
q: too many redirects | err too many redirects | redirect loop | page keeps redirecting | redirected you too many times
tags: redirect loop too many redirects cloudflare flexible ssl www https
hi: too many redirects error | baar baar redirect
go: Domain tab -> #/projects
screen: domain
---
The browser is sent back and forth between two addresses. Usual causes:

- **Cloudflare SSL mode is "Flexible"** while your server also forces HTTPS. In Cloudflare open **SSL/TLS > Overview** and choose **Full (strict)**.
- A **_redirects** rule sends a page to itself (for example `/* /index.html 301` - use `200` for single-page apps).
- **www and non-www** both redirect to each other: in the Domain tab choose only one *Primary address*.

Ask Jiv *"fix the redirects on bakery"*: it reads the log, then rebuilds the web server settings (tested before they go live).

=== error-502
title: 502 Bad Gateway / 503 / 504
q: 502 bad gateway | 503 service unavailable | 504 gateway timeout | nginx 502 | site shows bad gateway
tags: 502 503 504 bad gateway unavailable proxy upstream down restart
hi: 502 error aa raha hai
go: Overview -> #/dashboard
screen: dashboard
---
The front door (proxy) works, but the site behind it doesn't answer. Try in this order:

1. Ask Jiv *"why is my site down?"* - it checks the real status.
2. **Restart** the site (Overview > Restart).
3. If it started after a deploy, **roll back** to the previous deploy.
4. Still failing? Ask *"restart the stuck services on my server"* - the proxy or Docker may be stuck.

=== error-404-page
title: "404 Not Found" on some pages
q: 404 not found | page not found after deploy | refresh gives 404 | react router 404 | links broken 404
tags: 404 not found page missing spa react router refresh case sensitive
hi: 404 error aata hai
go: Redirect rules -> #/projects
screen: rules
---
- **Only on refresh / deep links of a React/Vue app**: add a single-page-app rule in the project's Rules: `/*  /index.html  200`.
- **A specific file**: online file names are **case-sensitive** (*Logo.PNG* is not *logo.png*). Check the name and folder.
- **Everything 404**: the upload probably has its files one folder too deep. Drop the folder again (Rover finds the right level).

=== error-403
title: "403 Forbidden" on the site
q: 403 forbidden on my website | access denied page | forbidden you don't have permission
tags: 403 forbidden access denied permission password index
hi: 403 forbidden
go: Project settings -> #/projects
screen: project-settings
---
- The folder has **no index.html**, so the server refuses to list it. Make sure the home page is called index.html.
- The site is **password-protected** and the login was cancelled - see Project > Settings > Access.
- On a home server or this PC, Windows/Linux file permissions may block the files; publish again from Rover.

=== error-mixed-content
title: Padlock missing because of "mixed content"
q: mixed content warning | not fully secure | http images on https page
tags: mixed content http insecure images scripts
hi: not secure dikh raha hai
go: App Testing -> #/testing
screen: testing
---
Your page loads some images, fonts or scripts with **http://** addresses. Change them to **https://** (or start them with `/` for your own files). App Testing's Check report lists the exact lines.

=== error-cert-expired
title: "Your connection is not private" / certificate expired
q: your connection is not private | net err cert date invalid | certificate has expired | err cert authority invalid
tags: expired not private cert date invalid authority
hi: connection is not private
go: Domain tab -> #/projects
screen: domain
---
- **Just connected the domain?** Wait 1-5 minutes while the certificate is issued.
- **Worked before, now expired**: ask Jiv *"renew the SSL for bakery"*. It checks the certificate and restarts the proxy so it renews.
- Renewal fails when the domain doesn't point to the server, port 80 is blocked, or an **AAAA** record points elsewhere. Ask Jiv to *diagnose the domain*.

=== error-aaaa-record
title: An AAAA (IPv6) record points somewhere else
q: aaaa record wrong | ipv6 record points elsewhere | works on wifi not on mobile | delete aaaa record
tags: aaaa ipv6 record dns wrong mobile parked
hi: aaaa record hatao
go: Domains -> #/domains
screen: domain
---
Many registrars add an **AAAA** record (IPv6) for "parked" pages. Phones on mobile data often use IPv6, so they see the parked page while your PC sees your site. Delete the AAAA record for `@` and `www` at your domain company (unless it's your server's real IPv6 address).

=== error-cloudflare-526
title: Cloudflare errors 521, 522, 525, 526
q: cloudflare 521 | cloudflare 522 connection timed out | cloudflare error 525 | cloudflare error 526 | web server is down cloudflare
tags: cloudflare 521 522 525 526 handshake origin orange
hi: cloudflare error 522
go: Domains -> #/domains
screen: domains
---
- **521/522**: Cloudflare can't reach your server. Check it's running and ports 80/443 are open (Oracle Cloud: open them in the security list too).
- **525/526**: the server's certificate isn't ready. Turn the cloud **grey (DNS only)** for a few minutes so the server can get its certificate, then turn it orange again and set SSL/TLS to **Full (strict)**.

=== error-cf-token
title: Cloudflare token doesn't work
q: cloudflare token invalid | cloudflare api token error | zone not found cloudflare | token needs permission
tags: cloudflare token permission zone dns edit invalid api
hi: cloudflare token galat
go: Domains -> #/domains
screen: domains
---
Create the token from Cloudflare's **Edit zone DNS** template, choose **All zones** (or your domain) under *Zone Resources*, then paste it in Rover's Domains page. "Zone not found" means the domain isn't in that Cloudflare account yet - add the site in Cloudflare first and change the nameservers at your registrar. The token is stored encrypted on this PC and never sent to any AI.

=== error-port-in-use
title: "Address already in use" (This PC hosting)
q: address already in use | port 8080 is busy | only one usage of each socket address | port already used
tags: port busy in use 8080 socket this pc conflict
hi: port busy hai
go: This PC -> #/thispc
screen: thispc
---
Another program (Skype, a dev server, another web server) uses the port. Open **This PC** and choose another port such as **8081**, then **Save**. With the Cloudflare tunnel the port number doesn't matter to visitors.

=== error-tunnel-down
title: Cloudflare tunnel disconnected
q: tunnel disconnected | cloudflared not connected | home server tunnel down | tunnel offline
tags: tunnel cloudflared disconnected offline home server this pc restart
hi: tunnel band hai
go: This PC -> #/thispc
screen: thispc
---
The tunnel needs the PC or home server to be on and online. Check the internet connection, then press **Reconnect** (This PC) or ask Jiv *"restart the tunnel"* (home server). If the token was deleted in Cloudflare, connect the tunnel again from the same page.

=== error-sudo
title: "sudo: a password is required"
q: sudo a password is required | sudo password needed | not in sudoers | permission denied sudo
tags: sudo password sudoers root user permission ubuntu
hi: sudo password maang raha hai
go: Server settings -> #/settings/server
screen: server
---
The login user can't run admin commands without typing a password. Use the **root** user, or (Ubuntu/Oracle) the default **ubuntu** user, which has password-less sudo. On a home server, run `sudo visudo` once and add `yourname ALL=(ALL) NOPASSWD:ALL`, or connect as root.

=== error-coolify-down
title: The hosting engine (Coolify) isn't answering
q: coolify not responding | coolify api unreachable | hosting engine offline | port 8000 not answering
tags: coolify engine down api 8000 restart docker unreachable
hi: coolify nahi chal raha
go: Server settings -> #/settings/server
screen: server
---
Ask Jiv *"restart coolify"* (⚠ confirm) - it restarts the engine and checks it comes back. If the server rebooted, wait 2 minutes. Still down? Check disk space (*"free disk space on my server"*) - a full disk stops Coolify.

=== error-oom
title: The server ran out of memory
q: out of memory error | server keeps crashing | oom killer | cannot allocate
tags: oom killed crash swap
hi: memory khatam
go: Hosting accounts -> #/hosting
screen: server
---
Small servers (1 GB) can run out of memory during deploys. Rover's setup adds swap space, which helps. For many sites choose a 4 GB server; on Oracle's free tier use the Ampere A1 shape (up to 24 GB). Restart stuck services afterwards (ask Jiv).

=== error-brain-start
title: The offline brain doesn't start
q: brain failed to start | llama server exited | offline brain error | jiv brain not loading
tags: brain start error llama server exited crash memory antivirus
hi: brain start nahi ho raha
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
- **Not enough free memory**: close big apps, or switch to the **Light** model.
- **Graphics card**: if it failed on the GPU, Jiv falls back to the processor by itself. Update the graphics driver to use the card again.
- **Antivirus** may block llama-server.exe the first time: allow it (it's the open-source llama.cpp, MIT licence).
- Press **Restart** in Settings > Jiv. Jiv's built-in answers keep working meanwhile.

=== error-brain-download
title: Brain download stops or is very slow
q: brain download failed | model download stuck | download keeps failing | huggingface blocked
tags: download failed slow resume model huggingface github blocked office network
hi: download ruk gaya
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Downloads resume where they stopped - just press Download again. Office networks sometimes block huggingface.co or github.com: use the **Rover Offline (full)** installer or an **offline pack** from a USB stick instead, or set your company mirror in Settings > Jiv.

=== error-pack-checksum
title: "Damaged" or "unknown" offline pack
q: offline pack damaged | checksum mismatch pack | pack part missing | not a rover offline pack
tags: pack checksum damaged part missing dlpack import usb
hi: pack kharab hai
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Copy the pack again: USB sticks and interrupted downloads can damage big files. For split packs keep **every part** (.dlpack.001, .002...) in one folder. Files that aren't on Rover's built-in list are always refused - that's a safety feature.

=== error-zip-invalid
title: "Not a valid zip" when uploading
q: zip is not valid | cannot open zip | zip file corrupted | rar file not supported
tags: zip invalid corrupt rar 7z archive upload
hi: zip kharab hai
go: Add new project -> #/new
screen: new
---
Make the zip again with Windows (right-click the folder > **Send to > Compressed (zipped) folder**), or simply drop the **folder** itself - Rover zips it for you. RAR and 7z files aren't supported.

=== error-oracle-ports
title: Oracle Cloud: site doesn't open although everything looks right
q: oracle cloud site not opening | oracle port 80 blocked | oracle security list ingress | oracle free vps not reachable
tags: oracle cloud security list ingress port 80 443 iptables firewall
hi: oracle par site nahi khul rahi
go: Hosting accounts -> #/hosting
screen: hosting
---
Oracle blocks web traffic in **two** places:

1. **Security list** in the Oracle console: Networking > Virtual cloud networks > your VCN > Security lists > Default > **Add ingress rules**: source `0.0.0.0/0`, TCP, destination ports **80,443**.
2. **The server's own firewall** (iptables). Rover's setup opens 80 and 443 automatically; run *Set up automatically* again if the server was set up elsewhere.

Ask *"show me how to add an Oracle server"* for the full walkthrough.

=== error-windows-firewall
title: Windows Firewall asks about Rover
q: windows firewall popup | allow rover through firewall | windows defender firewall rover | lan access blocked
tags: windows firewall defender allow private network lan this pc
hi: windows firewall poochh raha hai
go: This PC -> #/thispc
screen: thispc
---
Windows asks once when Rover's web server starts. Allow **Private networks** if you want other devices at home to open your sites; visitors from the internet come through the Cloudflare tunnel and don't need this. You can change it later in Windows Security > Firewall.

=== error-smartscreen
title: "Windows protected your PC" when installing
q: windows protected your pc | smartscreen warning | unknown publisher rover | antivirus flags rover
tags: smartscreen unknown publisher warning install antivirus signed
hi: windows protected your pc
go: Updates -> #/settings/updates
screen: updates
---
New apps without a paid code-signing certificate show this. Click **More info > Run anyway**. Check the file's SHA-256 against the one on the release page if you want to be sure. Updates inside Rover are verified with a signature before they install.
