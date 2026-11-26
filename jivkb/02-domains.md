=== add-domain
title: Connect your own domain
q: how do i connect my domain | add my domain | add a custom domain | use my own domain name | point domain to my site | domain setup | www domain
tags: domain custom connect point www dns a-record registrar
hi: domain kaise jode | apna domain kaise lagaye | डोमेन कैसे जोड़ें
go: Domains -> #/domains
do: set_domain
screen: domain
---
1. Open the project and click the **Domain** tab.
2. Type your domain (e.g. *mybakery.com*). Rover detects where it is registered.
3. Choose how:
   - **Fully automatic** - Rover moves the DNS to a free Cloudflare account and shows two nameservers to paste at your registrar, once.
   - **With an API key** (GoDaddy, Porkbun, Cloudflare) - Rover creates the records itself.
   - **Manual** - exact steps for your registrar plus a button that opens its DNS page.
4. Rover keeps checking and tells you when the domain is live. HTTPS is added automatically.
@hi
1. प्रोजेक्ट खोलें और **Domain** टैब दबाएँ।
2. अपना डोमेन लिखें (जैसे *mybakery.com*)। Rover पहचान लेगा कि डोमेन कहाँ रजिस्टर है।
3. तरीका चुनें: **Fully automatic** (Cloudflare), **API key** (GoDaddy/Porkbun) या **Manual** स्टेप।
4. डोमेन चालू होते ही Rover बता देगा। HTTPS अपने-आप लग जाता है।

=== godaddy
title: GoDaddy domain: step by step
q: how do i connect a godaddy domain | godaddy domain not working | godaddy dns settings | godaddy parked page | godaddy a record | domain bought on godaddy | godaddy forwarding
tags: godaddy dns parked a-record forwarding domaincontrol airo website-builder
hi: godaddy domain kaise jode | godaddy dns kaise badle | गोडैडी डोमेन कैसे जोड़ें
go: Domains -> #/domains
screen: domain
---
1. In the project's **Domain** tab type the domain and click **Open GoDaddy DNS**.
2. Edit the **A** record named **@** (it often says *Parked*) and paste your server IP (copy button in Rover). TTL 600.
3. Delete any other **A/AAAA** records for **@**.
4. Keep the **CNAME www -> @** (or add an A record *www* with the same IP).
5. Remove any **Forwarding**, and disconnect Website Builder / Airo if you used it.

Faster: connect a **GoDaddy API key** in the Domain tab and Rover does steps 2-4 for you. Changes usually work within 10-60 minutes.
@hi
1. **Domain** टैब में डोमेन लिखें और **Open GoDaddy DNS** दबाएँ।
2. **@** नाम वाला **A** रिकॉर्ड एडिट करें (अक्सर *Parked* लिखा होता है) और सर्वर का IP डालें।
3. **@** के बाकी A/AAAA रिकॉर्ड हटा दें।
4. **www** वाला CNAME रहने दें।
5. **Forwarding** हटा दें।

या GoDaddy API key जोड़ें, Rover यह सब खुद कर देगा।

=== other-registrars
title: Namecheap, Porkbun, Hostinger, Squarespace and other registrars
q: namecheap dns | porkbun domain | hostinger domain dns | squarespace domain | google domains | where do i change dns at my registrar
tags: namecheap porkbun hostinger squarespace registrar google dns advanced alias
hi: namecheap domain kaise jode | hostinger domain dns
go: Domains -> #/domains
screen: domain
---
Type the domain in the project's **Domain** tab - Rover detects the registrar and shows its exact steps with a button to its DNS page:

- **Namecheap**: Domain List > Manage > **Advanced DNS**. Delete the parking records, add **A @** and **A www** with your server IP.
- **Porkbun**: delete the default **ALIAS**, add the A records - or connect a Porkbun API key and Rover does it.
- **Hostinger**: Domains > **DNS / Nameservers**, edit the A records.
- **Squarespace**: remove the Squarespace defaults, add custom A records.

Any registrar: add **A @ -> server IP** and **A www -> server IP**, delete other A/AAAA records for those names.

=== cloudflare
title: Cloudflare (fully automatic DNS)
q: use cloudflare | cloudflare dns | orange cloud proxy | fully automatic domain | cloudflare nameservers | cloudflare token
tags: cloudflare nameserver proxy orange grey automatic token zone
hi: cloudflare kaise use kare | क्लाउडफ़्लेयर
go: Domains -> #/domains
screen: domain
---
**Fully automatic** in the Domain tab uses a free Cloudflare account:

