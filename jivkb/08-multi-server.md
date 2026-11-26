=== failover-backup
title: Keep a site online if a server goes down (backup server)
q: keep my site online if the server goes down | add a backup server | run on two servers | multiple servers at the same time | failover | high availability | what if my server crashes | site on 2 servers | redundancy | uptime
tags: backup failover redundancy uptime high availability two servers replica primary standby switch
hi: server band ho jaye to site chalti rahe | do server par site | backup server kaise lagaye | बैकअप सर्वर
go: Projects -> #/projects
screen: project-hosting
---
A project can run on **two or more servers at once**. Open the project > **Hosting** > **Add a backup server**.

1. Pick the other server: a rented VPS, a **machine at home** or **This PC**.
2. Rover sets the project up there and sends the live version.
3. From then on **every deploy goes to all servers**. Each server switches only when it has all the files, so all of them serve the same version.

Two ways visitors find a working server:
- **Primary + backup** (recommended): visitors go to the main server. Rover checks every server each minute; after 3 failed checks it points your domain at the backup through **Cloudflare** (live in seconds) and tells you. When the main server is healthy again (3 good checks) it switches back, if you leave that on. Needs Rover running.
- **Together**: all servers answer at once through one **Cloudflare tunnel**; Cloudflare skips a server that is off, even while your PC is off.

Automatic switching needs the domain on **Cloudflare** (free plan). Without it Rover still checks and tells you when to switch; you then change the A record at your registrar. **Switch now** in the Hosting tab switches by hand.
@hi
प्रोजेक्ट के **Hosting** टैब में **Add server** दबाकर दूसरा सर्वर (या This PC) बैकअप के रूप में जोड़ें। मुख्य सर्वर बंद होने पर Rover DNS बदलकर बैकअप पर भेज देता है। Jiv से कहें: *"bakery ka backup mere PC par rakho"*।

=== partial-deploy
title: A deploy reached only some servers (Retry)
q: deploy failed on one server | live on some servers | backup server has the old version | retry server | servers out of sync | different version on servers | sync check | send live version again
tags: partial retry sync servers failed backup replica version mismatch
hi: ek server par deploy nahi hua | retry kaise kare | सर्वर पर नया वर्जन नहीं
go: Projects -> #/projects
screen: project-hosting
---
If a server is off or unreachable during a deploy, the deploy shows **live on some servers**. The servers that received everything are already on the new version; the others keep the old one.

1. Open the project > **Hosting**.
2. The server shows **Not updated** with the reason. When it is reachable again click **Retry** (or **Send live version to all**).
3. Rover sends the live version from the copy saved on this PC, with the same version name, and switches that server.

**Check health & sync** asks every server which version it serves and marks each one **in sync** or not. A rollback is applied on every server.

=== together-mode
title: Together mode (Cloudflare tunnel on every server)
q: together mode | active active | cloudflare tunnel replicas | load balancing | both servers at once | serve from all servers | cloudflare load balancer price
tags: together active tunnel cloudflared replicas load balancing cloudflare
hi: dono server ek saath | cloudflare tunnel kya hai
go: Projects -> #/projects
screen: project-hosting
---
In **Together** mode Rover creates one **Cloudflare Tunnel** for the project, starts the small `cloudflared` program on every server and points the domain at the tunnel. Cloudflare sends each visitor to a server that is connected and tries another if one fails. This keeps working while your PC is off.

- Needs the domain on Cloudflare (free) and a token with **Cloudflare Tunnel: Edit** and **DNS: Edit**.
- This PC is not used in this mode (use Primary + backup to include it).
- If a server is on but its web server is broken, Cloudflare may still send visitors there: Rover's health checks tell you.
- Cloudflare's paid **Load Balancing** ($5/month for 2 servers) also works; Rover's free modes cover most sites.

=== home-server
title: Use your own server at home (old PC, mini PC, Raspberry Pi)
q: use my own server at home | home made server | raspberry pi hosting | host on old laptop linux | home server | self hosted at home | own hardware | mini pc server | lan server
tags: home server raspberry pi own hardware lan linux homelab self hosted
hi: ghar ka server | apna server use karna | raspberry pi par site | घर का सर्वर
go: Hosting accounts -> #/hosting/home
screen: home-server
---
1. Turn on SSH on the machine (Raspberry Pi Imager: *Enable SSH*; Ubuntu: `sudo apt install openssh-server`).
2. In Rover: **Hosting accounts** > **My own server (at home)** > **Add a home server**.
3. Enter its address on your network (for example `192.168.1.50` or `raspberrypi.local`), the user and the password. The password is used once and never saved.
4. Click **Set up the server**. Rover checks memory and processor and picks:
   - **Lightweight** (plain nginx, about 10 MB of memory) for small machines (under 2 GB or 32-bit ARM),
   - **Full** (Coolify) otherwise.
