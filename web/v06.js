// Rover 0.6: the project hub (Netlify-style project pages with a deploy
// dropzone, setup checklist, deploys, domains, hosting and configuration),
// faster deploying (drop anywhere, redeploy last folder, paste a path,
// shortcuts, Explorer menu hand-off) and self-update from GitHub Releases.
"use strict";

// ---------- project navigation ----------
projTabs.splice(0, projTabs.length,
  ["", "Overview", "&#127968;"], ["deploys", "Deploys", "&#128640;"], ["domain", "Domains", "&#127760;"], ["hosting", "Hosting", "&#128421;"],
  ["config", "Configuration", "&#9881;"], ["files", "Files", "&#128193;"], ["testing", "Testing", "&#129514;"], ["logs", "Visitors & logs", "&#128200;"]);
const cfgSections = [["general", "General"], ["build", "Build & deploy"], ["rules", "Redirects & headers"], ["access", "Access & password"], ["alerts", "Notifications"], ["backups", "Backups"], ["danger", "Danger zone"]];
let cfgSub = "general";
projectShell = function (s, tab, inner) {
  const base = `#/project/${esc(s.slug)}`;
  const sub = tab === "config" ? `<div class="psub2">${cfgSections.map(([k, l]) => `<a href="${base}/config/${k}" ${k === cfgSub ? 'aria-current="page"' : ""}>${l}</a>`).join("")}</div>` : "";
  let head = projectHead(s, tab);
  const tl = (projTabs.find(([k]) => k === (tab || "")) || [])[1];
  if (tab && tl) head = head.replace("</div>", ` <a href="${base}">${esc(s.name)}</a> / ${tl}${tab === "config" ? ` / ${(cfgSections.find(([k]) => k === cfgSub) || [])[1] || ""}` : ""}</div>`);
  return head + `<div class="plyt"><nav class="psub" aria-label="Project menu">${projTabs.map(([k, l, ic], i) => `<a href="${base}${k ? "/" + k : ""}" ${k === (tab || "") ? 'aria-current="page"' : ""} title="${esc(l)} (${i + 1})"><span aria-hidden="true">${ic}</span>${l}</a>${k === "config" ? sub : ""}`).join("")}</nav><div class="pmain">${inner}</div></div>`;
};
const projectOverviewV05 = projectOverview;
const projectDeploysV05 = projectDeploys;

route(/^\/project\/([^/]+)(?:\/(deploys|hosting|config|files|testing|logs|settings)(?:\/([a-z]+))?)?$/, async (slug, tab, sub) => {
  if (tab === "settings") { location.replace(`#/project/${encodeURIComponent(slug)}/config`); return; }
  if (sub && tab !== "config") { location.replace(`#/project/${encodeURIComponent(slug)}/${tab}`); return; }
  cfgSub = cfgSections.some(([k]) => k === sub) ? sub : "general";
  await refreshOverview();
  await loadOrg();
  const s = await getSite(slug);
  view().innerHTML = projectShell(s, tab, `${sitesErr ? `<div class="alert warn small">Live status unavailable: ${esc(sitesErr)}</div>` : ""}<div id="pBody"></div>`);
  const b = $("#pBody");
  store("lastProject", s.slug);
  if (tab === "logs") return projectLogs(s, b);
  if (tab === "files") return projectFiles(s, b);
  if (tab === "deploys") return deploysTab(s, b);
  if (tab === "hosting") return hostingTab(s, b);
  if (tab === "config") return configTab(s, b, cfgSub);
  if (tab === "testing") return testingTab(s, b);
  return overviewTab(s, b);
}, "projects");
routes.unshift(routes.pop());

// ---------- helpers ----------
const siteCfg = (s) => api("GET", `/api/sites/${s.slug}/config`);
const saveCfg = (s, body, msg) => api("POST", `/api/sites/${s.slug}/config`, body).then(r => { if (msg !== false) toast(msg || "Saved", "success"); return r; });
function liveURL(s) { return siteURL(s); }
function shortPath(p) { p = String(p || ""); return p.length > 48 ? "…" + p.slice(-46) : p; }
function looksLikePath(t) { t = String(t || "").trim().replace(/^"|"$/g, ""); return /^([A-Za-z]:[\\/]|\\\\[^\\]+\\|\/)[^\n<>|?*]*$/.test(t) && t.length < 400 ? t : ""; }
async function applyNow(s, el, why) {
  if (!await confirmBox("Publish the change now?", `<p>${why || "Rover publishes the current files of"} <b>${esc(s.name)}</b> again with the new settings. With fast deploys only the changed files are sent; the site stays online.</p>`, "Publish now")) return;
  const r = await api("POST", `/api/sites/${s.slug}/apply`);
  const j = await jobPanel(r.job, el, { title: "Publishing", doneTitle: "Published", failTitle: "Publishing failed" });
  if (!j.error) { toast("Published", "success"); sysNotify(`${s.name} is updated`, "Your change is live."); }
  return j;
}
function sysNotify(title, body) {
  try {
    if (!store("deployNotify") || !("Notification" in window) || Notification.permission !== "granted" || !document.hidden) return;
    new Notification(title, { body, icon: "/static/icon.png" });
  } catch (e) {}
}

