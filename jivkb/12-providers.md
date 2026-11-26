=== provider-oracle
title: Oracle Cloud free server (Always Free)
q: oracle cloud free vps | oracle always free | create oracle instance | oracle ampere a1 | oracle cloud ssh key
tags: oracle cloud oci free always free ampere a1 instance vcn security list
hi: oracle free server kaise banaye | ओरेकल
go: Hosting accounts -> #/hosting
screen: hosting
---
1. Sign up at cloud.oracle.com (a card is needed for verification; Always Free resources cost nothing).
2. **Compute > Instances > Create instance**: image **Ubuntu 22.04/24.04**, shape **VM.Standard.A1.Flex** (Ampere, free up to 4 cores/24 GB) or E2.1.Micro.
3. Under *Add SSH keys* choose **Paste public keys** and paste the key Rover shows (copy button in the walkthrough).
4. Create, copy the **Public IP address**.
5. **Open ports 80 and 443**: Networking > VCN > Security lists > Add ingress rules (source 0.0.0.0/0, TCP, 80,443).
6. In Rover: Hosting accounts > **Any other VPS**, paste the IP, user **ubuntu**. (Advanced: Hosting accounts > Oracle Cloud with an API signing key lets Rover create the instance itself.)

Rover also opens the server's own iptables firewall. "Out of capacity" for A1: try another availability domain or later. Ask *"show me how to add an Oracle server"*.
@hi
1. cloud.oracle.com पर साइन अप करें।
2. **Compute > Instances > Create instance**: Ubuntu, shape **A1.Flex** (मुफ़्त)।
3. SSH key में Rover की दी हुई key पेस्ट करें।
4. **Public IP** कॉपी करें।
5. Security list में पोर्ट **80, 443** खोलें।
6. Rover में **Any other VPS** चुनें, IP डालें, यूज़र **ubuntu**।

=== provider-hetzner
title: Hetzner Cloud
q: hetzner api token | create hetzner server | hetzner cloud account | hetzner firewall
tags: hetzner cloud token api cx22 server create firewall
hi: hetzner token kaise banaye
go: Hosting accounts -> #/hosting
screen: hosting
---
In Hetzner Cloud Console open your project > **Security > API tokens > Generate API token**, choose **Read & Write**, copy it and paste it in Rover (Hosting accounts > Hetzner). Rover then creates servers for you (it shows the monthly price first) and adds its SSH key. If you use a Hetzner firewall, allow TCP 22, 80 and 443.

=== provider-digitalocean
title: DigitalOcean
q: digitalocean token | create droplet | digitalocean api key
tags: digitalocean droplet token api personal access
hi: digitalocean token
go: Hosting accounts -> #/hosting
screen: hosting
---
Control panel > **API > Generate New Token** with *Read* and *Write* scopes. Paste it in Hosting accounts > DigitalOcean. Rover creates an Ubuntu droplet (price shown first). Existing droplet? Use **Any other VPS** with its IP and root.

=== provider-vultr-linode
title: Vultr, Linode (Akamai) and others
q: vultr api key | linode token | akamai cloud | contabo vps | hostinger vps
tags: vultr linode akamai contabo hostinger ovh vps provider token
hi: dusra vps provider
go: Hosting accounts -> #/hosting
screen: hosting
---
Vultr, Linode, Contabo, Hostinger VPS, Scaleway and UpCloud can be connected in **Hosting accounts** with an API key (the wizard links to the right page), so Rover can create servers and show prices. Any other provider (OVH, a local data centre...) works through **Any other VPS**: create an **Ubuntu** server in their panel, then give Rover the IP and root password once.

=== provider-aws
title: AWS Lightsail / EC2
q: aws lightsail | amazon ec2 hosting | aws security group | aws key pair pem
tags: aws amazon lightsail ec2 security group pem key
hi: aws par host
go: Hosting accounts -> #/hosting
screen: hosting
---
Lightsail can be connected in Hosting accounts with an IAM access key so Rover creates the instance. Or create an **Ubuntu** instance yourself, download its **.pem** key, and in Rover choose **Any other VPS** with user **ubuntu** and the key file. Open ports **80 and 443** in the instance's *Networking* (Lightsail) or *security group* (EC2). Attach a static IP so it doesn't change.

=== registrar-namecheap
title: Namecheap DNS
q: namecheap dns | namecheap advanced dns | point namecheap domain
tags: namecheap registrar advanced dns a record host
hi: namecheap domain
go: Domains -> #/domains
screen: domain
---
Domain List > **Manage** > **Advanced DNS**. Delete the *URL Redirect* and *CNAME www → parkingpage* records, then **Add new record**: A Record, Host `@`, Value = your server IP; another for Host `www`. TTL Automatic.

=== registrar-hostinger
title: Hostinger domain DNS
q: hostinger dns zone | hostinger domain point to vps | hpanel dns
tags: hostinger hpanel dns zone registrar a record
hi: hostinger domain
go: Domains -> #/domains
screen: domain
---
hPanel > **Domains** > your domain > **DNS / Nameservers** > *DNS records*. Edit the A record for `@` (and `www`) to your server IP; delete AAAA records pointing to Hostinger parking.

=== registrar-bigrock
title: BigRock, ResellerClub, Google/Squarespace domains
q: bigrock dns | resellerclub dns | squarespace domains dns | google domains moved
tags: bigrock resellerclub squarespace google domains dns registrar india
hi: bigrock domain
go: Domains -> #/domains
screen: domain
---
- **BigRock/ResellerClub**: My Account > Domain > **DNS Management** > A Records > Add (`@` and `www` → IP).
- **Squarespace (ex-Google Domains)**: Domains > your domain > **DNS > DNS Settings** > Custom records.

Not sure where your DNS is? Rover's Domain tab shows who hosts it.

=== registrar-porkbun
title: Porkbun (automatic DNS)
q: porkbun api key | porkbun dns automatic
tags: porkbun api key secret dns automatic registrar
hi: porkbun api
go: Domains -> #/domains
screen: domain
---
Porkbun: Account > **API Access** > create a key, and enable *API Access* on the domain. Paste the key and secret in the Domain tab. Rover then creates the records itself (stored encrypted, never sent to any AI).