5. Choose how visitors reach it: **Cloudflare tunnel** (recommended, no router changes), **port forwarding** (advanced) or **home network only**.

Then create a project on it, or add it to a project as a backup server. Remember: it must stay on, your upload speed is the site's speed, and power cuts take it down (a UPS helps).
@hi
घर का पुराना लैपटॉप या Raspberry Pi भी सर्वर बन सकता है। Hosting accounts > **Add a home server** में उसका लोकल पता (जैसे 192.168.1.20), यूज़र और पासवर्ड डालें, फिर **Cloudflare tunnel** चुनें।

=== cloudflare-tunnel
title: No public IP, CGNAT or port forwarding problems
q: no public ip | cgnat | port forwarding not working | dynamic ip | ip keeps changing | router port forward | jio airtel home internet hosting | ports 80 443 closed at home | duckdns
tags: tunnel cgnat port forwarding dynamic ip ddns router public ip home internet
hi: public ip nahi hai | port forward kaise kare | ip badalta hai | पोर्ट फॉरवर्ड
go: Hosting accounts -> #/hosting/home
screen: home-server
---
Most home internet (and all mobile data) has no reachable public address (**CGNAT**) and the address changes. The easy fix is a **Cloudflare tunnel**: the server connects out to Cloudflare, so nothing on the router needs changing and a changing IP does not matter.

- Rover installs Cloudflare's official `cloudflared` (fixed version, checksum checked) and starts it as a service.
- Needs your domain on Cloudflare (free) and a token with **Cloudflare Tunnel: Edit**.

**Port forwarding** (advanced): reserve the machine's address in the router, forward TCP 80 and 443 to it, and use a dynamic-DNS name (DuckDNS, No-IP or the router's own). Check the router's WAN address matches whatismyip.com; if it starts with 100.64-100.127 you are behind CGNAT and forwarding cannot work.

=== this-pc
title: Host websites on this PC (Windows)
q: host on this pc | host from my laptop | use my computer as server | windows hosting | serve website from my pc | local web server | start with windows | keep pc awake
tags: this pc windows laptop local host built-in server autostart sleep awake
hi: apne computer se website chalana | laptop ko server banana | इस पीसी पर होस्ट
go: This PC -> #/thispc
screen: thispc
---
Open **Hosting accounts** > **This PC** (or press Ctrl+K and type *This PC*).

1. Tick **Serve websites from this PC** and **Save**. Rover's built-in web server applies the same redirects, headers, password protection and primary domain as your servers.
2. Click **Connect to the internet**: Rover downloads `cloudflared` (official, checksum checked) and opens a Cloudflare tunnel.
3. In a project > **Hosting** > **Add a backup server**, choose **This PC**.

Options: **Start with Windows** (runs in the background), **Keep this PC awake**, and **answer other devices on your home network**. On laptops set *When I close the lid* to *Do nothing*. Best as a backup next to a rented server.
@hi
**This PC** पेज पर **Host on this PC** चालू करें। Rover का अपना वेब सर्वर आपकी साइटें चलाएगा। इंटरनेट से पहुँच के लिए **Connect tunnel** (Cloudflare) दबाएँ - पोर्ट फ़ॉरवर्डिंग की ज़रूरत नहीं। यह बैकअप सर्वर के रूप में सबसे अच्छा है।

=== connection-backup
title: Wi-Fi with phone mobile data as backup
q: wifi down use mobile data | phone hotspot backup | usb tethering backup internet | internet went offline | which network is active | this pc offline alert | switch to mobile data
tags: wifi mobile data hotspot tethering backup internet offline network monitor
hi: wifi band ho to mobile data | hotspot backup | इंटरनेट बंद
go: This PC -> #/thispc
screen: thispc
---
While This PC hosts sites, Rover watches the internet every 30 seconds and shows which connection is used: **Wi-Fi** (with its name), **cable**, **phone over USB** or **mobile data**.

- Plan: home Wi-Fi first, the phone's data as backup (**USB tethering** is the most reliable; a hotspot works too). Windows switches by itself when Wi-Fi drops.
- The Cloudflare tunnel reconnects within seconds on the new connection, so the site keeps working. Port forwarding never works on mobile data.
- Rover notifies you when the PC goes offline, comes back, or moves to the backup connection. Watch your data plan.
