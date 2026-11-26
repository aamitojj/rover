=== guide-mode
title: "Show me how" walkthroughs
q: show me how | guide me step by step | walk me through it | teach me where to click | interactive tutorial | highlight the button
tags: guide walkthrough tour show me how step by step highlight tutorial learn
hi: mujhe dikhao kaise | step by step batao | dikhao kahan click karna hai | मुझे दिखाओ
go: All walkthroughs -> #/guides
screen: any
---
Jiv can **walk you through** any task inside Rover. Ask *"show me how to add a domain"* (or press **Show me how** under an answer) and:

1. Jiv **highlights the real button or field** and explains it in one line.
2. You do the step yourself. Jiv notices (a click, a page change, a filled field or a server check) and moves on by itself.
3. Steps on other websites (Oracle Cloud, GoDaddy, Cloudflare) appear as a **checklist** with a picture of the page and **copy buttons** for the values. When you come back to Rover, Jiv checks the result automatically (for example that your domain now points to the server).

Use **Back**, **Skip** or **Exit** at any time. All walkthroughs work offline; the full list is on the **Walkthroughs** page.
@hi
Jiv आपको Rover के अंदर हर काम **कदम-दर-कदम** दिखा सकता है। पूछें *"domain jodna dikhao"* या जवाब के नीचे **Show me how** दबाएँ:

1. Jiv **असली बटन या फ़ील्ड को हाइलाइट** करता है और एक लाइन में समझाता है।
2. आप कदम खुद करते हैं; Jiv पहचान लेता है और अपने-आप अगले कदम पर जाता है।
3. दूसरी वेबसाइटों (Oracle, GoDaddy, Cloudflare) के कदम **चेकलिस्ट** में तस्वीर और **कॉपी बटन** के साथ आते हैं। वापस आने पर Jiv खुद जाँचता है।

=== do-it-for-me
title: "Do it for me": let Jiv run a multi-step task
q: do it for me | can jiv do it for me | can jiv do it automatically | put my site on my domain with https | set everything up for me | automate the steps | jiv plan
tags: do it for me plan automatic tasks steps run checklist automate agent
hi: mere liye kar do | khud kar do | sab set kar do | मेरे लिए कर दो
go: Open Jiv -> #/jiv
screen: jiv
---
Tell Jiv the result you want in one sentence, for example *"put my bakery site on joesbakery.com with HTTPS and a backup on my home PC"*. Jiv:

1. Turns it into a **plan**: a checklist of steps built from Rover's own actions (save the domain, create the DNS records, wait for DNS, turn on HTTPS, add the backup server...).
2. Shows the plan first. Nothing happens until you press **Run**. Steps marked **⚠** (restarting services, cleaning a server, moving a project, renewing certificates) ask for their own confirm click.
3. Runs the steps and shows the progress. If your domain company has no automatic DNS, Jiv pauses, shows the two records to add with copy buttons and a walkthrough, and continues by itself when they work.
4. If a step fails, Jiv stops, explains why in plain words and offers a fix, **Retry** or **Skip**.

Read-only checks (logs, DNS, "why did my deploy fail?") run straight away because they change nothing.
@hi
Jiv को एक वाक्य में बताइए, जैसे *"bakery site ko joesbakery.com par HTTPS ke saath daalo aur ghar ke PC par backup rakho"*। Jiv:

1. इसे एक **प्लान** (चेकलिस्ट) में बदलता है।
2. पहले प्लान दिखाता है - **Run** दबाने तक कुछ नहीं होता। **⚠** वाले कदम अलग से पुष्टि माँगते हैं।
3. कदम चलाता है और प्रगति दिखाता है।
4. कोई कदम फ़ेल हो तो रुककर कारण और समाधान बताता है (**Retry** / **Skip**)।

=== plan-safety
title: How Jiv keeps "do it for me" safe
q: is it safe to let jiv change my server | will jiv delete anything | why does jiv ask to confirm | risky steps warning | can jiv run commands on my server
tags: safety confirm risky plan steps server changes delete permission
hi: kya jiv safe hai | jiv kuch delete karega kya
go: Open Jiv -> #/jiv
screen: jiv
---
- Jiv can only use **Rover's own actions** (the same ones behind the buttons). It never types passwords on websites and never sends your tokens to any AI.
- A plan runs only after you press **Run**. Risky steps (⚠) ask again just before they run, with a one-time confirm.
- Cleaning a server only removes **unused** Docker images, old build cache, old logs and downloaded packages. Your sites, databases and uploads are never touched.
- Changing web server settings is tested first (nginx -t); if the test fails the old settings stay.
- Every deploy is kept, so anything published can be rolled back from the Deploys tab.

=== offline-pack
title: Offline packs: install Jiv's brain without internet
q: offline pack | install the brain from usb | dlpack file | import offline pack | no internet install model | rover offline full installer | copy the brain to another pc
tags: offline pack usb dlpack import export installer full no internet model brain
hi: bina internet ke brain kaise daale | usb se install | ऑफ़लाइन पैक
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Everything Jiv needs can come from a file instead of the internet:

- **Rover Offline (full) installer** already contains the Light brain and the llama.cpp engines. Install it and Jiv's brain is ready on first start.
- **Offline pack (.dlpack)**: in **Settings > Jiv > Offline pack** click **Import offline pack** and choose the file, its folder or your USB stick. Big packs come in parts (*.dlpack.001*, *.dlpack.002*) - keep all parts in the same folder and choose any of them.
- **Export** writes the brain you have to a USB stick for another PC.

Rover checks every file against fingerprints (SHA-256) built into the app, so a damaged or altered file is refused. The knowledge base and all walkthroughs are always inside the app - no download needed.
@hi
Jiv को जो चाहिए वह इंटरनेट के बिना भी आ सकता है:

- **Rover Offline (full) इंस्टॉलर** में Light ब्रेन पहले से है।
- **ऑफ़लाइन पैक (.dlpack)**: **Settings > Jiv > Offline pack** में **Import offline pack** दबाएँ और फ़ाइल या USB चुनें। बड़े पैक हिस्सों (.001, .002) में आते हैं - सब एक ही फ़ोल्डर में रखें।
- **Export** से ब्रेन USB पर कॉपी करें।

हर फ़ाइल की SHA-256 जाँच होती है; बदली हुई फ़ाइल स्वीकार नहीं होती।

=== gpu-brain
title: Using your graphics card (GPU) for Jiv's brain
q: use my graphics card | gpu for jiv | nvidia gtx | does the brain use the gpu | vulkan | make the brain faster | 4 gb graphics card
tags: gpu graphics card nvidia amd intel vulkan vram faster speed gtx rtx radeon
hi: graphics card use karo | gpu se fast | ग्राफ़िक्स कार्ड
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
Rover detects your graphics card (NVIDIA, AMD or Intel Arc) and shows it in **Settings > Jiv > Graphics card**.

- With a card that has **4 GB or more** (for example a GTX 1650), Rover recommends the **Standard** model on the card using llama.cpp's **Vulkan** engine. Whatever doesn't fit in the card's memory stays in normal RAM automatically.
- With **2-4 GB**, the **Light** model runs on the card.
- If the card can't load the model (old driver, too little memory), Jiv switches to the processor by itself and tells you. Updating the graphics driver usually fixes it.

The Vulkan engine is 38 MB and works with all vendors; no CUDA download is needed. Laptop tip: plug in the charger - on battery many laptops slow the graphics card down.
@hi
Rover आपका ग्राफ़िक्स कार्ड (NVIDIA, AMD, Intel Arc) पहचानता है। **4 GB या ज़्यादा** (जैसे GTX 1650) हो तो Standard मॉडल कार्ड पर चलता है (Vulkan)। कार्ड पर न चले तो Jiv खुद प्रोसेसर पर चला जाता है। ड्राइवर अपडेट रखें और लैपटॉप चार्जर लगाकर रखें।

=== jiv-free-disk
title: Free disk space on a server (Jiv can do it)
q: free disk space on my server | clean up the server | remove old docker images | make space on vps | what is using my disk
tags: disk space clean cleanup docker prune storage free logs
hi: server ki jagah khali karo | disk saaf karo | डिस्क खाली करो
go: Open Jiv -> #/jiv
screen: server
---
Ask Jiv *"free disk space on my server"*. It first shows **what fills the disk** (read-only), then offers a cleanup step (⚠ asks to confirm) that:

- removes **unused** Docker images and build cache older than 3 days,
- shrinks the system logs to 200 MB,
- clears downloaded update packages and old compressed logs.

Running sites, databases and your uploads are never touched. You'll see how much was freed. If the disk fills up again quickly, a bigger disk (or fewer old deploys) is the lasting fix.
@hi
Jiv से पूछें *"server ki disk khali karo"*। पहले वह दिखाता है कि जगह किसने भरी है, फिर पुष्टि के बाद पुराने Docker images, build cache, पुराने लॉग और पैकेज हटाता है। चल रही साइटें और डेटा कभी नहीं छुए जाते।

=== jiv-restart-services
title: Restart stuck services (proxy, nginx, Coolify, tunnel)
q: restart stuck services | site stuck restart the proxy | restart nginx | restart coolify | restart docker | cloudflare tunnel stuck | server not responding services
tags: restart stuck service proxy traefik nginx coolify docker tunnel hung frozen
hi: service restart karo | atak gaya | सर्विस रीस्टार्ट
go: Open Jiv -> #/jiv
screen: server
---
Ask Jiv *"restart the stuck services on my server"*. Jiv first checks which services are stopped, restarting or unhealthy (read-only), then restarts **only those** after your confirm click and checks they came back.

You can also name one: *"restart the proxy"* (HTTPS front door), *"restart nginx"*, *"restart coolify"*, *"restart docker"* or *"restart the tunnel"*. Restarting the proxy or Docker interrupts sites for a few seconds.

If a service doesn't come back, Jiv says which one and suggests reading its log.
@hi
Jiv से कहें *"atki hui services restart karo"*। Jiv पहले देखता है कौन-सी सर्विस रुकी है, फिर पुष्टि के बाद सिर्फ़ उन्हें रीस्टार्ट करता है और जाँचता है कि वे वापस चलीं।

