# Hosting at home with Rover

Rover 0.7 can run your sites on machines you own: an old PC or laptop with Linux, a mini PC,
a Raspberry Pi, or this Windows PC. Use them on their own for small sites, or as a **backup**
next to a rented server so your site stays up if one goes down.

## Before you start (plain trade-offs)

* The machine must stay **on and awake**. Sleep, restarts for updates and power cuts take the site
  down (unless another server is a backup).
* Your **upload speed** is the site's speed. 10 Mbit/s upload serves a few visitors at once well.
* **Power and internet cuts** take the site down. A small UPS for the router and the machine helps.
* Best use: **backup** next to a rented server, small personal sites, testing.

## A Linux machine at home

1. Install a 64-bit Linux (Ubuntu Server, Debian, or Raspberry Pi OS 64-bit) and turn on SSH.
2. Rover → **Hosting accounts** → **My own server (at home)** → **Add a home server**.
3. Enter its address (e.g. `192.168.1.50` or `raspberrypi.local`), user and password. The password
   is used once to install Rover's own key; it is never saved.
4. **Set up the server.** Rover chooses:
   * **Lightweight** (plain nginx) if the machine has less than ~2 GB of memory or a 32-bit OS;
   * **Full** (Coolify) otherwise.
5. **How visitors reach it:**
   * **Cloudflare tunnel** (recommended): no router changes, works behind CGNAT, changing IPs and
     mobile data. Needs your domain on Cloudflare (free) and a Cloudflare token with
     *Cloudflare Tunnel: Edit* and *DNS: Edit*.
   * **Port forwarding** (advanced): reserve the machine's IP in the router, forward TCP 80 and 443,
     and use a dynamic-DNS name. Does not work behind CGNAT or on mobile data.
   * **Home network only**.

## This PC (Windows)

1. **Hosting accounts** → **This PC** → tick **Serve websites from this PC** → **Save**.
2. **Connect to the internet** downloads `cloudflared.exe` (official release, checksum checked) and
   opens a tunnel.
3. In a project → **Hosting** → **Add a backup server** → **This PC**.
4. Turn on **Start with Windows** and **Keep this PC awake**. On a laptop set *When I close the lid*
   to *Do nothing* and plug it in.

### Wi-Fi with mobile data as backup

Use home Wi-Fi as the main connection and your phone as backup (USB tethering is the most reliable,
a hotspot also works). Windows switches by itself; the Cloudflare tunnel reconnects within seconds.
Rover's connection monitor shows which one is active and notifies you when the PC goes offline,
comes back, or moves to mobile data.

## Several servers for one project

Project → **Hosting** → **Add a backup server**. Every deploy goes to all servers. Choose:

* **Primary + backup** (default): Rover checks each server every minute and switches your
  domain through Cloudflare when the main one is down (and back when it recovers).
* **Together**: all servers answer at once through one Cloudflare tunnel; keeps working while
  your PC is off.