1. Connect Cloudflare once with an API token (the link opens the right page with the right permissions).
2. Rover adds the domain to Cloudflare, copies your email (MX/TXT) records, and creates the A records.
3. Paste the **two nameservers** it shows at your registrar (one time).

Keep the records **DNS only (grey cloud)** until HTTPS works; Rover can turn the orange proxy on afterwards if you ticked it.
@hi
Cloudflare से जोड़ने पर Rover DNS रिकॉर्ड खुद बना सकता है। Cloudflare में **Edit zone DNS** टोकन बनाएँ और Domains पेज पर पेस्ट करें। टोकन इसी PC पर एन्क्रिप्ट रहता है और किसी AI को नहीं जाता।

=== dns-waiting
title: Domain is still waiting for DNS
q: domain not working | my domain does not work | domain still not live | dns not ready | domain does not point to server | why does my domain not open | dns propagation
tags: domain dns waiting propagation pending not-live point wrong ip parked
hi: domain kaam nahi kar raha | domain open nahi ho raha | डोमेन काम नहीं कर रहा
go: Domains -> #/domains
do: check_dns
screen: domain
---
Rover connects a domain only when the internet sees it pointing to your server. Common causes:

1. **The A record still points elsewhere** (often the registrar's *Parked* page). Change it to your server IP.
2. **An extra A or AAAA record** for the same name points somewhere else - delete it.
3. **Nameservers were just changed** - this can take a few hours (rarely up to 24-48 h).
4. **Typo** in the domain, or the domain has expired.

Ask me *"check the domain of bakery"* and I will look up what the internet sees right now and tell you exactly what to change.
@hi
डोमेन तभी जुड़ता है जब इंटरनेट पर वह आपके सर्वर की ओर इशारा करे। आम कारण:

1. **A रिकॉर्ड** अभी भी कहीं और जा रहा है (अक्सर *Parked* पेज)। उसे सर्वर IP पर बदलें।
2. उसी नाम का **दूसरा A या AAAA रिकॉर्ड** - उसे हटा दें।
3. **नेमसर्वर अभी बदले हैं** - कुछ घंटे लग सकते हैं।
4. डोमेन में **टाइपो** या डोमेन की अवधि खत्म।

मुझसे पूछें *"bakery ka domain check karo"*।

=== https-ssl
title: HTTPS / SSL certificate
q: how do i get https | ssl certificate | not secure warning | padlock missing | certificate error | https not working | lets encrypt
tags: https ssl certificate padlock secure letsencrypt cert warning expire
hi: ssl kaise lagaye | https nahi chal raha | एसएसएल
go: Domains -> #/domains
screen: domain
---
HTTPS is automatic and free (Let's Encrypt):

- After the domain points to your server, the certificate usually arrives in **1-5 minutes**. Rover checks and notifies you.
- Still a warning after 30 minutes? Check that **ports 80 and 443** are open in your provider's firewall, both A records point to the server, and there is **no AAAA record** pointing elsewhere.
- Using Cloudflare? Keep the cloud **grey** until HTTPS works.
- Certificates renew themselves; Rover warns you if one is about to expire.
@hi
HTTPS (ताला) अपने-आप मिलता है जब डोमेन आपके सर्वर की ओर इशारा करता है। ताला नहीं दिख रहा? 1-5 मिनट रुकें, जाँचें कि A रिकॉर्ड सही IP पर है, कोई पुराना AAAA रिकॉर्ड नहीं है, और पोर्ट 80/443 खुले हैं। Jiv से पूछें: *"bakery ka domain check karo"*।

=== instant-url
title: The instant address (sslip.io)
q: what is sslip.io | my site address looks strange | temporary url | instant address | link before domain | no domain yet | online without a domain | i do not have a domain | share the site before buying a domain
tags: sslip instant url address temporary preview link
hi: sslip kya hai
go: Projects -> #/projects
screen: project
---
Every project gets an **instant address** like *https://bakery.203-0-113-10.sslip.io* right away, before you add a domain. sslip.io is a free public DNS name that simply points to your server's IP - no account needed. It keeps working after you add your own domain, so you can share it for testing.

=== email-dns
title: Will my email keep working?
q: will my email break | mx records | email after changing dns | google workspace email | domain email
tags: email mx txt records mail workspace outlook break
hi: email band ho jayega kya | ईमेल
go: Domains -> #/domains
screen: domain
---
Rover only changes the **A** records for **@** and **www**. Your **MX/TXT** email records are left alone. With *Fully automatic* (Cloudflare) Rover copies the existing MX and TXT records into Cloudflare before you switch nameservers, and shows them so you can double-check.