=== jiv-renew-ssl
title: Renew or fix an HTTPS certificate
q: renew ssl certificate | certificate expired | renew https | ssl expiring soon | fix certificate not renewing
tags: ssl certificate renew expired https lets encrypt expiring padlock
hi: ssl renew karo | certificate expire ho gaya | सर्टिफ़िकेट
go: Backups & alerts -> #/settings/backups
screen: domain
---
Certificates from Let's Encrypt renew by themselves about 30 days before they expire. Ask Jiv *"renew the SSL for bakery"* and it:

1. Checks the certificate (days left). If it's fine, nothing is changed.
2. If it's expired, expiring within 3 weeks or broken, restarts the web proxy (⚠ confirm) so it requests a fresh certificate, then checks again.

Renewal needs the domain to point to the server and port 80 to be open. Sites behind a Cloudflare tunnel get their certificate from Cloudflare instead (SSL/TLS > Edge Certificates).
@hi
Let's Encrypt सर्टिफ़िकेट अपने-आप रिन्यू होते हैं। Jiv से कहें *"bakery ka ssl renew karo"* - वह पहले जाँचेगा; ज़रूरत हो तो पुष्टि के बाद proxy रीस्टार्ट करके नया सर्टिफ़िकेट लेगा।

=== jiv-deploy-failed
title: Find why a deploy failed and fix it
q: why did my deploy fail | find why the deploy failed and fix it | deploy keeps failing | publish failed what now | fix failed deploy
tags: deploy failed why fix retry error publish broken
hi: deploy fail kyun hua | deploy fail ho gaya | डिप्लॉय फ़ेल क्यों
go: Deploys -> #/projects
screen: deploys
---
Ask Jiv *"why did bakery's deploy fail?"*. Jiv reads the failed deploy (read-only) and explains it in plain words. Then it adds the fix to a plan:

- **Network or timeout**: publish the same files again.
- **Server disk full**: free disk space, then publish again.
- **A _redirects/_headers line was refused**: Jiv shows which line; the live site was not changed.
- **No index.html / needs a build**: Jiv shows how to fix the files.

Your live site keeps running on the last good deploy while you fix it.
@hi
Jiv से पूछें *"bakery ka deploy fail kyun hua?"*। Jiv फ़ेल हुआ डिप्लॉय पढ़कर कारण बताता है और समाधान प्लान में जोड़ता है (फिर से पब्लिश, डिस्क खाली करना आदि)। तब तक आपकी साइट पिछले अच्छे वर्ज़न पर चलती रहती है।

=== jiv-move-project
title: Move (migrate) a project to another server
q: move my project to another server | migrate site to new server | transfer project to hetzner | change the server of a project
tags: move migrate transfer project server change new vps
hi: project dusre server par le jao | migrate karo
go: Project > Hosting -> #/projects
screen: project-hosting
---
Ask Jiv *"move bakery to my Hetzner server"*, or open the project's **Hosting** tab and choose **Move**. Rover creates the project on the new server and publishes the live version there (⚠ confirm).

Afterwards point your domain to the new server's IP (Jiv's domain walkthrough shows how; with Cloudflare connected it is automatic), check the site, and remove the old copy. Your deploy history stays on this PC.

=== jiv-explain-logs
title: Jiv reads and explains logs
q: explain my logs | what do these log lines mean | read the error log for me | logs make no sense
tags: logs explain read error lines nginx meaning
hi: log samjhao | log ka matlab
go: Logs -> #/projects
screen: logs
---
Ask *"explain bakery's logs"*. Jiv fetches the latest log lines from the server (read-only), picks the important ones (errors, refused connections, missing files) and explains each in one sentence, with a link to the fix. Normal visitor lines are skipped.

=== jiv-dns-diagnose
title: Diagnose DNS problems with Jiv
q: diagnose my dns | check my domain dns | why does my domain not open | dns problem check | is my domain pointing right
tags: dns diagnose check domain records a record nameserver propagation
hi: dns check karo | domain kyun nahi khul raha
go: Domains -> #/domains
screen: domain
---
Ask *"diagnose bakery's domain"*. Jiv checks, read-only: the domain's nameservers (who hosts its DNS), the A records (do they point to your server?), extra AAAA (IPv6) records that point elsewhere, and whether HTTPS answers. Each problem comes with the exact fix and, for GoDaddy, Cloudflare and others, a walkthrough.

=== guides-library
title: Walkthroughs list
q: list of walkthroughs | all tutorials | which guides are there | what can you show me
tags: guides walkthroughs list tutorials library
hi: kaun kaun se guide hai
go: All walkthroughs -> #/guides
screen: guides
---
The **Walkthroughs** page lists every guided task: add a server (Oracle Cloud, Hetzner, any VPS, a home server), set up the server, add a project, deploy by drop, restore an old deploy, read logs, password-protect a site, add a domain (GoDaddy, Cloudflare, nameservers), turn on HTTPS, add a backup server, host on this PC, update the app, connect AI and install an offline pack. Press one to start; you can leave at any time.
