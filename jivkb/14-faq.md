=== faq-cost
title: How much does it cost?
q: how much does rover cost | is rover free | monthly cost of hosting | cheapest way to host
tags: cost price free monthly cheap budget money
hi: kitna kharcha hoga | rover free hai kya | कितना खर्च
go: Hosting accounts -> #/hosting
screen: any
---
Rover itself is free. You pay only for what you choose: a server (free on Oracle's Always Free tier, otherwise roughly ₹350-₹1,000/month) and a domain (about ₹800-₹1,200/year for a .com, less for .in). Hosting on this PC or a home server costs nothing extra. Jiv's offline brain is free too.
@hi
Rover मुफ़्त है। खर्च सिर्फ़ सर्वर (Oracle पर मुफ़्त, वरना लगभग ₹350-₹1,000/महीना) और डोमेन (लगभग ₹800-₹1,200/साल) का है। अपने PC या घर के सर्वर पर होस्ट करना मुफ़्त है।

=== faq-internet
title: What works without internet?
q: does rover work without internet | what needs internet | offline mode
tags: offline internet no connection works without network
hi: bina internet ke kya chalega | इंटरनेट के बिना
go: Settings > Jiv -> #/settings/jiv
screen: any
---
Without internet you can still use Jiv (built-in answers, all walkthroughs, and the offline brain if installed), browse projects and deploy history, build sites and prepare deploys. Publishing to a cloud server and checking domains naturally need a connection. Sites hosted on this PC keep working on your home network.
@hi
इंटरनेट के बिना भी Jiv (सारे जवाब, गाइड और ऑफ़लाइन ब्रेन), प्रोजेक्ट और डिप्लॉय इतिहास चलते हैं। क्लाउड सर्वर पर पब्लिश करने और डोमेन जाँचने के लिए इंटरनेट चाहिए।

=== faq-many-sites
title: Can I host many websites?
q: how many websites can i host | multiple sites one server | host sites for clients
tags: many sites multiple clients agency one server limit
hi: kitni website host kar sakte hai
go: Projects -> #/projects
screen: projects
---
Yes - there's no limit in Rover. A 2-4 GB server comfortably runs dozens of static sites, each with its own domain and HTTPS. Use groups and tags (Settings > Groups & tags) to organise client projects.

=== faq-uninstall
title: Uninstalling or moving to a new PC
q: uninstall rover | move rover to a new computer | will my sites stop if i uninstall | reinstall windows rover
tags: uninstall new pc move reinstall backup config
hi: naya pc par shift
go: Backups & alerts -> #/settings/backups
screen: settings
---
Your sites run on your servers, so they **keep running** if you uninstall Rover or turn this PC off (except sites hosted on this PC). To move: make a backup in Settings > Backups & alerts, install Rover on the new PC and restore it. Connect to each server once again (the encrypted keys are tied to your Windows account).

=== faq-security
title: Is my data safe?
q: is rover secure | where are my passwords stored | does rover see my data | privacy of my tokens
tags: security privacy passwords tokens dpapi encrypted data safe
hi: kya mera data safe hai | सुरक्षित
go: Settings -> #/settings
screen: any
---
Tokens, keys and passwords are encrypted with Windows **DPAPI** on this PC and never sent to any AI. Server passwords are used once to install Rover's own key and then forgotten. Rover never types passwords into websites. Everything that changes a server or DNS needs your click. Jiv's offline brain runs entirely on this PC.
@hi
टोकन, की और पासवर्ड Windows **DPAPI** से इसी PC पर एन्क्रिप्ट होते हैं और किसी AI को नहीं भेजे जाते। सर्वर या DNS बदलने वाले हर काम के लिए आपकी पुष्टि ज़रूरी है।

=== faq-restore-backup
title: Restoring a backup
q: restore from backup | recover deleted project | get my old files back | restore a backup zip
tags: restore backup recover deleted files zip old
hi: backup se wapas lao
go: Backups & alerts -> #/settings/backups
screen: settings
---
Every deploy is kept on this PC: open the project's **Deploys** tab and publish an older one. For a deleted project or a new PC, Settings > Backups & alerts > **Restore** brings back projects and settings from your backup folder. Ask *"show me how to restore an old deploy"*.

=== faq-speed
title: Making my site faster
q: make my website faster | site is slow | improve page speed | compress images
tags: speed slow fast performance images cache compress cdn
hi: site slow hai
go: App Testing -> #/testing
screen: testing
---
Compress big images (WebP, under 300 KB each), remove unused scripts, and choose a server close to your visitors (Mumbai/Singapore for India). Rover already serves files compressed with long caching. Cloudflare's orange cloud adds a CDN. App Testing shows the heaviest files.

=== faq-seo
title: Google and SEO
q: show my site on google | seo for my site | google search console verify | sitemap
tags: seo google search console sitemap ranking index verify txt
hi: google par kaise aaye
go: Domains -> #/domains
screen: any
---
Use your own domain with HTTPS (Rover does both), add a `sitemap.xml` and good page titles, then verify the domain in **Google Search Console** with the TXT record it gives you (add it at your DNS host). New sites take a few days to appear.

=== faq-mac-linux
title: Does Rover run on Mac or Linux?
q: rover for mac | linux version | does it work on macos
tags: mac macos linux windows platform support
hi: mac par chalega
go: Updates -> #/settings/updates
screen: any
---
Rover is built for **Windows 10/11**. The servers it manages run Linux (Ubuntu/Debian). A browser mode exists for testing, but the installer, encrypted storage (DPAPI) and This PC hosting are Windows features.

=== faq-team
title: Can my team use it?
q: share with my team | multiple users | give access to a colleague | two people manage sites
tags: team users share colleague access multiple people
hi: team ke saath use
go: Settings -> #/settings
screen: any
---
Rover runs on one PC for one person. A colleague can install Rover on their own PC and connect the same server (each PC gets its own key). Avoid deploying the same project from two PCs at the same time.

=== faq-gpu-laptop
title: Will Jiv's brain slow down my laptop?
q: will the brain slow my laptop | brain uses too much ram | jiv makes pc slow | battery usage brain
tags: slow laptop ram battery cpu brain idle unload
hi: laptop slow ho jayega kya
go: Settings > Jiv -> #/settings/jiv
screen: jiv-settings
---
The brain only works while Jiv answers, and unloads itself after 20 idle minutes. Light needs about 2 GB of RAM, Standard about 3.5 GB (less on a graphics card). On 8 GB laptops choose Light, or Standard on a 4 GB graphics card. You can stop it any time in Settings > Jiv.
@hi
ब्रेन सिर्फ़ जवाब देते समय काम करता है और 20 मिनट खाली रहने पर बंद हो जाता है। 8 GB लैपटॉप पर Light चुनें, या 4 GB ग्राफ़िक्स कार्ड हो तो Standard।

=== faq-renamed-rover
title: Dilreet is now called Rover
q: what happened to dilreet | dilreet renamed | where is dilreet | is rover the same as dilreet | where did my dilreet folder go | new name
tags: dilreet rover rename renamed name new old hostpanel folder moved
hi: dilreet ka naam kya hua | dilreet kaha gaya | डिलरीट का नाम
go: Settings -> #/settings
screen: any
---
Rover is the new name of Dilreet (version 0.8 and later); it is the same app and Jiv is still Jiv. Your settings, projects and servers came with it: on first start Rover moved `%APPDATA%\Dilreet` to `%APPDATA%\Rover` and `Documents\Dilreet` to `Documents\Rover`, and left a `MOVED-TO-ROVER.txt` note in the old place. If a folder was in use (for example by OneDrive), Rover keeps using the old folder and nothing is lost. Your websites keep running; nothing changes on your servers.
@hi
Rover, Dilreet का नया नाम है (0.8 से)। ऐप वही है और Jiv भी वही है। आपकी सेटिंग, प्रोजेक्ट और सर्वर अपने-आप `Rover` फ़ोल्डर में आ गए हैं; पुराने फ़ोल्डर में `MOVED-TO-ROVER.txt` नोट है। आपकी वेबसाइटें चलती रहती हैं।
