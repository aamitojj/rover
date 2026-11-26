=== error-index-missing
title: Deploy failed: no index.html
q: no index.html found | deploy failed index html missing | homepage not found | site shows directory listing
tags: index html missing homepage deploy failed top folder
hi: index.html nahi mila
go: Add new project -> #/new
screen: deploys
---
Your upload has no **index.html** in its top folder. Fixes:

- Zip the **contents** of your site folder, not the folder itself (or just drop the folder - Rover finds the right level).
- If your home page has another name (home.html, default.htm), Rover renames it automatically - otherwise rename it to index.html.
- A React/Vue project? Build it first (see *This project must be built first*).

=== error-too-large
title: Upload too large
q: file too large | upload too big | deploy size limit | video files too big
tags: large size limit upload big video files
hi: file bahut badi hai
go: Storage settings -> #/settings/storage
screen: deploys
---
Very big files (videos, backups, node_modules) slow everything down. Remove **node_modules**, .git and backup files from the folder; host large videos on YouTube/Vimeo or a storage bucket. Rover already skips node_modules and .git when you deploy a folder.

=== error-rate-limit
title: Too many requests / rate limit
q: rate limit | too many requests 429 | lets encrypt rate limit | api limit
tags: rate limit 429 too many requests letsencrypt api
hi: rate limit error
go: Dashboard -> #/dashboard
screen: any
---
A service is asking Rover to slow down. Wait a few minutes and try again. For Let's Encrypt certificates, avoid deleting and re-adding the same domain many times in one day (limit: 5 identical certificates per week).

=== error-git-private
title: Git import fails: "private" or 404
q: repository not found | git 404 | private repository error | authentication required git | could not read from remote
tags: git private 404 repository token auth github gitlab
hi: github private repo error
go: Git settings -> #/settings/git
screen: git
---
The repository is private or the address is wrong. Click **Connect account** next to the Git link and create a **read-only** token with the link shown (GitHub: *Contents: read* on that repo). Rover stores it encrypted on this PC. Check the branch name too (main vs master).

=== error-node-missing
title: npm or Node.js not found
q: npm not found | node is not recognized | install nodejs | build failed npm
tags: npm node nodejs not found build install lts
hi: npm nahi mila
go: Add new project -> #/new
screen: build
---
**Build it for me** needs Node.js on this PC. Install the **LTS** version from nodejs.org (default options), restart Rover, and click Build again. Or build in your editor and drop the output folder (dist/build).

=== error-timeout
title: Timeouts and "context deadline exceeded"
q: context deadline exceeded | timeout error | operation timed out | server too slow to answer
tags: timeout deadline slow network unreachable
hi: timeout error
go: Server settings -> #/settings/server
screen: any
---
The server took too long to answer. Usually: the server is busy (installing or out of memory), the internet connection dropped, or a firewall is silently blocking a port. Wait a minute and retry. If it keeps happening, check the server in your provider panel (CPU/RAM) and that ports 22 and 8000 are open.