// ---------- the deploy dropzone ----------
// mode: "deploy" (publish now), "draft" (save only) or "test" (App Testing)
function deployZone(el, s, opt) {
  opt = opt || {};
  const big = opt.big !== false;
  el.innerHTML = `<div class="dz6 ${big ? "big" : ""}" tabindex="0" role="button" aria-label="Drag and drop your new build folder or zip here, or press Enter to browse">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M12 16V4M7 9l5-5 5 5"/><path d="M4 16v3a1 1 0 001 1h14a1 1 0 001-1v-3"/></svg>
      <div class="big">${opt.mode === "test" ? "Drop a version here to test it" : "Drag and drop your new build folder or zip here"}</div>
      <div class="muted small">${opt.mode === "test" ? "It opens in App Testing; nothing is published." : `It goes live on <b>${esc((liveURL(s) || "your site").replace(/^https?:\/\//, ""))}</b> in seconds. Your previous version is kept for one-click rollback.`}</div>
      <div class="row center" style="margin-top:10px"><button type="button" class="sm" data-b="dir">&#128193; Browse for a folder</button><button type="button" class="sm" data-b="zip">&#128230; Choose a .zip</button></div>
      <input type="file" class="hidden" data-i="dir" webkitdirectory multiple><input type="file" class="hidden" data-i="zip" accept=".zip"></div>
    ${opt.mode === "test" ? "" : `<div class="row dzopts"><label class="check small"><input type="checkbox" data-i="draft" ${opt.mode === "draft" ? "checked" : ""}> Save as a draft (preview first, publish later)</label><span class="grow"></span><details class="dzpath"><summary class="small">Deploy from a folder path on this PC</summary>
      <div class="row" style="margin-top:6px"><input data-i="path" list="dzRecent-${esc(s.slug)}" placeholder="C:\\Sites\\${esc(s.slug)}\\dist" style="flex:1" aria-label="Folder or zip path"><datalist id="dzRecent-${esc(s.slug)}"></datalist><button type="button" class="sm primary" data-b="path">Deploy</button></div>
      <p class="hint">Tip: copy a folder in File Explorer (Shift + right-click &rarr; Copy as path), then press <span class="kbd">Ctrl</span> <span class="kbd">V</span> anywhere on this page.</p></details></div>`}
    <div class="dzStatus" aria-live="polite"></div>`;
  const d = $(".dz6", el), st = $(".dzStatus", el);
  // Keep the last result (live address, Undo) visible when the page refreshes after a deploy.
  const lr = lastDeployResult[s.slug];
  if (lr && Date.now() - lr.t < 10 * 60 * 1000 && opt.mode !== "test") st.innerHTML = lr.html;
  const draft = () => { const c = $("[data-i=draft]", el); return !!(c && c.checked); };
  const run = (sel) => runDeploy(s, sel, st, { mode: opt.mode === "test" ? "test" : draft() ? "draft" : "deploy", onDone: opt.onDone });
  ["dragenter", "dragover"].forEach(ev => d.addEventListener(ev, e => { e.preventDefault(); e.stopPropagation(); d.classList.add("over"); }));
  ["dragleave", "drop"].forEach(ev => d.addEventListener(ev, e => { e.preventDefault(); d.classList.remove("over"); }));
  d.addEventListener("drop", async e => { e.stopPropagation(); hideDropOverlay(); const sel = await readDropped(e.dataTransfer); run(sel); });
  d.addEventListener("click", e => { if (!e.target.closest("button")) $("[data-i=dir]", el).click(); });
  d.addEventListener("keydown", e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); $("[data-i=dir]", el).click(); } });
  $("[data-b=dir]", el).onclick = () => $("[data-i=dir]", el).click();
  $("[data-b=zip]", el).onclick = () => $("[data-i=zip]", el).click();
  $("[data-i=dir]", el).onchange = (e) => { const files = Array.from(e.target.files).map(f => [f, f.webkitRelativePath || f.name]).filter(([, p]) => !SKIP_DIRS.test(p)); e.target.value = ""; if (files.length) run({ files }); };
  $("[data-i=zip]", el).onchange = (e) => { const f = e.target.files[0]; e.target.value = ""; if (f) run({ zip: f }); };
  if ($("[data-b=path]", el)) {
    $("[data-b=path]", el).onclick = () => { const p = $("[data-i=path]", el).value.trim().replace(/^"|"$/g, ""); if (p) run({ path: p }); };
    $("[data-i=path]", el).onkeydown = (e) => { if (e.key === "Enter") $("[data-b=path]", el).click(); };
    api("GET", "/api/quick").then(q => { const dl = $(`#dzRecent-${CSS.escape(s.slug)}`); if (dl) dl.innerHTML = (q.recent || []).map(p => `<option value="${esc(p)}">`).join(""); }).catch(() => {});
  }
  el._run = run;
  return run;
}

// runDeploy sends the files and follows the job: progress, result, instant URL.
let deployBusy = false;
const lastDeployResult = {};
async function runDeploy(s, sel, st, opt) {
  opt = opt || {};
  delete lastDeployResult[s.slug];
  if (!sel || (!sel.zip && !(sel.files || []).length && !sel.path)) { st.innerHTML = `<div class="alert bad">Drop a folder or a single .zip file.</div>`; return; }
  if (deployBusy) { toast("A deploy is already running. Wait for it to finish.", "error"); return; }
  deployBusy = true;
  const t0 = Date.now();
  const what = sel.zip ? esc(sel.zip.name) : sel.path ? `<code>${esc(shortPath(sel.path))}</code>` : `${sel.files.length} files (${esc(humanSize(sel.files.reduce((n, [f]) => n + f.size, 0)))})`;
  st.innerHTML = `<div class="dzrun"><div class="row"><span class="spin"></span><b>${opt.mode === "test" ? "Loading" : "Sending"} ${what}&hellip;</b></div><div class="progress indet"><div style="width:0"></div></div></div>`;
  try {
    if (opt.mode === "test") {
      const fd = formFrom(sel);
      await api("POST", "/api/test/load", fd);
      store("testSite", s.slug);
      toast("Loaded in App Testing. Click around, then publish from there.", "success");
      go("#/testing");
      return;
    }
    let r;
    if (sel.path) r = await api("POST", `/api/sites/${s.slug}/redeploy-last`, { path: sel.path, draft: opt.mode === "draft" });
    else r = await api("POST", `/api/sites/${s.slug}/deploy${opt.mode === "draft" ? "?draft=1" : ""}`, formFrom(sel));
    st.innerHTML = `<div class="dzrun" id="dzJob"></div>`;
    const j = await jobPanel(r.job, $("#dzJob", st), { title: opt.mode === "draft" ? "Saving the draft" : "Deploying", doneTitle: opt.mode === "draft" ? "Draft saved" : "Live", failTitle: "Deploy failed" });
    const secs = Math.max(1, Math.round((Date.now() - t0) / 1000));
    if (j.error) { sysNotify(`${s.name}: deploy failed`, j.error); return; }
    await loadSites();
    const fresh = sites.find(x => x.slug === s.slug) || s, url = liveURL(fresh);
    const res = h(opt.mode === "draft" ? `<div class="dzres draft"><div class="big">&#128221; Draft saved</div><p class="small">The live site was not changed. Preview it, then publish when you are happy.</p><div class="row"><a class="btn primary" href="#/project/${esc(s.slug)}/deploys">Open Deploys to preview or publish</a></div></div>`
      : `<div class="dzres"><div class="big">&#10004; Your new version is live</div>${url ? `<p><a href="${esc(url)}" target="_blank" rel="noopener" class="lurl">${esc(url.replace(/^https?:\/\//, ""))} &#8599;</a></p>` : ""}<p class="small muted">Published in ${secs} s${fresh.instant_url && fresh.domain ? ` &middot; also at <a href="${esc(fresh.instant_url)}" target="_blank" rel="noopener">${esc(fresh.instant_url.replace(/^https?:\/\//, ""))}</a>` : ""}</p>
        <div class="row">${url ? `<a class="btn primary" href="${esc(url)}" target="_blank" rel="noopener">Open site</a>${copyBtn(url)}` : ""}<a class="btn ghost" href="#/project/${esc(s.slug)}/deploys">Deploy history</a><button class="ghost" data-undo>Undo (roll back)</button></div></div>`);
    st.appendChild(res);
    lastDeployResult[s.slug] = { html: res.outerHTML, t: Date.now() };
    sysNotify(opt.mode === "draft" ? `${s.name}: draft saved` : `${s.name} is live`, url || "");
    if (opt.onDone) opt.onDone(j);
  } catch (e) {
    const off = e.status === 409 && /connect/i.test(e.message);
    st.innerHTML = `<div class="alert bad">${esc(e.message)}${off ? ` <a class="btn sm" href="#/project/${esc(s.slug)}/hosting">Open Hosting</a>` : ""}</div>`;
  } finally { deployBusy = false; }
}

// ---------- drop anywhere on a project page ----------
let dropDepth = 0;
function onProjectPage() { const m = /^#\/project\/([^/]+)/.exec(location.hash); return m && !/\/domain$/.test(location.hash) ? decodeURIComponent(m[1]) : ""; }
function showDropOverlay(name) {
  let o = $("#dropAll");
  if (!o) { o = h(`<div id="dropAll" class="dropall" aria-hidden="true"><div class="inner"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M12 16V4M7 9l5-5 5 5"/><path d="M4 16v3a1 1 0 001 1h14a1 1 0 001-1v-3"/></svg><div class="big"></div><div class="small">Release to deploy. Hold <b>Shift</b> to save a draft instead.</div></div></div>`); document.body.appendChild(o); }
  $(".big", o).textContent = `Drop to deploy to ${name}`;
  o.classList.add("on");
}
function hideDropOverlay() { dropDepth = 0; const o = $("#dropAll"); if (o) o.classList.remove("on"); }
document.addEventListener("dragenter", (e) => {
  const slug = onProjectPage();
  if (!slug || !e.dataTransfer || !Array.from(e.dataTransfer.types || []).includes("Files") || $(".modal-back")) return;
  dropDepth++;
  const s = sites.find(x => x.slug === slug);
  showDropOverlay(s ? s.name : slug);
});
document.addEventListener("dragover", (e) => { if (onProjectPage() && $("#dropAll.on")) e.preventDefault(); });
document.addEventListener("dragleave", () => { if (--dropDepth <= 0) hideDropOverlay(); });
document.addEventListener("drop", async (e) => {
  const slug = onProjectPage();
  if (!slug || !$("#dropAll.on")) return;
  e.preventDefault();
  hideDropOverlay();
  const shift = e.shiftKey;
  const sel = await readDropped(e.dataTransfer);
  const s = await getSite(slug);
  let zone = $("#pBody .dzwrap");
  if (!zone) { go(`#/project/${slug}`); await new Promise(r => setTimeout(r, 400)); zone = $("#pBody .dzwrap"); }
  if (zone) { const c = $("[data-i=draft]", zone); if (c && shift) c.checked = true; zone.scrollIntoView({ block: "center" }); zone._run(sel); }
  else runDeploy(s, sel, $("#pBody"), { mode: shift ? "draft" : "deploy" });
});

// ---------- paste a folder path to deploy ----------
document.addEventListener("paste", async (e) => {
  const slug = onProjectPage();
  if (!slug || (e.target.closest && e.target.closest("input, textarea, [contenteditable]")) || $(".modal-back")) return;
  const p = looksLikePath((e.clipboardData || window.clipboardData).getData("text"));
  if (!p) return;
  e.preventDefault();
  const s = await getSite(slug);
  if (!await confirmBox(`Deploy this folder to ${s.name}?`, `<p><code>${esc(p)}</code></p><p class="small muted">Rover checks the files first and keeps your current version for rollback.</p>`, "Deploy")) return;
  let zone = $("#pBody .dzwrap");
  if (!zone) { go(`#/project/${slug}`); await new Promise(r => setTimeout(r, 400)); zone = $("#pBody .dzwrap"); }
  if (zone) zone._run({ path: p }); else runDeploy(s, { path: p }, $("#pBody"), {});
});

// ---------- redeploy last folder ----------
async function redeployLast(s, c, el) {
  const p = c.watch_path || c.last_source || (s.watch_path || "");
  if (!p) { toast("Deploy from a folder path once (or use the Explorer menu); then this button redeploys it in one click.", "info"); return; }
  if (!await confirmBox(`Redeploy ${s.name}?`, `<p>Publish the files in <code>${esc(p)}</code> again. Only changed files are sent.</p>`, "Redeploy")) return;
  const zone = $("#pBody .dzwrap");
  if (zone) { zone.scrollIntoView({ block: "center" }); zone._run({ path: p }); } else runDeploy(s, { path: p }, el, {});
}

async function rollbackPrev(s) {
  const r = await api("GET", `/api/sites/${s.slug}/releases`);
  const rel = (r.releases || []).filter(x => /^\d{8}-\d{6}$/.test(x)).sort().reverse();
  const i = rel.indexOf(r.current), prev = rel[i + 1];
  if (!prev) { toast("There is no earlier version on the server to go back to.", "info"); return; }
  if (!await confirmBox("Roll back to the previous version?", `<p>Make the version from <b>${esc(relTime(prev))}</b> (<code>${esc(prev)}</code>) live again. It switches instantly; you can switch forward again from Deploys.</p>`, "Roll back")) return;
  await api("POST", `/api/sites/${s.slug}/rollback`, { release: prev });
  delete lastDeployResult[s.slug];
  toast("Rolled back. The previous version is live.", "success");
  rerender();
}
document.addEventListener("click", (e) => {
  const b = e.target.closest("[data-undo]"); if (!b) return;
  const slug = onProjectPage(); if (!slug) return;
  e.preventDefault(); e.stopImmediatePropagation();
  getSite(slug).then(s => busy(b, () => rollbackPrev(s))).catch(err);
}, true);

// ---------- Overview ----------
function checklistItems(s, c, srv, bk) {
  const base = `#/project/${s.slug}`;
  return [
    { k: "host", t: "Add hosting (a server)", d: srv ? `Hosted on ${srv.label}` : "Connect a VPS or a hosting account", done: !!srv, href: `${base}/hosting`, btn: "Add hosting" },
    { k: "deploy", t: "Publish your site", d: s.last_deploy ? `Last deploy ${ago(s.last_deploy)}` : "Drop your build folder above", done: !!s.last_deploy, href: `${base}/deploys`, btn: "Deploy" },
    { k: "domain", t: "Add your domain", d: s.domain ? s.domain : "Use a name like yourshop.com", done: !!s.domain, href: `${base}/domain`, btn: "Add domain" },
    { k: "https", t: "Turn on HTTPS (padlock)", d: s.https_active ? "Certificate active" : s.domain_live ? "Waiting for the certificate (automatic)" : "Starts automatically once the domain points here", done: !!s.https_active, href: `${base}/domain`, btn: s.domain ? "Check" : "Set up", wait: !!s.domain_live && !s.https_active },
    { k: "git", t: "Connect Git (optional)", d: s.git ? `${s.git.display} (${s.git.branch || "default branch"})` : "Deploy automatically on every push", done: !!s.git, href: s.git ? `${base}/config/build` : "#/new/git", btn: "Connect", opt: true },
    { k: "alerts", t: "Set up alerts", d: c.alert_down && c.alert_fail ? "You'll be told when the site is down or a deploy fails" : "Some alerts are off", done: c.alert_down && c.alert_fail, href: `${base}/config/alerts`, btn: "Alerts" },
    { k: "backup", t: "Backups", d: bk && bk.every !== "off" && !c.no_backup ? `${bk.every === "daily" ? "Daily" : "Weekly"} backups on this PC` : c.no_backup ? "This project is left out of backups" : "Backups are off", done: !!(bk && bk.every !== "off" && !c.no_backup), href: `${base}/config/backups`, btn: "Backups" },
  ];
}
function lockNote(on) {
  return on ? `<div class="alert info small" style="margin:0 0 12px">&#128274; <b>Publishing is locked.</b> Deploys from a watched folder or Git are saved as <b>drafts</b>; publish one below when you are ready. What you drop here still goes live (tick <i>Save as a draft</i> to keep it as a draft).</div>` : "";
}
async function overviewTab(s, b) {
  const url = liveURL(s);
  b.innerHTML = `<div class="card ovtop"><div class="ovgrid">
      <div><div class="small muted">Live address</div>${url ? `<a class="lurl" href="${esc(url)}" target="_blank" rel="noopener">${esc(url.replace(/^https?:\/\//, ""))} &#8599;</a>` : `<span class="muted">Not online yet: drop your site below</span>`}
        <div class="small muted" id="ovMeta">${s.last_deploy ? `Last deploy ${esc(ago(s.last_deploy))}` : "Never deployed"}${s.framework ? ` &middot; ${esc(s.framework)}` : ""}${s.locked ? ` &middot; <span class="badge warn">Publishing locked</span>` : ""}</div></div>
      <div class="row qa"><button id="qRe" class="sm" title="Publish the same folder again (R)">&#8635; Redeploy last folder</button>${url ? `<a class="btn sm" href="${esc(url)}" target="_blank" rel="noopener" title="Open site (O)">Open site</a>${copyBtn(url)}` : ""}${s.last_deploy ? `<button class="sm ghost" id="qRb" title="Go back to the previous version">&#8630; Roll back</button>` : ""}<a class="btn sm ghost" href="#/project/${esc(s.slug)}/config">Settings</a></div>
    </div></div>
    <div class="ovcols"><div class="card"><div class="card-head"><h2>Deploy a new version</h2><span class="grow"></span><span class="small muted">or drop anywhere on this page</span></div><div class="dzwrap" id="ovDz"></div></div>
      <div class="card" id="ovCheck"><h2>Set up this project</h2><div class="muted small">Loading&hellip;</div></div></div>
    <div id="ovRest"></div>`;
  deployZone($("#ovDz"), s, { onDone: () => later(rerender, 2500) });
  if ($("#qRb")) $("#qRb").onclick = (e) => busy(e.target, () => rollbackPrev(s));
  const rest = $("#ovRest");
  projectOverviewV05(s, rest);
  const first = rest.querySelector(".grid2"); if (first) first.remove();
  const [c, sv, bk] = await Promise.all([siteCfg(s).catch(() => ({})), api("GET", "/api/servers").catch(() => ({})), api("GET", "/api/backup").catch(() => ({}))]);
  const srv = (sv.servers || []).find(x => x.id === s.server) || (sv.servers || []).find(x => x.active);
  $("#qRe").onclick = () => redeployLast(s, c, $("#ovDz .dzStatus"));
  if (!(c.watch_path || c.last_source)) { if (!s.last_deploy) $("#qRe").remove(); else $("#qRe").title = "Deploy from a folder path once to enable one-click redeploys"; }
  const items = checklistItems(s, c, srv, bk.settings);
  const req = items.filter(i => !i.opt), n = req.filter(i => i.done).length;
  const el = $("#ovCheck");
  if (!el) return;
  el.innerHTML = `<div class="card-head"><h2>Set up this project</h2><span class="grow"></span><span class="small muted">${n} of ${req.length} done</span></div><div class="progress thin"><div style="width:${Math.round(n / req.length * 100)}%"></div></div>
    <ol class="chk">${items.map(i => `<li class="${i.done ? "done" : i.wait ? "wait" : ""}"><span class="ic" aria-hidden="true">${i.done ? "&#10004;" : i.wait ? "&#8987;" : "&#9675;"}</span><span class="grow"><b>${esc(i.t)}</b><span class="small muted">${esc(i.d)}</span></span>${i.done ? `<a class="small" href="${esc(i.href)}">Manage</a>` : `<a class="btn sm ${i.opt ? "ghost" : "primary"}" href="${esc(i.href)}">${esc(i.btn)}</a>`}</li>`).join("")}</ol>`;
}

// ---------- Deploys ----------
async function deploysTab(s, b) {
  b.innerHTML = `<div class="card"><div class="card-head"><h2>Deploy a new version</h2><span class="grow"></span><label class="switch small" title="Like Netlify's locked deploys: watched-folder and Git deploys are saved as drafts until you publish them"><input type="checkbox" id="dLock" ${s.locked ? "checked" : ""}> Lock publishing</label></div><div id="dLockNote">${lockNote(s.locked)}</div><div class="dzwrap" id="dDz"></div></div><div id="dOld"></div>`;
  deployZone($("#dDz"), s, { onDone: () => later(rerender, 1500) });
  $("#dLock").onchange = (e) => saveCfg(s, { locked: e.target.checked }, e.target.checked ? "Publishing locked: automatic deploys are saved as drafts" : "Publishing unlocked").then(() => { $("#dLockNote").innerHTML = lockNote(e.target.checked); return loadSites(); }).catch(x => { e.target.checked = !e.target.checked; err(x); });
  await projectDeploysV05(s, $("#dOld"));
  // Drafts: badge + Preview, and clearer labels.
  let r; try { r = await api("GET", `/api/projects/${s.slug}/deploys`); } catch (e) { return; }
  const byId = {}; (r.deploys || []).forEach(d => byId[d.id] = d);
  $$("#dList li[data-id]").forEach(li => {
    const d = byId[li.dataset.id]; if (!d) return;
    const pub = $("[data-a=publish]", li);
    if (pub) { pub.textContent = d.status === "draft" ? "Publish this deploy" : "Publish (roll back)"; }
    if (d.status === "draft") {
      const g = $(".grow b", li); if (g) g.insertAdjacentHTML("afterend", ` <span class="badge info">Draft</span>`);
      const prev = h(`<button class="sm" data-pv="${esc(d.id)}" title="Open this draft in App Testing">Preview</button>`);
      prev.onclick = (e) => busy(e.target, async () => { await api("POST", `/api/projects/${s.slug}/deploys/${d.id}/preview`); store("testSite", s.slug); toast("Draft opened in App Testing", "success"); go("#/testing"); });
      const row = $(".row", li); if (row) row.insertBefore(prev, row.firstChild);
    }
  });
  const head = $("#dOld .card-head h2"); if (head) head.textContent = "Deploy history";
}

// ---------- Hosting ----------
async function hostingTab(s, b) {
  b.innerHTML = `<div class="card"><div class="card-head"><h2>Hosting for this project</h2><span class="grow"></span><a class="btn sm" href="#/hosting?add=">+ Add a hosting account</a></div><div id="hSrv" class="muted small">Loading&hellip;</div></div>
    <div id="hHealth"></div>
    <div class="card"><h2>Server health</h2><div id="hSH" class="small muted">Loading&hellip;</div></div>
    <div class="card"><h2>Move to another server</h2><div id="hMove" class="small muted">Loading&hellip;</div></div>`;
  projectHealthCard(s, $("#hHealth"));
  let sv; try { sv = await api("GET", "/api/servers"); } catch (e) { $("#hSrv").innerHTML = `<div class="alert warn">${esc(e.message)}</div>`; return; }
  const list = sv.servers || [];
  const cur = list.find(x => x.id === s.server) || list.find(x => x.active);
  if (!cur) {
    $("#hSrv").innerHTML = `<div class="empty"><p><b>No server yet.</b> A server is the computer on the internet that shows your site. Connect one in a few clicks:</p><div class="row center"><a class="btn primary" href="#/hosting?add=">Add a hosting account</a><a class="btn" href="#/setup">I already have a VPS</a></div></div>`;
    $("#hSH").textContent = "Connect a server first."; $("#hMove").textContent = "Connect a server first."; return;
  }
  $("#hSrv").innerHTML = `<div class="psrv6"><div class="grow"><div class="big">${esc(cur.label)}</div><div class="small muted">${esc(cur.ip)}${cur.provider ? ` &middot; ${esc(cur.provider)}` : ""} &middot; ${cur.projects} project${cur.projects === 1 ? "" : "s"}</div></div>
      <span class="badge ${cur.active ? (cur.connected ? "ok" : "warn") : ""}">${cur.active ? (cur.connected ? "Connected" : "Not connected") : "Not the active server"}</span></div>
    <div class="row" style="margin-top:10px">${cur.active ? "" : `<button id="hAct" class="primary sm">Switch to this server</button>`}<a class="btn sm ghost" href="#/hosting">All hosting accounts</a><button id="hRestart" class="sm">Restart the site</button></div><div id="hJob"></div>`;
  if ($("#hAct")) $("#hAct").onclick = (e) => busy(e.target, async () => { await api("POST", `/api/servers/${cur.id}/activate`); await refreshOverview(); drawServerChip(); rerender(); });
  $("#hRestart").onclick = (e) => busy(e.target, async () => { if (!await confirmBox("Restart the site?", "The small web server that shows this site is restarted. The site is offline for a few seconds.", "Restart")) return; await api("POST", `/api/sites/${s.slug}/restart`); toast("Restart queued", "success"); });
  const sh = async () => {
    $("#hSH").innerHTML = `<span class="spin"></span> Checking&hellip;`;
    const t = await api("GET", "/api/coolify/test").catch(e => ({ error: e.message }));
    $("#hSH").innerHTML = `<ul class="hlist"><li>${cur.connected ? "&#10004;" : "&#9888;"} SSH connection: <b>${cur.connected ? "working" : "not connected"}</b>${cur.connected ? "" : ` <button class="sm" id="hRe">Reconnect</button>`}</li><li>${t.error ? "&#9888;" : "&#10004;"} Hosting engine (Coolify): <b>${t.error ? esc(t.error) : "version " + esc(t.version || "?")}</b></li></ul><div class="row"><button id="hShR" class="sm">Check again</button><a class="btn sm ghost" href="#/settings/server">Server settings</a></div>`;
    $("#hShR").onclick = (e) => busy(e.target, sh);
    if ($("#hRe")) $("#hRe").onclick = (e) => reconnect(e.target);
  };
  if (cur.active) sh(); else $("#hSH").textContent = "Switch to this server to check it.";
  const others = list.filter(x => x !== cur);
  $("#hMove").innerHTML = others.length ? `<div class="row"><select id="hTo" style="max-width:340px">${others.map(x => `<option value="${esc(x.id)}">${esc(x.label)} (${esc(x.ip)})</option>`).join("")}</select><button id="hMv">Move this project&hellip;</button></div><p class="hint">Rover sets the project up on the other server and publishes its last deploy there. Then point the domain to the new IP (Rover shows it). The old copy keeps running until you delete it.</p><div id="hMvJob"></div>`
    : `<p>You have one server. <a href="#/hosting?add=">Add another hosting account</a> to move projects between servers.</p>`;
  if ($("#hMv")) $("#hMv").onclick = async (e) => {
    const to = others.find(x => x.id === $("#hTo").value);
    if (!await confirmBox(`Move ${s.name} to ${to.label}?`, `<p>Rover switches to <b>${esc(to.label)}</b>, creates the project there and publishes the last deploy from this PC.${s.domain ? ` Afterwards point <b>${esc(s.domain)}</b> to <code>${esc(to.ip)}</code>.` : ""}</p>`, "Move project")) return;
    busy(e.target, async () => { const r = await api("POST", `/api/sites/${s.slug}/server`, { server: to.id }, { noSwitch: true }); const j = await jobPanel(r.job, $("#hMvJob"), { title: "Moving", doneTitle: "Moved" }); await refreshOverview(); drawServerChip(); if (!j.error) later(rerender, 800); });
  };
}

// ---------- Testing (project-scoped) ----------
function testingTab(s, b) {
  store("testSite", s.slug);
  b.innerHTML = `<div class="card"><h2>Test a version of ${esc(s.name)}</h2><p class="small muted">Try a new build on this PC first: Rover opens it in App Testing with a live preview, link checks and rule checks. Nothing is published until you click <b>Publish</b> there.</p><div class="dzwrap" id="tDz"></div></div>
    <div class="card"><h2>Other ways to test</h2><ul class="small"><li>Save a deploy as a <b>draft</b> (tick the box under the drop area), then <b>Preview</b> it in <a href="#/project/${esc(s.slug)}/deploys">Deploys</a>.</li><li><b>Lock publishing</b> in Deploys to turn watched-folder and Git deploys into drafts.</li><li><a href="#/testing">Open App Testing</a> to see what is loaded now.</li></ul></div>`;
  deployZone($("#tDz"), s, { mode: "test" });
}

// ---------- Configuration ----------
async function configTab(s, b, sub) {
  b.innerHTML = `<div class="muted small"><span class="spin"></span> Loading&hellip;</div>`;
  let c; try { c = await siteCfg(s); } catch (e) { b.innerHTML = `<div class="alert bad">${esc(e.message)}</div>`; return; }
  const f = { general: cfgGeneral, build: cfgBuild, rules: cfgRules, access: cfgAccess, alerts: cfgAlerts, backups: cfgBackups, danger: cfgDanger }[sub] || cfgGeneral;
  b.innerHTML = "";
  return f(s, b, c);
}
function cfgGeneral(s, b) {
  b.innerHTML = `<div class="card"><h2>General</h2><div class="field"><label for="gName">Project name</label><div class="row"><input id="gName" value="${esc(s.name)}" style="flex:1"><button id="gRen">Rename</button></div><p class="hint">Only the name you see changes; the address and files stay the same.</p></div>
      <div class="kv small muted"><div>Project id</div><div><code>${esc(s.slug)}</code></div><div>Created</div><div>${esc(ago(s.created_at))}</div><div>Last deploy</div><div>${esc(s.last_deploy ? ago(s.last_deploy) : "never")}</div><div>Folder on this PC</div><div><code id="gDir">&hellip;</code> <button class="sm ghost" id="gOpen">Open</button></div><div>Coolify app</div><div><code>${esc(s.app_uuid || "")}</code></div></div></div><div id="gOrg"></div>`;
  projectOrganise(s, $("#gOrg"));
  $("#gRen").onclick = (e) => busy(e.target, async () => { await api("POST", `/api/sites/${s.slug}/rename`, { name: $("#gName").value }); toast("Renamed", "success"); await loadSites(); rerender(); });
  $("#gOpen").onclick = (e) => busy(e.target, () => api("POST", `/api/projects/${s.slug}/open`, { what: "folder" }));
  api("GET", `/api/projects/${s.slug}/deploys`).then(r => $("#gDir").textContent = r.dir).catch(() => {});
}
function cfgBuild(s, b, c) {
  const g = s.git;
  b.innerHTML = `<div class="card"><h2>Build &amp; deploy</h2>
      <div class="kv small" style="margin-bottom:10px"><div>Framework</div><div>${esc(c.framework || "Plain HTML or not detected yet")}</div></div>
      <div class="field"><label for="bPub">Publish folder <span class="muted small">(the folder that contains index.html)</span></label><div class="row"><input id="bPub" value="${esc(c.publish_dir)}" placeholder="${esc(c.detected_publish || "detected automatically")}" style="flex:1"><button id="bPubS">Save</button></div><p class="hint">Leave empty and Rover finds it (dist, build, public, out&hellip;). Set it if your upload contains several folders.</p></div>
      <div class="field"><label for="bCmd">Build command <span class="muted small">(Git projects built on this PC)</span></label><div class="row"><input id="bCmd" value="${esc(c.build_cmd)}" placeholder="npm run build" list="bCmdL" style="flex:1"><datalist id="bCmdL"><option value="npm run build"><option value="npm run generate"><option value="npm run export"></datalist><button id="bCmdS">Save</button></div><p class="hint">Only <code>npm run &lt;script&gt;</code> is allowed, for safety. Dropped folders are published as they are.</p></div>
      <label class="switch"><input type="checkbox" id="bLock" ${c.locked ? "checked" : ""}> Lock publishing <span class="muted small">(watched-folder and Git deploys are saved as drafts until you publish them)</span></label>
      <label class="switch"><input type="checkbox" id="bInc" ${c.incremental ? "checked" : ""}> Fast deploys: send only changed files <span class="muted small">(all projects)</span></label></div>
    <div class="card"><h2>Watch a folder</h2><label class="switch"><input type="checkbox" id="bWatch" ${s.watch ? "checked" : ""}> Redeploy automatically when files in this folder change</label>
      <div class="row" style="margin-left:52px"><input id="bWPath" value="${esc(s.watch_path || c.last_source || "")}" placeholder="C:\\Sites\\${esc(s.slug)}\\dist" style="flex:1" list="bRecent"><datalist id="bRecent">${(c.recent || []).map(p => `<option value="${esc(p)}">`).join("")}</datalist></div></div>
    <div class="card"><h2>Git</h2>${g ? `<p><span class="badge accent">${esc(g.kind)}</span> <b>${esc(g.display)}</b> &middot; branch <code>${esc(g.branch || "default")}</code>${g.last_sha ? ` &middot; deployed <code>${esc(g.last_sha.slice(0, 7))}</code>` : ""}</p>
      <label class="switch"><input type="checkbox" id="gAuto" ${g.auto_deploy ? "checked" : ""}> Deploy automatically when the repository changes</label>
      <label class="switch"><input type="checkbox" id="gTest" ${g.test_first ? "checked" : ""}> Test before publishing automatic deploys</label>
      <div class="row"><button id="gPull" class="primary sm">Pull latest and deploy</button><button id="gUnlink" class="ghost sm">Unlink repository</button></div><div id="gJob"></div>`
      : `<p class="small">This project is not connected to a repository. Connect one to deploy automatically on every push (GitHub, GitLab, Bitbucket, Azure DevOps, Gitea).</p><a class="btn" href="#/new/git">Import from Git</a>`}</div>
    <div class="card"><h2>Recent folders</h2>${(c.recent || []).length ? `<ul class="deploys">${c.recent.map(p => `<li><span class="grow"><code>${esc(p)}</code></span><button class="sm" data-rp="${esc(p)}">Deploy this folder</button></li>`).join("")}</ul>` : `<p class="small muted">Folders you deploy from by path appear here for one-click redeploys.</p>`}<div class="dzStatus" id="bRun"></div></div>`;
  $("#bPubS").onclick = (e) => busy(e.target, () => saveCfg(s, { publish_dir: $("#bPub").value }).then(r => $("#bPub").value = r.publish_dir));
  $("#bCmdS").onclick = (e) => busy(e.target, () => saveCfg(s, { build_cmd: $("#bCmd").value }).then(r => $("#bCmd").value = r.build_cmd));
  $("#bLock").onchange = (e) => saveCfg(s, { locked: e.target.checked }).then(() => loadSites()).catch(x => { e.target.checked = !e.target.checked; err(x); });
  $("#bInc").onchange = (e) => saveCfg(s, { incremental: e.target.checked }, e.target.checked ? "Fast deploys on" : "Every deploy now sends all files").catch(err);
  $("#bWatch").onchange = (e) => api("POST", `/api/sites/${s.slug}/watch`, { enabled: e.target.checked, path: $("#bWPath").value.trim() }).then(() => toast(e.target.checked ? "Watching the folder: changes deploy automatically" : "Stopped watching", "success")).catch(x => { e.target.checked = !e.target.checked; err(x); });
  $$("[data-rp]", b).forEach(x => x.onclick = async () => { if (!await confirmBox(`Deploy to ${s.name}?`, `<p>Publish <code>${esc(x.dataset.rp)}</code> now.</p>`, "Deploy")) return; runDeploy(s, { path: x.dataset.rp }, $("#bRun"), {}); });
  if (g) {
    const save = (body) => api("POST", `/api/sites/${s.slug}/git`, body).then(() => toast("Saved", "success")).catch(err);
    $("#gAuto").onchange = () => save({ auto_deploy: $("#gAuto").checked });
    $("#gTest").onchange = () => save({ test_first: $("#gTest").checked });
    $("#gPull").onclick = (e) => busy(e.target, async () => pullDeploy(s.slug, false, $("#gJob")));
    $("#gUnlink").onclick = async () => { if (await confirmBox("Unlink the repository?", "The site stays online. Automatic deploys from the repository stop.", "Unlink")) { await save({ unlink: true }); await loadSites(); rerender(); } };
  }
}
const ruleSnippets = [
  ["Single-page app (React, Vue)", "r", "/*    /index.html   200"],
  ["Redirect an old page", "r", "/old-page    /new-page    301"],
  ["Move a whole folder", "r", "/blog/*    /news/:splat    301"],
  ["Security headers", "h", "/*\n  X-Frame-Options: DENY\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin"],
  ["Cache images for a year", "h", "/images/*\n  Cache-Control: public, max-age=31536000, immutable"],
];
async function cfgRules(s, b) {
  b.innerHTML = `<div class="card"><div class="card-head"><h2>Redirects &amp; headers</h2><span class="grow"></span><span class="small muted" id="rWhere"></span></div>
      <p class="small muted">The same rules as Netlify's <code>_redirects</code> and <code>_headers</code> files. Rover checks every line and turns them into the web server's settings.</p>
      <div class="row wrap snips">${ruleSnippets.map((x, i) => `<button class="sm ghost" data-sn="${i}">+ ${esc(x[0])}</button>`).join("")}</div>
      <div class="grid2"><div class="field"><label for="rRed">Redirects <code>_redirects</code></label><textarea id="rRed" rows="10" class="code" spellcheck="false" placeholder="/old   /new   301"></textarea></div>
        <div class="field"><label for="rHdr">Headers <code>_headers</code></label><textarea id="rHdr" rows="10" class="code" spellcheck="false" placeholder="/*\n  X-Frame-Options: DENY"></textarea></div></div>
      <div id="rIss"></div>
      <div class="row"><button id="rChk">Check</button><button id="rSave">Save</button><button id="rPub" class="primary">Save and publish</button><a class="small" href="#/project/${esc(s.slug)}/files">Edit other files</a></div><div id="rJob"></div></div>`;
  let r; try { r = await api("GET", `/api/projects/${s.slug}/rules`); } catch (e) { $("#rIss").innerHTML = `<div class="alert bad">${esc(e.message)}</div>`; return; }
  $("#rRed").value = r.redirects || ""; $("#rHdr").value = r.headers || "";
  $("#rWhere").textContent = r.has_files ? `in ${r.publish ? r.publish + "/" : "the top folder"} of your current files` : "deploy once to edit rules";
  if (r.has_toml) $("#rIss").innerHTML = `<div class="alert info small">This project also has a <code>netlify.toml</code>; its rules are applied too.</div>`;
  const showIss = (iss) => { $("#rIss").innerHTML = (iss || []).length ? `<ul class="issues">${iss.map(i => `<li class="${esc(i.level)}"><span class="pill ${esc(i.level)}">${esc(i.level)}</span> <code>${esc(i.source)}${i.line ? ":" + i.line : ""}</code> ${esc(i.msg)}${i.fix ? `<div class="small muted">${esc(i.fix)}</div>` : ""}</li>`).join("")}</ul>` : `<div class="alert ok small">&#10004; All rules look good.</div>`; };
  const body = (check) => ({ redirects: $("#rRed").value, headers: $("#rHdr").value, check_only: check });
  $$("[data-sn]", b).forEach(x => x.onclick = () => { const [, k, t] = ruleSnippets[+x.dataset.sn]; const ta = k === "r" ? $("#rRed") : $("#rHdr"); ta.value = (ta.value.trim() ? ta.value.replace(/\s*$/, "\n") : "") + t + "\n"; ta.focus(); });
  $("#rChk").onclick = (e) => busy(e.target, async () => showIss((await api("POST", `/api/projects/${s.slug}/rules`, body(true))).issues));
  const save = async () => { const x = await api("POST", `/api/projects/${s.slug}/rules`, body(false)); showIss(x.issues); if (!x.saved) { toast(`Fix the ${x.fails} problem(s) first`, "error"); return false; } toast("Rules saved on this PC", "success"); return true; };
  $("#rSave").onclick = (e) => busy(e.target, save);
  $("#rPub").onclick = (e) => busy(e.target, async () => { if (await save()) await applyNow(s, $("#rJob"), "Publish the new rules with the current files of"); });
}
function genPassword() { const a = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"; const b = new Uint32Array(14); crypto.getRandomValues(b); return Array.from(b, x => a[x % a.length]).join("").replace(/(.{4})(?=.)/g, "$1-"); }
function cfgAccess(s, b, c) {
  b.innerHTML = `<div class="card"><h2>Access &amp; password</h2>
      <p class="small muted">Ask visitors for a user name and password before they see the site, for example while it is not finished or for a client preview. Search engines cannot see protected pages.</p>
      <div class="alert ${c.access_on ? "warn" : "info"} small">${c.access_on ? `&#128274; Password protection is <b>on</b> (user <code>${esc(c.access_user)}</code>).` : "The site is public: anyone with the address can open it."}</div>
      <label class="switch"><input type="checkbox" id="aOn" ${c.access_on ? "checked" : ""}> Protect this site with a password</label>
      <div id="aForm" class="${c.access_on ? "" : "hidden"}"><div class="grid2"><div class="field"><label for="aUser">User name</label><input id="aUser" value="${esc(c.access_user || "guest")}" autocomplete="off"></div>
        <div class="field"><label for="aPw">New password</label><div class="row"><input id="aPw" type="text" autocomplete="off" placeholder="${c.access_on ? "leave empty to keep the current one" : "at least 8 characters"}" style="flex:1"><button class="sm" id="aGen" type="button">Generate</button></div></div></div>
        <p class="hint">Rover keeps only a scrambled copy (a hash) of the password, encrypted on this PC. Write it down or share it with the people who need it.</p></div>
      <div class="row"><button id="aSave" class="primary">Save and publish</button></div><div id="aJob"></div></div>`;
  $("#aOn").onchange = () => $("#aForm").classList.toggle("hidden", !$("#aOn").checked);
  $("#aGen").onclick = () => { $("#aPw").value = genPassword(); };
  $("#aSave").onclick = (e) => busy(e.target, async () => {
    const on = $("#aOn").checked, pw = $("#aPw").value;
    if (on && !pw && !(c.access_on && $("#aUser").value.trim() === c.access_user)) { toast("Enter a password (or click Generate)", "error"); return; }
    if (on && pw) await api("POST", `/api/sites/${s.slug}/access`, { enabled: true, user: $("#aUser").value, password: pw });
    if (!on && c.access_on) await api("POST", `/api/sites/${s.slug}/access`, { enabled: false });
    if (on === c.access_on && !pw) { toast("Nothing changed", "info"); return; }
    toast(on ? "Password saved" : "Protection removed", "success");
    const j = await applyNow(s, $("#aJob"), on ? "Turn on password protection for" : "Remove password protection from");
    if (j && !j.error) later(rerender, 1200);
  });
}
function cfgAlerts(s, b, c) {
  const can = "Notification" in window;
  b.innerHTML = `<div class="card"><h2>Notifications</h2>
      <label class="switch"><input type="checkbox" id="nDown" ${c.alert_down ? "checked" : ""}> Tell me when the site stops answering (checked every 10 minutes)</label>
      <label class="switch"><input type="checkbox" id="nFail" ${c.alert_fail ? "checked" : ""}> Tell me when an automatic deploy fails</label>
      <label class="switch"><input type="checkbox" id="nSys" ${store("deployNotify") ? "checked" : ""} ${can ? "" : "disabled"}> Show a Windows notification when a deploy finishes while Rover is in the background</label>
      <p class="hint">Alerts appear in the bell at the top. The taskbar button also flashes when a deploy finishes in the background. Certificate warnings and email settings are in <a href="#/settings/backups">Backups &amp; alerts</a>.</p></div>`;
  $("#nDown").onchange = (e) => saveCfg(s, { alert_down: e.target.checked }).catch(err);
  $("#nFail").onchange = (e) => saveCfg(s, { alert_fail: e.target.checked }).catch(err);
  $("#nSys").onchange = async (e) => {
    if (e.target.checked && can && Notification.permission !== "granted") { const p = await Notification.requestPermission().catch(() => "denied"); if (p !== "granted") { e.target.checked = false; toast("Notifications are blocked for Rover in Windows settings", "error"); return; } }
    store("deployNotify", e.target.checked); toast("Saved", "success");
  };
}
async function cfgBackups(s, b, c) {
  b.innerHTML = `<div class="card"><h2>Backups</h2><div id="kSt" class="small muted">Loading&hellip;</div>
      <label class="switch"><input type="checkbox" id="kInc" ${c.no_backup ? "" : "checked"}> Include this project in scheduled backups</label>
      <div class="row"><button id="kNow">Back up now</button><a class="btn ghost" href="#/settings/backups">Backup schedule &amp; folder</a></div><div id="kJob"></div>
      <p class="hint">Backups are zip files on this PC with your projects' current files and settings (secrets stay encrypted). Every deploy is also kept in the project's deploys folder.</p></div>`;
  $("#kInc").onchange = (e) => saveCfg(s, { no_backup: !e.target.checked }).catch(err);
  $("#kNow").onclick = (e) => busy(e.target, async () => { const r = await api("POST", "/api/backup/run"); if (r.job) await jobPanel(r.job, $("#kJob"), { title: "Backing up", doneTitle: "Backup saved" }); else toast("Backup saved", "success"); });
  const r = await api("GET", "/api/backup").catch(e => ({ error: e.message }));
  const st = r.settings || {};
  $("#kSt").innerHTML = r.error ? esc(r.error) : `<p>Schedule: <b>${esc(st.every === "off" || !st.every ? "off" : st.every)}</b>${st.last ? ` &middot; last backup ${esc(ago(st.last))}` : ""}${st.last_error ? ` &middot; <span class="badge bad">last one failed</span>` : ""}</p>`;
}
function cfgDanger(s, b) {
  projectSettingsV04(s, b);
  Array.from(b.children).forEach(x => { if (!x.classList.contains("danger-zone")) x.remove(); });
  const dz = $(".danger-zone", b);
  const dzh = dz && dz.querySelector("h2, h3");
  if (dz) (dzh || dz).insertAdjacentHTML(dzh ? "afterend" : "afterbegin", `<div class="row" style="margin:6px 0 14px"><button id="xArc">${s.archived ? "Restore from archive" : "Archive project"}</button><span class="small muted">Archive hides it from your lists; the site keeps running.</span></div>`);
  $("#xArc").onclick = (e) => busy(e.target, async () => { await api("POST", `/api/sites/${s.slug}/meta`, { archived: !s.archived }); toast(s.archived ? "Restored" : "Archived. The site keeps running.", "success"); await loadSites(); rerender(); });
}

// ---------- Domains: primary domain + HTTPS ----------
const domainRoute = routes.find(r => String(r.re).includes("\\/domain$") && String(r.re).includes("project"));
if (domainRoute) {
  const orig = domainRoute.fn;
  domainRoute.fn = async (slug) => {
    await orig(slug);
    const s = sites.find(x => x.slug === slug);
    if (!s || !s.domain) return;
    const c = await siteCfg(s).catch(() => ({}));
    const card = h(`<div class="dextra"><div class="card" id="dPrim"><h2>Primary domain</h2>${s.include_www ? `<p class="small muted">Visitors should end up on one address. Pick it; the other one forwards there (good for search engines).</p>
      <div class="radios"><label class="check"><input type="radio" name="prim" value="apex" ${c.primary === "apex" ? "checked" : ""}> <b>${esc(s.domain)}</b> <span class="small muted">(www.${esc(s.domain)} forwards here)</span></label>
      <label class="check"><input type="radio" name="prim" value="www" ${c.primary === "www" ? "checked" : ""}> <b>www.${esc(s.domain)}</b> <span class="small muted">(${esc(s.domain)} forwards here)</span></label>
      <label class="check"><input type="radio" name="prim" value="" ${!c.primary ? "checked" : ""}> Both work, no forwarding</label></div>
      <div class="row"><button id="pSave" class="primary">Save and publish</button></div><div id="pJob"></div>` : `<p class="small">Only <b>${esc(s.domain)}</b> is connected. Tick <b>also www</b> above and continue to add www.${esc(s.domain)} too.</p>`}</div>
      <div class="card"><h2>HTTPS</h2><p>${s.https_active ? `<span class="badge ok">&#128274; Active</span> Your certificate renews by itself.` : s.domain_live ? `<span class="badge warn">Waiting</span> The certificate is requested automatically. This takes a few minutes after DNS points here.` : `<span class="badge">Not yet</span> HTTPS turns on by itself once the domain points to your server.`}</p></div></div>`);
    const tgt = $("#dStep3") || $("#pBody") || view();
    tgt.after ? tgt.after(card) : view().appendChild(card);
    const ps = $("#pSave", card);
    if (ps) ps.onclick = (e) => busy(e.target, async () => {
      const v = ($("input[name=prim]:checked", card) || {}).value || "";
      await saveCfg(s, { primary: v }, false);
      await applyNow(s, $("#pJob", card), "Apply the primary domain to");
    });
  };
}

// ---------- Updates (GitHub Releases) ----------
let upd = null, updDialogShown = false;
function updWaiting(u) { const i = u && u.info; return !!(i && i.newer && !i.skipped && !i.snoozed); }
function updLine(i) {
  if (!i) return `<span class="muted">Not checked yet.</span>`;
  if (i.offline) return `<span class="badge">Offline</span> ${esc(i.error)}`;
  if (i.error) return `<span class="badge warn">Could not check</span> ${esc(i.error)}`;
  if (i.no_releases) return `<span class="badge">No releases yet</span> Nothing is published on GitHub yet.`;
  if (i.newer) return `<span class="badge info">Rover ${esc(i.latest)} is available</span>${i.prerelease ? ` <span class="badge warn">beta</span>` : ""}${i.skipped ? ` <span class="small muted">(you skipped this version)</span>` : ""}`;
  return `<span class="badge ok">&#10004; You're up to date</span> <span class="small muted">(newest is ${esc(i.latest)})</span>`;
}
async function loadUpd() { try { upd = await api("GET", "/api/update/status"); } catch (e) { upd = null; } drawUpdBadge(); return upd; }
function drawUpdBadge() {
  const a = $('#nav a[data-route="settings"]'); if (!a) return;
  let dot = $(".nbadge", a);
  const on = updWaiting(upd);
  if (on && !dot) { dot = h(`<span class="nbadge" title="Update available">1</span>`); a.appendChild(dot); }
  if (!on && dot) dot.remove();
  let pill = $("#updPill");
  if (on && !pill) {
    pill = h(`<button id="updPill" class="updpill" type="button"><span aria-hidden="true">&#11014;</span> <span class="lbl">Update available</span></button>`);
    pill.onclick = () => updateDialog(upd.info);
    const nav = $("#nav"); nav.after(pill);
  }
  if (on && pill) $(".lbl", pill).textContent = `Update to ${upd.info.latest}`;
  if (!on && pill) pill.remove();
}
function updateDialog(i) {
  if (!i) return;
  const size = i.asset_size ? ` (${humanSize(i.asset_size)})` : "";
  modal(`<h2>Rover ${esc(i.latest)} is available</h2><p class="small muted">You have ${esc(i.current)}.${i.published ? ` Released ${esc(ago(i.published))}.` : ""}${i.prerelease ? ` <span class="badge warn">beta</span>` : ""}</p>
    <div class="relnotes">${i.notes ? md(i.notes) : "<p>No release notes.</p>"}</div>
    ${i.kind ? `<p class="small muted">Rover downloads ${esc(i.asset)}${size} from GitHub, checks it against the release's SHA-256 list and ${i.kind === "installer" ? "runs the installer quietly" : "replaces the program"}, then restarts by itself. Your projects and settings stay as they are.</p>` : `<div class="alert info small">This copy can't install updates by itself. Download the new version from GitHub.</div>`}
    <div id="updJob"></div>
    <div class="row end"><a class="btn ghost" href="${esc(i.page || "#")}" target="_blank" rel="noopener">View on GitHub</a><button id="uSkip" class="ghost">Skip this version</button><button id="uLater">Remind me later</button>${i.kind ? `<button id="uNow" class="primary">Install and restart</button>` : ""}</div>`, (m, close) => {
    $("#uLater", m).onclick = async () => { await api("POST", "/api/update/later", { hours: 24 }).catch(() => {}); close(); await loadUpd(); toast("We'll remind you tomorrow", "info"); };
    $("#uSkip", m).onclick = async () => { await api("POST", "/api/update/skip", { version: i.latest }).catch(() => {}); close(); await loadUpd(); toast(`Skipped ${i.latest}. Newer versions will still be offered.`, "info"); };
    if ($("#uNow", m)) $("#uNow", m).onclick = (e) => busy(e.target, async () => {
      const r = await api("POST", "/api/update/apply", { version: i.latest });
      $$("#uSkip, #uLater", m).forEach(x => x.disabled = true);
      const j = await jobPanel(r.job, $("#updJob", m), { title: "Downloading the update", doneTitle: "Installed: Rover restarts now", failTitle: "The update was not installed" });
      if (!j.error) { e.target.textContent = "Restarting…"; toast("Rover is restarting", "success"); waitForRestart(); }
    });
  });
}
function waitForRestart() {
  let n = 0;
  const t = setInterval(async () => {
    n++;
    try { const r = await fetch("/api/update/status", { headers: { "X-Rover": "1" } }); if (r.ok) { const x = await r.json(); if (x.current !== (upd && upd.current)) { clearInterval(t); location.reload(); } } } catch (e) {}
    if (n > 90) clearInterval(t);
  }, 2000);
}
setUpdates = async function (el) {
  el.innerHTML = `<div class="card"><h2>Rover updates</h2><p>You have version <b>${esc(ov.version)}</b>. <span id="uRes" class="muted small">Loading&hellip;</span></p>
      <div class="row"><button id="uCheck" class="primary">Check for updates now</button><button id="uShow" class="hidden">See what's new and install</button><span class="small muted" id="uWhen"></span></div><div id="uJob"></div></div>
    <div class="card"><h2>Automatic updates</h2>
      <label class="switch"><input type="checkbox" id="uAuto"> Check for updates automatically <span class="muted small">(when Rover starts and once a day)</span></label>
      <div class="field" style="margin-left:52px"><div class="radios"><label class="check"><input type="radio" name="uInst" value="ask"> Ask me before installing <span class="small muted">(shows what's new first)</span></label><label class="check"><input type="radio" name="uInst" value="auto"> Download and install automatically <span class="small muted">(Rover restarts by itself when nothing is running)</span></label></div></div>
      <div class="field"><label for="uChan">Update channel</label><select id="uChan" style="max-width:360px"><option value="stable">Stable: tested releases (recommended)</option><option value="beta">Beta: also try pre-releases early</option></select></div>
      <details><summary class="small muted" style="cursor:pointer">Where updates come from</summary><div style="margin-top:8px"><div class="field"><label for="uRepo">GitHub repository</label><div class="row"><input id="uRepo" placeholder="owner/name" style="flex:1;max-width:360px"><button id="uRepoS" class="sm">Save</button></div><p class="hint">Rover reads the public releases of this repository (no account or token needed). Every download is checked against the <code>SHA256SUMS.txt</code> file published with the release<span id="uSig"></span>; if anything doesn't match, nothing is installed.</p></div></div></details></div>
    <div class="card"><h2>Hosting engine (Coolify)</h2><p>Installed: <b id="kVer">&ndash;</b> &middot; Latest: <b id="kLat">&ndash;</b></p><div class="row"><button id="kCheck">Check</button><button id="kUp" class="primary">Update Coolify</button></div><p class="hint">Websites keep running during the update; only the dashboard is unavailable for about a minute.</p><div id="kJob"></div></div>`;
  const draw = (u) => {
    const i = u && u.info, s = (u && u.settings) || {};
    $("#uRes").innerHTML = updLine(i);
    $("#uWhen").textContent = i && i.checked ? `Last checked ${ago(i.checked)}` : "";
    $("#uShow").classList.toggle("hidden", !(i && i.newer));
    $("#uAuto").checked = !!u.auto_check;
    $$("input[name=uInst]").forEach(r => r.checked = r.value === (s.install || "ask"));
    $("#uChan").value = s.channel || "stable";
    $("#uRepo").value = s.repo || "";
    $("#uSig").textContent = u.signed ? " and its ed25519 signature" : "";
  };
  draw(await loadUpd() || { settings: {} });
  const set = (body, msg) => api("POST", "/api/update/settings", body).then(async () => { toast(msg || "Saved", "success"); draw(await loadUpd()); }).catch(err);
  $("#uCheck").onclick = (e) => busy(e.target, async () => { const i = await api("GET", "/api/update/check"); await loadUpd(); draw(upd); if (i.newer && !i.skipped) updateDialog(i); });
  $("#uShow").onclick = () => updateDialog(upd.info);
  $("#uAuto").onchange = (e) => set({ auto_check: e.target.checked }, e.target.checked ? "Rover checks for updates by itself" : "Automatic checks are off. Use “Check for updates now”.");
  $$("input[name=uInst]").forEach(r => r.onchange = () => set({ install: r.value }, r.value === "auto" ? "Updates install automatically" : "Rover asks before installing"));
  $("#uChan").onchange = (e) => set({ channel: e.target.value }, e.target.value === "beta" ? "Beta channel: pre-releases are offered too" : "Stable channel");
  $("#uRepoS").onclick = (e) => busy(e.target, () => set({ repo: $("#uRepo").value }, "Update source saved"));
  $("#kCheck").onclick = (e) => busy(e.target, async () => {
    try { $("#kVer").textContent = (await api("GET", "/api/coolify/test")).version; } catch (x) { $("#kVer").textContent = "unknown"; }
    try { $("#kLat").textContent = (await api("GET", "/api/coolify/latest")).latest; } catch (x) { $("#kLat").textContent = "unknown"; }
  });
  $("#kUp").onclick = async (e) => {
    if (!await confirmBox("Update Coolify?", "The Coolify dashboard is unavailable for about a minute; websites keep running.", "Update Coolify")) return;
    busy(e.target, async () => { const j = await api("POST", "/api/coolify/upgrade"); await jobPanel(j.job, $("#kJob"), { title: "Updating Coolify", doneTitle: "Coolify updated" }); });
  };
};
(async function updLoop() {
  await loadUpd();
  if (updWaiting(upd) && upd.settings.install !== "auto" && !updDialogShown && !$(".modal-back")) { updDialogShown = true; updateDialog(upd.info); }
  setTimeout(updLoop, 60000);
})();

// ---------- Explorer "Deploy with Rover" ----------
async function quickDeployPoll() {
  let q; try { q = await api("GET", "/api/quick"); } catch (e) { return; }
  if (!q.pending || $(".modal-back")) return;
  await api("POST", "/api/quick/clear").catch(() => {});
  await loadSites();
  const p = q.pending, base = p.split(/[\\/]/).filter(Boolean).pop() || p;
  const guess = sites.find(s => s.watch_path && s.watch_path.toLowerCase() === p.toLowerCase()) || sites.find(s => s.last_source && s.last_source.toLowerCase() === p.toLowerCase()) || sites.find(s => s.slug === store("lastProject"));
  modal(`<h2>Deploy “${esc(base)}”</h2><p class="small"><code>${esc(p)}</code></p>
    <div class="field"><label for="qdTo">Deploy to</label><select id="qdTo">${sites.filter(s => !s.archived).map(s => `<option value="${esc(s.slug)}" ${guess && guess.slug === s.slug ? "selected" : ""}>${esc(s.name)}${s.domain ? " - " + esc(s.domain) : ""}</option>`).join("")}<option value="__new">+ A new project&hellip;</option></select></div>
    <label class="check small"><input type="checkbox" id="qdDraft"> Save as a draft (don't publish yet)</label>
    <div class="row end"><button data-close>Cancel</button><button id="qdGo" class="primary">Deploy</button></div>`, (m, close) => {
    $("#qdGo", m).onclick = async () => {
      const to = $("#qdTo", m).value, draft = $("#qdDraft", m).checked;
      close();
      if (to === "__new") { go("#/new"); setTimeout(() => { const i = $("[data-i=path]"); if (i) { i.value = p; const d = i.closest("details"); if (d) d.open = true; } }, 500); return; }
      go(`#/project/${to}`);
      setTimeout(() => { const z = $("#pBody .dzwrap"); if (z) { const c = $("[data-i=draft]", z); if (c) c.checked = draft; z._run({ path: p }); } }, 700);
    };
  });
}
setInterval(quickDeployPoll, 3000);
setTimeout(quickDeployPoll, 800);

// ---------- keyboard shortcuts on project pages ----------
shortcutList.push(["1 … 8", "On a project page: switch between its tabs"], ["D", "On a project page: deploy a folder (browse)"], ["R", "On a project page: redeploy the last folder"], ["O", "On a project page: open the live site"], ["Ctrl + V", "On a project page: deploy the folder path you copied"]);
document.addEventListener("keydown", async (e) => {
  const slug = onProjectPage();
  if (!slug || e.ctrlKey || e.metaKey || e.altKey || (e.target.closest && e.target.closest("input, textarea, select, [contenteditable]")) || $(".modal-back, .modal, #palette")) return;
  const k = e.key.toLowerCase();
  if (/^[1-8]$/.test(k)) { const t = projTabs[+k - 1]; if (t) { e.preventDefault(); go(`#/project/${slug}${t[0] ? "/" + t[0] : ""}`); } return; }
  if (k === "d") { const i = $("#pBody .dzwrap [data-i=dir]"); if (i) { e.preventDefault(); i.click(); } else { go(`#/project/${slug}/deploys`); } return; }
  if (k === "o") { const s = sites.find(x => x.slug === slug), u = s && liveURL(s); if (u) { e.preventDefault(); window.open(u, "_blank", "noopener"); } return; }
  if (k === "r") { e.preventDefault(); const s = await getSite(slug); const c = await siteCfg(s).catch(() => ({})); redeployLast(s, c, $("#pBody")); }
}, true);
palCommands.push(["Updates: check for a new Rover", "#/settings/updates", "update upgrade version release github"]);
// On a project page, offer that project's own pages first (e.g. "Bakery: Password protection").
const projCmdList = [["Overview", "", "home summary checklist"], ["Deploy a new version", "/deploys", "deploy drop upload publish history rollback draft"], ["Domains", "/domain", "domain dns https www"], ["Hosting", "/hosting", "server move restart health"],
  ["Build & deploy settings", "/config/build", "build command publish folder lock watch git"], ["Redirects & headers", "/config/rules", "redirect headers rules spa 404"], ["Password protection", "/config/access", "password protect login basic auth private"],
  ["Notifications", "/config/alerts", "alerts notify down failed"], ["Backups", "/config/backups", "backup"], ["Danger zone", "/config/danger", "delete archive remove"], ["Files", "/files", "files edit"], ["Visitors & logs", "/logs", "visitors logs traffic"]];
palCommands.filter = function (fn) {
  const slug = onProjectPage(), s = slug && sites.find(x => x.slug === slug);
  const extra = s ? projCmdList.map(([l, path, kw]) => [`${s.name}: ${l}`, `#/project/${encodeURIComponent(slug)}${path}`, kw + " project this"]) : [];
  return Array.prototype.filter.call(extra.concat(Array.from(this)), fn);
};

// ---------- Jiv knows where you are ----------
curScreen = function () {
  const p = (location.hash || "#/dashboard").slice(2).split(/[/?#]/);
  const a = p[0] || "dashboard";
  if (a === "project") return ({ files: "files", deploys: "deploys", settings: "project-settings", config: "project-settings", hosting: "project-hosting", testing: "testing", logs: "logs", domain: "domain" })[p[2]] || "project";
  if (a === "settings") return ({ server: "server", storage: "storage", ai: "ai", git: "git", updates: "updates", organise: "organise", backups: "backups", jiv: "jiv-settings" })[p[1]] || "settings";
  if (a === "new") return ({ git: "git", netlify: "netlify", template: "new" })[p[1]] || "new";
  return a;
};
