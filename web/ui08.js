// Rover 0.8 UI layer: icons for every navigation item, tab, button, menu,
// status, provider and file type, plus the new sidebar and theme toggle.
// One MutationObserver decorates what the older screens render, so no
// template had to be rewritten. Cheap: only added nodes are looked at.
"use strict";
(function () {
  const ic = window.ic, brand = window.brand;
  if (!ic) return;

  // ---------- emoji (as used by the 0.4-0.7 templates) -> icon ----------
  const EMO = {
    "\u{1F3E0}": "house", "\u{1F680}": "rocket", "\u{1F4C1}": "folder", "\u{1F310}": "globe", "\u{1F4C8}": "chart-line", "\u2699": "settings",
    "\u{1F5A5}": "server", "\u{1F9EA}": "flask-conical", "\u{1F4E6}": "package", "\u{1F4DD}": "notebook-pen", "\u2714": "check", "\u2716": "x",
    "\u{1F389}": "party-popper", "\u{1F512}": "lock", "\u{1F513}": "lock-open", "\u2728": "sparkles", "\u{1F511}": "key-round", "\u{1F517}": "link",
    "\u2601": "cloud", "\u{1F4C4}": "file", "\u{1F5D1}": "trash-2", "\u21E9": "download", "\u231B": "hourglass", "\u23F3": "hourglass",
    "\u25CB": "circle", "\u25B6": "play", "\u2197": "arrow-up-right", "\u27F3": "refresh-cw", "\u21BB": "refresh-cw", "\u2B06": "upload",
    "\u{1F50D}": "search", "\u{1F514}": "bell", "\u25D1": "sun", "\u{1F4BB}": "laptop", "\u{1F50C}": "plug", "\u{1F50B}": "battery-charging",
    "\u{1F465}": "users", "\u2139": "info", "\u26A0": "triangle-alert", "\u279C": "arrow-right", "\u2605": "star", "\u2606": "star", "\u22EF": "ellipsis",
    "\u{1F449}": "hand", "\u{1F6E1}": "shield", "\u{1F9ED}": "compass", "\u{1F916}": "bot", "\u{1F4CB}": "list-checks", "\u{1F4AC}": "message-circle",
    "\u{1F6A8}": "siren", "\u{1F4F1}": "smartphone", "\u{1F4E1}": "satellite", "\u{1F4BE}": "save", "\u2B07": "download", "\u{1F4E5}": "download",
    "\u{1F4E4}": "upload", "\u{1F527}": "wrench", "\u{1F4CA}": "chart-no-axes-column", "\u{1F4A1}": "sparkles", "\u2630": "menu", "\u{1F4D6}": "book-open",
  };
  const KEEP_TEXT = new Set(["\u2197"]); // the "opens in a new tab" arrow stays small
  const emoRe = new RegExp("(" + Object.keys(EMO).map(k => k.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|") + ")\uFE0F?", "gu");
  const keptRe = new RegExp("[" + [...KEEP_TEXT].join("") + "]", "gu");
  const onlyKept = (d) => { const r = d.replace(keptRe, ""); return r !== d && !new RegExp(emoRe.source, "u").test(r) && /\S/.test(r); };
  const CLS = { check: "ic-ok", x: "ic-bad", "triangle-alert": "ic-warn", hourglass: "ic-warn", circle: "ic-muted" };

  // ---------- button / link text -> icon ----------
  const BTN = [
    [/^(deploy|redeploy|publish( it| now| again)?|go live|deploy from)/i, "rocket"], [/^(roll ?back|undo|restore)/i, "undo-2"],
    [/^(refresh|reload|check again|check now|re-?scan|re-?check|scan)/i, "refresh-cw"], [/^(retry|try again)/i, "rotate-ccw"],
    [/^(delete|remove|forget|discard|clear)/i, "trash-2"], [/^(cancel|close|dismiss|not now|no thanks|hide)/i, "x"],
    [/^(save|keep)/i, "save"], [/^(open site|visit|open (the )?(site|page|link|dns|token|console|guide)|view (site|live))/i, "external-link"],
    [/^copy/i, "copy"], [/^(compare|diff)/i, "git-compare"], [/^(download|export)/i, "download"], [/^(upload|import|choose a \.zip)/i, "upload"],
    [/^(browse|choose|pick) (a |the )?folder|^open folder|^open in (file )?explorer/i, "folder-open"], [/^(edit|rename|change)/i, "pencil"],
    [/^(preview|view|show|see|open)/i, "eye"], [/^(test|run checks|check the site)/i, "flask-conical"], [/^(connect|sign in|log ?in|authori[sz]e|link)/i, "plug"],
    [/^(disconnect|sign out|log ?out)/i, "log-out"], [/^(add|new|create|\+)/i, "plus"], [/^(send|ask)/i, "send"], [/^(run|start|play)/i, "play"],
    [/^(stop|pause)/i, "circle-stop"], [/^(skip)/i, "skip-forward"], [/^(next|continue|go|use this|switch|make active|do it)/i, "arrow-right"],
    [/^(back|previous)/i, "arrow-left"], [/^(set up|setup|install|update|upgrade|get the)/i, "download"], [/^(restart|reboot|turn (it )?(on|off))/i, "power"],
    [/^(fix|repair|free up|clean)/i, "wrench"], [/^(diagnose|explain|why)/i, "stethoscope"], [/^(show me how|guide|walk me)/i, "graduation-cap"],
    [/^(settings|configure|manage|options)/i, "sliders-horizontal"], [/^(lock)/i, "lock"], [/^(unlock)/i, "lock-open"], [/^(pin)/i, "pin"],
    [/^(archive)/i, "archive"], [/^(move)/i, "move"], [/^(yes|ok|done|confirm|i did it|got it|finish|allow|agree)/i, "check"],
    [/^(notes?|add a note)/i, "notebook-pen"], [/^(logs?|visitors)/i, "scroll-text"], [/^(domain)/i, "globe"], [/^(server|hosting)/i, "server"],
    [/^(backup)/i, "hard-drive"], [/^(jiv|ask jiv)/i, "sparkles"], [/^(help)/i, "circle-help"], [/^(filter)/i, "filter"], [/^(search)/i, "search"],
  ];
  const TAB = { overview: "house", deploys: "rocket", domains: "globe", domain: "globe", hosting: "server", configuration: "sliders-horizontal", files: "folder",
    testing: "flask-conical", "visitors & logs": "chart-line", settings: "settings", "test a version": "flask-conical", general: "settings", "build & deploy": "package",
    "redirects & headers": "split", "access & password": "lock", notifications: "bell", backups: "hard-drive", "danger zone": "triangle-alert", storage: "database",
    server: "server", "connect ai": "bot", accounts: "users", updates: "download", jiv: "sparkles", appearance: "palette", shortcuts: "keyboard", "this pc": "laptop",
    "home server": "router", "hosting accounts": "cloud", guides: "graduation-cap", privacy: "shield", advanced: "sliders-horizontal" };

  // ---------- providers ----------
  const PROV = { hetzner: "hetzner", digitalocean: "digitalocean", vultr: "vultr", linode: "linode", akamai: "linode", cloudflare: "cloudflare", godaddy: "godaddy",
    namecheap: "namecheap", porkbun: "porkbun", hostinger: "hostinger", github: "github", gitlab: "gitlab", bitbucket: "bitbucket", netlify: "netlify",
    squarespace: "squarespace", contabo: "contabo", ovh: "ovh", raspberry: "raspberrypi", ubuntu: "ubuntu", docker: "docker", nginx: "nginx", coolify: "coolify",
    ollama: "ollama", anthropic: "anthropic", claude: "anthropic", gemini: "gemini", google: "google", mistral: "mistral", "hugging face": "huggingface",
    huggingface: "huggingface", openrouter: "openrouter", "lm studio": "lmstudio", lmstudio: "lmstudio", codeberg: "codeberg", gitea: "gitea", deepseek: "deepseek" };
  const GENERIC = { oracle: "cloud", lightsail: "cloud", aws: "cloud", scaleway: "cloud", upcloud: "cloud", other: "server", home: "house", thispc: "laptop",
    "this pc": "laptop", openai: "bot", groq: "zap", xai: "bot", custom: "plug", bigrock: "globe", azure: "cloud" };
  const lum = (hex) => { const n = parseInt(hex.slice(1), 16); return (0.299 * (n >> 16) + 0.587 * (n >> 8 & 255) + 0.114 * (n & 255)) / 255; };
  function provKey(s) {
    s = String(s || "").toLowerCase();
    for (const k in PROV) if (s.includes(k)) return { b: PROV[k] };
    for (const k in GENERIC) if (s.includes(k)) return { g: GENERIC[k] };
    return null;
  }
  // provTile: a brand mark on its brand colour, or a generic icon.
  window.provTile = function (idOrName, size) {
    const k = provKey(idOrName) || { g: "server" };
    if (k.b) {
      const c = (window.RV_BRAND || {})[k.b] || "#64748b";
      return `<span class="btile ${size || ""} ${lum(c) > 0.72 ? "dark-ink" : ""}" style="--b:${c}" aria-hidden="true">${brand(k.b)}</span>`;
    }
    return `<span class="btile generic ${size || ""}" aria-hidden="true">${ic(k.g)}</span>`;
  };
  if (typeof provLogo === "function") {
    const old = provLogo;
    provLogo = function (p, size) { // eslint-disable-line no-global-assign
      if (!p || (!p.id && !p.name)) return old(p, size);
      return window.provTile(p.id + " " + (p.name || ""), size === "lg" ? "lg" : "");
    };
  }

  // ---------- file types ----------
  function fileIcon(name, dir) {
    if (dir) return ["folder", ""];
    const e = (String(name).split(".").pop() || "").toLowerCase();
    if (/^html?$/.test(e)) return ["file-code-2", "ft-html"];
    if (/^(css|scss|sass|less)$/.test(e)) return ["file-code", "ft-css"];
    if (/^(m?js|ts|jsx|tsx|mjs|cjs)$/.test(e)) return ["file-code", "ft-js"];
    if (/^(json|toml|ya?ml|xml|webmanifest)$/.test(e)) return ["file-json", "ft-json"];
    if (/^(png|jpe?g|gif|webp|avif|svg|ico|bmp)$/.test(e)) return ["file-image", "ft-img"];
    if (/^(zip|gz|tgz|tar|7z|rar)$/.test(e)) return ["file-archive", "ft-zip"];
    if (/^(mp4|webm|mov|mkv)$/.test(e)) return ["file-video", ""];
    if (/^(mp3|wav|ogg|m4a)$/.test(e)) return ["file-audio", ""];
    if (/^(woff2?|ttf|otf|eot)$/.test(e)) return ["file-type", ""];
    if (/^(md|txt|csv|log|pdf)$/.test(e)) return ["file-text", ""];
    if (/^(redirects|headers)$/.test(e) || /^_(redirects|headers)$/.test(name)) return ["split", ""];
    return ["file", ""];
  }
  window.fileIcon = (name, dir) => { const [n, c] = fileIcon(name, dir); return ic(n, c); };

  // ---------- the decorator ----------
  const SKIP = "textarea,input,pre,code,script,style,svg,.composer,.mono,[contenteditable],.no-ic";
  function swapEmoji(root) {
    const probe = new RegExp(emoRe.source, "u");
    const tw = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, { acceptNode: n => probe.test(n.data) && !onlyKept(n.data) && !(n.parentElement && n.parentElement.closest(SKIP)) ? 1 : 2 });
    const hits = []; let n; while ((n = tw.nextNode())) hits.push(n);
    for (const t of hits) {
      const p = t.parentElement;
      if (!p) continue;
      // file rows: pick the icon from the file name instead
      if (p.classList.contains("fname")) {
        const row = p.closest(".frow"), name = t.data.replace(emoRe, "").trim();
        const [nm, c] = fileIcon(name, row && row.classList.contains("dir"));
        const span = document.createElement("span"); span.innerHTML = ic(nm, c) + " ";
        t.data = name; p.insertBefore(span.firstChild, t); continue;
      }
      const s = t.data, onlyEmoji = /^\s*$/.test(s.replace(new RegExp(emoRe.source, "gu"), ""));
      const frag = document.createDocumentFragment(); let last = 0; emoRe.lastIndex = 0; let m;
      while ((m = emoRe.exec(s))) {
        const k = m[1];
        if (KEEP_TEXT.has(k) && !onlyEmoji) continue;
        frag.appendChild(document.createTextNode(s.slice(last, m.index)));
        const name = EMO[k], w = document.createElement("span");
        w.innerHTML = ic(name, CLS[name] || ""); frag.appendChild(w.firstChild); last = m.index + m[0].length;
      }
      if (!last) continue;
      frag.appendChild(document.createTextNode(s.slice(last)));
      p.replaceChild(frag, t);
    }
  }
  function label(el) { return (el.getAttribute("aria-label") || el.textContent || el.title || "").replace(/\s+/g, " ").trim(); }
  function decorateButtons(root) {
    const list = root.matches && root.matches("button,a.btn") ? [root] : [];
    root.querySelectorAll && root.querySelectorAll("button,a.btn").forEach(b => list.push(b));
    for (const b of list) {
      if (b.dataset.ic || b.querySelector("svg,img,.spin,.btile,.plogo,.dot") || b.closest(".no-ic,.star,.chip,.chips,.provbtn,.choice,.mode,.tabs8,.g8bub,.palette,.seg")) continue;
      b.dataset.ic = "1";
      const t = label(b);
      if (!t || t.length > 48) continue;
      const hit = BTN.find(([re]) => re.test(t.replace(/^[+\s]+/, m => m.includes("+") ? "add " : "")));
      if (!hit) continue;
      if (/^\+\s*/.test(b.textContent)) b.innerHTML = b.innerHTML.replace(/^\s*\+\s*/, "");
      b.insertAdjacentHTML("afterbegin", ic(hit[1]));
    }
  }
  function decorateTabs(root) {
    (root.querySelectorAll ? root.querySelectorAll(".psub2 a,.tabs button,.gtabs a,.settabs a,.stabs a,.subnav a") : []).forEach(a => {
      if (a.dataset.ic || a.querySelector("svg")) return; a.dataset.ic = "1";
      const k = label(a).toLowerCase().replace(/\s*\(\d+\)$/, "").replace(/\s+\d+$/, "");
      if (TAB[k]) a.insertAdjacentHTML("afterbegin", ic(TAB[k]));
    });
  }
  const BADGE = { ok: "circle-check", bad: "circle-x", warn: "triangle-alert", info: "info", accent: "sparkles" };
  function decorateStatus(root) {
    (root.querySelectorAll ? root.querySelectorAll(".badge,.alert") : []).forEach(b => {
      if (b.dataset.ic || b.querySelector("svg")) return; b.dataset.ic = "1";
      const kind = ["ok", "bad", "warn", "info"].find(k => b.classList.contains(k)) || (b.classList.contains("accent") ? "accent" : "");
      if (!kind) return;
      if (b.classList.contains("alert") && b.textContent.trim().length < 2) return;
      b.insertAdjacentHTML("afterbegin", ic(BADGE[kind], "i-sm"));
    });
  }
  // brand marks next to provider / registrar names in headings and pickers
  function decorateBrands(root) {
    (root.querySelectorAll ? root.querySelectorAll(".provider .row>.dot, .reg-name, [data-registrar], .provbtn>span:last-child") : []).forEach(el => {
      if (el.dataset.ic) return; el.dataset.ic = "1";
      if (el.classList.contains("dot")) { // AI provider cards: swap the colour dot for the logo
        const name = label(el.parentElement.querySelector("b") || el.parentElement);
        if (provKey(name)) { const w = document.createElement("span"); w.innerHTML = window.provTile(name); el.replaceWith(w.firstChild); }
      } else if (el.dataset.registrar && provKey(el.dataset.registrar)) el.insertAdjacentHTML("afterbegin", window.provTile(el.dataset.registrar) + " ");
    });
  }
  function decorate(root) {
    if (!root || root.nodeType !== 1 || root.closest && root.closest("svg")) return;
    swapEmoji(root); decorateButtons(root); decorateTabs(root); decorateStatus(root); decorateBrands(root);
  }
  let queue = new Set(), raf = 0;
  const flush = () => { raf = 0; const q = queue; queue = new Set(); q.forEach(n => n.isConnected && decorate(n)); };
  new MutationObserver(ms => {
    for (const m of ms) for (const n of m.addedNodes) { if (n.nodeType === 1) queue.add(n); else if (n.nodeType === 3 && n.parentElement) queue.add(n.parentElement); }
    if (queue.size && !raf) raf = requestAnimationFrame(flush);
  }).observe(document.body, { childList: true, subtree: true });

  // ---------- project tabs (explicit icons instead of emoji) ----------
  const PT = { "": "house", deploys: "rocket", domain: "globe", hosting: "server", config: "sliders-horizontal", files: "folder", testing: "flask-conical", logs: "chart-line", settings: "settings" };
  if (typeof projTabs !== "undefined") projTabs.forEach(t => { if (PT[t[0]] !== undefined) t[2] = ic(PT[t[0]]); });

  // ---------- sidebar ----------
  const NAV = { dashboard: "layout-dashboard", projects: "folder-kanban", deploys: "rocket", domains: "globe", hosting: "cloud", integrations: "plug", analytics: "chart-line", monitor: "activity", testing: "flask-conical", jiv: "sparkles", settings: "settings", thispc: "laptop", guides: "graduation-cap" };
  function navIcons() {
    document.querySelectorAll("#nav > a[data-route]").forEach(a => {
      const n = NAV[a.dataset.route] || (a.getAttribute("href") || "").replace(/^#\//, "").split("/")[0];
      const name = NAV[n] || NAV[a.dataset.route];
      if (!name || a.dataset.ic === name) return;
      const old = a.querySelector("svg"); const w = document.createElement("span"); w.innerHTML = ic(name);
      if (old) old.replaceWith(w.firstChild); else a.prepend(w.firstChild);
      a.dataset.ic = name;
    });
  }
  navIcons(); new MutationObserver(navIcons).observe(document.getElementById("nav"), { childList: true });
  const tb = document.getElementById("themeBtn"), bell = document.getElementById("bell");
  const themeIcon = () => { if (!tb) return; const d = document.documentElement.getAttribute("data-theme") === "dark"; tb.innerHTML = ic(d ? "sun" : "moon") + `<span class="lbl">${d ? "Light theme" : "Dark theme"}</span>`; };
  if (tb) { themeIcon(); tb.addEventListener("click", () => setTimeout(themeIcon, 0)); new MutationObserver(themeIcon).observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] }); }
  if (bell && !bell.querySelector("svg")) { bell.childNodes.forEach(n => { if (n.nodeType === 3) n.data = ""; }); bell.insertAdjacentHTML("afterbegin", ic("bell")); }
  const conn = document.getElementById("connState");
  if (conn && !conn.querySelector("svg")) conn.insertAdjacentHTML("beforeend", ic("chevron-right", "i-sm ic-muted"));
  decorate(document.body);
})();
