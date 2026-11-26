=== basics-hosting
title: What is web hosting?
q: what is web hosting | what is website hosting | web hosting explained | what is hosting | what is a server | what is a vps | where does my website live | hosting meaning
tags: hosting web hosting meaning definition explained basics beginner server vps computer internet
hi: hosting kya hai | server kya hota hai | होस्टिंग क्या है
go: Hosting accounts -> #/hosting
screen: any
---
A website is a folder of files. **Hosting** means a computer that is always on and connected to the internet serves those files to visitors. A **VPS** (virtual private server) is your own rented slice of such a computer, from about ₹350-₹1,000 a month (Oracle Cloud has a free one). Rover installs everything on it and publishes your folders - you never have to log in to it yourself. You can also host on **this PC** or a **home server**.
@hi
वेबसाइट फ़ाइलों का एक फ़ोल्डर है। **होस्टिंग** का मतलब है एक हमेशा चालू कंप्यूटर जो ये फ़ाइलें विज़िटर्स को दिखाता है। **VPS** उसी तरह का आपका किराए का सर्वर है (Oracle Cloud पर मुफ़्त भी)। Rover उस पर सब कुछ इंस्टॉल करता है - आपको खुद लॉग-इन नहीं करना पड़ता।

=== basics-domain-vs-hosting
title: Domain vs hosting: what's the difference?
q: difference between domain and hosting | do i need both domain and hosting | what is a domain name
tags: domain hosting difference registrar name address basics
hi: domain aur hosting me kya fark hai | डोमेन और होस्टिंग
go: Domains -> #/domains
screen: any
---
- The **domain** (joesbakery.com) is the *name*. You rent it yearly from a registrar such as GoDaddy, Namecheap, Hostinger or Cloudflare.
- The **hosting** (your server) is the *place* where the files live.
- **DNS** connects them: an *A record* says "joesbakery.com lives at 203.0.113.10".

You don't need a domain to start: every Rover project gets a free https address immediately.
@hi
**डोमेन** (joesbakery.com) *नाम* है, जो आप GoDaddy जैसी कंपनी से सालाना लेते हैं। **होस्टिंग** वह *जगह* है जहाँ फ़ाइलें रहती हैं। **DNS** दोनों को जोड़ता है (A रिकॉर्ड)। शुरू करने के लिए डोमेन ज़रूरी नहीं - हर प्रोजेक्ट को मुफ़्त https पता मिलता है।

=== basics-dns-records
title: DNS records explained (A, AAAA, CNAME, MX, TXT)
q: what is an a record | what is a cname | dns record types | what is mx record | txt record meaning
tags: dns records a aaaa cname mx txt types explained basics
hi: a record kya hai | dns record kya hota hai | डीएनएस रिकॉर्ड
go: Domains -> #/domains
screen: domain
---
- **A**: name → IPv4 address of your server (what Rover needs).
- **AAAA**: name → IPv6 address. Delete stray ones that point to parked pages.
- **CNAME**: name → another name (e.g. www → joesbakery.com). Not allowed on the bare domain at most registrars.
- **MX**: where your **email** goes. Never delete these when moving a website.
- **TXT**: proofs and email settings (SPF, verification codes).

*Name* `@` means the bare domain; `www` means www.joesbakery.com.
@hi
- **A**: नाम → सर्वर का IP (Rover को यही चाहिए)।
- **AAAA**: IPv6 पता; पार्क्ड पेज वाले हटा दें।
- **CNAME**: नाम → दूसरा नाम।
- **MX**: ईमेल - इन्हें कभी न हटाएँ।
- **TXT**: वेरिफ़िकेशन और ईमेल सेटिंग।

=== basics-propagation
title: How long do DNS changes take?
q: how long does dns take | dns propagation time | ttl meaning | changed dns still old
tags: dns propagation ttl time wait cache minutes hours
hi: dns update hone me kitna time | कितना समय
go: Domains -> #/domains
screen: domain
---
Usually **5-30 minutes**, sometimes up to a few hours. The **TTL** (time to live) is how long others may remember the old value - choose the lowest offered (or Auto) before a change. Nameserver changes can take up to 24 hours. Jiv's plans wait and check by themselves, so you can close the window.
@hi
आमतौर पर **5-30 मिनट**, कभी-कभी कुछ घंटे। nameserver बदलने में 24 घंटे तक लग सकते हैं। Jiv अपने-आप जाँचता रहता है।

=== basics-nameservers
title: What are nameservers?
q: what are nameservers | should i change nameservers | nameserver vs dns records | where is my dns hosted
tags: nameservers ns dns host registrar change
hi: nameserver kya hai
go: Domains -> #/domains
screen: domain
---
Nameservers decide **which company holds your DNS records**. By default it's your registrar (GoDaddy: *domaincontrol.com*). If you move them to Cloudflare, you edit records in Cloudflare instead - and Rover can then create them automatically. Changing nameservers doesn't move your domain or your email, but copy any MX/TXT records across.

=== basics-https
title: HTTPS and certificates in plain words
q: what is https | what is an ssl certificate | why do i need https | is https free
tags: https ssl tls certificate padlock free lets encrypt secure basics
hi: https kya hai | ssl kya hai | एचटीटीपीएस
go: Domains -> #/domains
screen: domain
---
**HTTPS** encrypts the connection and shows the padlock; browsers warn visitors without it and Google ranks it higher. It needs a **certificate** proving you own the domain. Rover gets one free from **Let's Encrypt** automatically when the domain points to your server, and it renews every ~60 days by itself.
@hi
**HTTPS** कनेक्शन को सुरक्षित करता है और ताला (padlock) दिखाता है। इसके लिए **सर्टिफ़िकेट** चाहिए - Rover इसे **Let's Encrypt** से मुफ़्त और अपने-आप लेता और रिन्यू करता है।

=== basics-ssh
title: What is SSH?
q: what is ssh | ssh key vs password | do i need putty | how does rover log in to my server
tags: ssh key password putty login terminal secure shell
hi: ssh kya hai
go: Server settings -> #/settings/server
screen: server
---
**SSH** is the secure way to control a server remotely. Rover uses it for you: you type the server password **once**, Rover installs its own key and forgets the password. The key is stored encrypted (DPAPI) on this PC. You don't need PuTTY or a terminal.

=== basics-linux
title: Linux basics for website owners
q: basic linux commands | how to check disk space linux | how to see running services | how to reboot server linux
tags: linux commands terminal df systemctl docker reboot basics ubuntu
hi: linux command kaise chalaye
go: Server settings -> #/settings/server
screen: server
---
You rarely need these - Jiv runs them for you - but for reference:

- `df -h` - disk space · `free -h` - memory · `uptime` - load
- `systemctl status nginx` - is a service running · `sudo systemctl restart nginx`
- `docker ps` - running containers · `docker system df` - Docker disk use
- `sudo journalctl -u nginx -n 50` - last log lines of a service
- `sudo reboot` - restart the server (sites are down 1-2 minutes)

=== basics-ports
title: Ports and firewalls (22, 80, 443)
q: what ports need to be open | what is port 80 and 443 | port 22 ssh | open ports on my server
tags: ports firewall 22 80 443 open blocked security list ufw
hi: kaun se port khulne chahiye
go: Hosting accounts -> #/hosting
screen: server
---
- **22**: SSH, how Rover controls the server.
- **80**: plain web traffic and certificate checks (must stay open for HTTPS renewals).
- **443**: HTTPS.

Rover opens them on the server's own firewall. Some providers also have a cloud firewall (Oracle *security list*, AWS *security group*, Hetzner *firewall*) where you must allow 80 and 443 yourself - the walkthroughs show where.

=== basics-static
title: Which websites can Rover host?
q: can rover host wordpress | static vs dynamic site | does it support php | react app hosting
tags: static dynamic wordpress php react vue html site types support
hi: kaun si website host kar sakte hai
go: Add new project -> #/new
screen: new
---
Rover publishes **static sites**: HTML/CSS/JS, exports from site builders, and React/Vue/Vite/Astro/Next (static export) apps - it can build them for you. Forms can use a service such as Formspree. Server-side apps (WordPress, PHP, databases) aren't managed by Rover's screens, but the same server (Coolify) can run them from its own dashboard.

=== basics-vps-size
title: How much RAM and disk does my server (VPS) need?
q: how much ram does my server need | which vps size | how many sites per server | 1gb vs 2gb vps
tags: vps size ram memory need cpu disk plan sites capacity how much
hi: kitni ram chahiye server me
go: Hosting accounts -> #/hosting
screen: hosting
---
- **1-2 GB RAM**: a few small sites (swap is added automatically).
- **4 GB**: dozens of static sites and building React projects on the server.
- **Disk**: 20-40 GB is plenty for static sites; old Docker images are cleaned up when you ask Jiv.

Oracle's free Ampere A1 (up to 4 cores, 24 GB) is generous if you can get one; Hetzner CX22 is great value.

=== basics-ip
title: IP addresses, IPv4, IPv6 and CGNAT
q: what is my server ip | ipv4 vs ipv6 | what is cgnat | static ip needed
tags: ip address ipv4 ipv6 cgnat static public home internet
hi: ip address kya hai
go: Server settings -> #/settings/server
screen: server
---
Your server's **public IPv4** address (like 203.0.113.10) is what the A record points to; it's in Settings > Server. Home internet often has **no public IP** (CGNAT) or a changing one - that's why home servers and this PC use a **Cloudflare tunnel**, which needs no port forwarding or static IP.

=== basics-cdn
title: Cloudflare proxy (orange cloud) and CDN
q: what is the orange cloud | should i proxy through cloudflare | what is a cdn | dns only vs proxied
tags: cloudflare proxy orange grey cloud cdn cache speed ddos
hi: orange cloud kya hai
go: Domains -> #/domains
screen: domains
---
With the **orange cloud** Cloudflare sits in front of your server: it caches files, hides your IP and blocks attacks. With the **grey cloud** (DNS only) visitors go straight to your server. Rover creates records **grey** first so the certificate can be issued; you may switch to orange afterwards with SSL/TLS set to **Full (strict)**.

=== basics-www
title: www or no www?
q: should i use www | www vs non www | www not working
tags: www apex bare cname
hi: www lagana chahiye kya
go: Domain tab -> #/projects
screen: domain
---
Both work: Rover adds `www` automatically with a new domain (two A records). Pick one **Primary address** in the Domain tab and the other redirects to it. If only www fails, its record is missing or still points to the old host.

=== basics-email
title: Email with your domain
q: email on my domain | can rover host email | business email setup | will moving my site break email
tags: email mx zoho google workspace outlook domain mail
hi: domain par email
go: Domains -> #/domains
screen: domain
---
Rover hosts websites, not mailboxes. Use Zoho Mail (free plan), Google Workspace or Microsoft 365 and add the **MX** and **TXT** records they give you. Moving your website only changes **A** records, so email keeps working - never delete MX records.
