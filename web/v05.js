"use strict";
// Rover 0.5: Jiv (offline helper with actions), command palette (Ctrl+K),
// Hosting accounts portal, several servers, undo, proactive help.
// Same security model: same-origin API only; every server/DNS/money
// change goes through a confirm click and a server-issued action id.

// ---------- small helpers ----------
function curScreen() {
  const p = (location.hash || "#/dashboard").slice(2).split(/[/?#]/);
  const a = p[0] || "dashboard";
  if (a === "project") return ({ files: "files", deploys: "deploys", settings: "project-settings", logs: "logs", domain: "domain" })[p[2]] || "project";
  if (a === "settings") return ({ server: "server", storage: "storage", ai: "ai", git: "git", updates: "updates", organise: "organise", backups: "backups", jiv: "jiv-settings" })[p[1]] || "settings";
  if (a === "new") return ({ git: "git", netlify: "netlify", template: "new" })[p[1]] || "new";
  return a;
}
function curSite() { const m = /^#\/project\/([^/]+)/.exec(location.hash); return m ? decodeURIComponent(m[1]) : ""; }
function jivPrefs() { return store("jivPrefs") || {}; }
let lastUndoAt = 0, lastUndo = null;
function toastUndo(msg, undo, kind) {
  lastUndoAt = Date.now();
  const t = h(`<div class="toast ${esc(kind || "success")}" role="status"><div>${esc(msg)}</div><button class="sm undo">Undo</button><button class="x" aria-label="Dismiss">&times;</button></div>`);
  $(".x", t).onclick = () => t.remove();
  const run = async () => { t.remove(); lastUndo = null; try { await undo(); toast("Undone", "success"); await loadSites(); rerender(); } catch (e) { err(e); } };
  $(".undo", t).onclick = run;
  lastUndo = run;
  $("#toasts").appendChild(t);
  setTimeout(() => { t.remove(); if (lastUndo === run) lastUndo = null; }, 9000);
}
// plain toasts that duplicate an undo toast are skipped
const toastBase = toast;
toast = function (msg, kind, opts) {
  if (Date.now() - lastUndoAt < 1500 && /^(Archived|Restored|Pinned|Unpinned|Updated \d+ project)/.test(String(msg))) return;
  return toastBase(msg, kind, opts);
};

// ---------- api: projects on another server, undo for small changes ----------
const apiBase = api;
api = async function (method, path, body, opts) {
  try {
    const r = await apiBase(method, path, body, opts);
    undoHook(method, path, body, r);
    return r;
  } catch (e) {
    if (e.status === 409 && e.data && e.data.switch_server && !(opts && opts.noSwitch)) {
      const ok = await confirmBox("Switch server?", `<p>${esc(e.data.error)}</p><p class="small muted">Rover connects to <b>${esc(e.data.server_label)}</b> with its own key. Nothing is changed or deleted on either server.</p>`, "Switch and continue");
      if (!ok) throw e;
      await apiBase("POST", `/api/servers/${encodeURIComponent(e.data.switch_server)}/activate`);
      await refreshOverview(); drawServerChip();
      return api(method, path, body, opts);
    }
    throw e;
  }
};
function undoHook(method, path, body) {
  if (method !== "POST" || !body) return;
  const m = /^\/api\/sites\/([^/]+)\/meta$/.exec(path);
  if (m && ("archived" in body || "pinned" in body)) {
    const k = "archived" in body ? "archived" : "pinned", v = body[k];
    const what = k === "archived" ? (v ? "Archived. The site keeps running." : "Restored from the archive") : (v ? "Pinned" : "Unpinned");
    setTimeout(() => toastUndo(what, () => apiBase("POST", path, { [k]: !v })), 0);
    return;
  }
  if (path === "/api/bulk" && ["archive", "unarchive", "pin", "unpin"].includes(body.action)) {
    const inv = { archive: "unarchive", unarchive: "archive", pin: "unpin", unpin: "pin" }[body.action];
    setTimeout(() => toastUndo(`${body.slugs.length} project(s): ${body.action}${body.action.endsWith("e") ? "d" : "ned"}`, () => apiBase("POST", "/api/bulk", { action: inv, slugs: body.slugs, value: "" })), 0);
  }
}

// ---------- provider logos ----------
function provLogo(p, size) {
  return `<span class="plogo${size ? " " + size : ""}" style="background:${esc(p.color || "#64748b")}" aria-hidden="true">${esc(p.short || "?")}</span>`;
}

// ---------- sidebar: Hosting entry and the active-server chip ----------
(function addNav() {
  const dom = $('#nav a[data-route="domains"]');
  const a = h(`<a href="#/hosting" data-route="hosting"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="3" y="4" width="18" height="7" rx="1.5"/><rect x="3" y="13" width="18" height="7" rx="1.5"/><path d="M7 7.5h.01M7 16.5h.01"/></svg><span>Hosting accounts</span></a>`);
  dom.parentNode.insertBefore(a, dom.nextSibling);
  const chip = h(`<button id="srvChip" class="srvchip hidden" title="Active server - click to switch" aria-haspopup="true"><span class="lbl"></span><span aria-hidden="true">&#9662;</span></button>`);
  $(".side-bottom").insertBefore(chip, $("#connState"));
  chip.onclick = serverMenu;
})();
let srvCache = null;
async function drawServerChip() {
  try { srvCache = await apiBase("GET", "/api/servers"); } catch (e) { return; }
  const list = srvCache.servers || [], act = list.find(s => s.active);
  const chip = $("#srvChip");
  chip.classList.toggle("hidden", list.length < 2);
  if (act) $(".lbl", chip).textContent = act.label;
}
async function serverMenu(e) {
  e.stopPropagation();
  const old = $("#srvMenu"); if (old) { old.remove(); return; }
  const list = (srvCache && srvCache.servers) || [];
  const m = h(`<div id="srvMenu" class="srvmenu" role="menu">${list.map(s => `<button role="menuitem" data-id="${esc(s.id)}" ${s.active ? 'aria-current="true"' : ""}><b>${esc(s.label)}</b><span class="small muted">${esc(s.ip)} &middot; ${s.projects} project(s)${s.active ? " &middot; active" : ""}</span></button>`).join("")}<a href="#/hosting" role="menuitem">Manage servers &amp; accounts&hellip;</a></div>`);
  document.body.appendChild(m);
  const r = $("#srvChip").getBoundingClientRect();
  m.style.left = r.left + "px"; m.style.bottom = (innerHeight - r.top + 6) + "px";
  const close = () => { m.remove(); document.removeEventListener("click", close); };
  setTimeout(() => document.addEventListener("click", close), 0);
  m.onclick = async (ev) => {
    const b = ev.target.closest("button[data-id]"); if (!b) return;
    const s = list.find(x => x.id === b.dataset.id); if (!s || s.active) return;
    if (!await confirmBox(`Switch to ${s.label}?`, `<p>New projects, deploys and checks will use <b>${esc(s.label)}</b> (${esc(s.ip)}). Projects on other servers keep running. Nothing is deleted.</p>`, "Switch")) return;
    try { await apiBase("POST", `/api/servers/${encodeURIComponent(s.id)}/activate`); toast("Now using " + s.label, "success"); await refreshOverview(); drawServerChip(); rerender(); } catch (x) { err(x); }
  };
}

// ---------- Hosting accounts portal ----------
let hostProv = null;
async function loadProviders() { if (!hostProv) hostProv = await api("GET", "/api/hosting/providers"); return hostProv; }
const fmtMoney = (n, cur) => (cur === "EUR" ? "€" : "$") + (Math.round(n * 100) / 100).toFixed(2);
function srvCacheActive(id) { return srvCache && (srvCache.servers || []).some(s => s.id === id && s.active); }

route(/^\/hosting(?:\?add=([a-z]*))?$/, async (add) => {
  const v = view();
  v.className = "page wide";
  const [pv, r] = await Promise.all([loadProviders(), api("GET", "/api/hosting/accounts")]);
  await drawServerChip();
  const accts = r.accounts || [], servers = r.servers || [];
  v.innerHTML = `<div class="head"><h1>Hosting accounts</h1><span class="grow"></span><button id="hAdd" class="primary">+ Connect an account</button></div>
    <p class="muted">Connect the companies where you rent servers. Rover lists your servers, sets one up with one click, and can create new ones (it shows the price first). Add as many accounts as you like, even several for the same provider.</p>
    <div class="alert info small">&#128274; Rover never asks for your website password. It uses each provider's official sign-in or API token, saved encrypted on this PC and never sent to any AI.</div>
    <h2 class="sect">Your servers in Rover</h2><div id="hSrv"></div>
    <h2 class="sect">Connected accounts</h2><div id="hAcc"></div>`;
  $("#hAdd").onclick = () => addAccountWizard();
  $("#hSrv").innerHTML = servers.length ? `<div class="srvgrid">${servers.map(s => {
    const a = accts.find(x => x.id === s.account_id), p = pv.providers.find(x => x.id === (s.provider || "other")) || {};
    return `<div class="card flat srvcard ${s.active ? "active" : ""}" data-id="${esc(s.id)}"><div class="row nowrap">${provLogo(p)}<div class="grow"><b>${esc(s.label)}</b><div class="small muted">${esc(s.ip)} &middot; ${esc(s.user || "root")}${a ? " &middot; " + esc(a.nickname) : ""}</div></div>${s.active ? `<span class="badge ok">Active</span>` : ""}</div>
      <div class="small" style="margin:8px 0">${s.projects} project(s) &middot; ${s.engine ? "hosting engine installed" : `<span class="warnc">not set up yet</span>`}${s.active ? (s.connected ? " &middot; connected" : " &middot; not connected") : ""}</div>
      <div class="row">${s.active ? (s.engine ? "" : `<a class="btn sm primary" href="#/setup">Set up now</a>`) : `<button class="sm primary" data-a="activate">Make active</button>`}<button class="sm ghost" data-a="rename">Rename</button>${s.active || s.projects ? "" : `<button class="sm ghost danger" data-a="remove">Remove</button>`}</div></div>`;
  }).join("")}</div>` : `<div class="empty">No server yet. Connect an account below and click <b>Use this server for Rover</b>, or add <b>Any other VPS</b>.</div>`;
  $("#hSrv").onclick = async (e) => {
    const b = e.target.closest("button[data-a]"); if (!b) return;
    const id = b.closest("[data-id]").dataset.id, s = servers.find(x => x.id === id);
    if (b.dataset.a === "activate") {
      if (!await confirmBox(`Make ${s.label} active?`, `<p>New projects and deploys go to this server. Nothing is deleted anywhere.</p>`, "Make active")) return;
      busy(b, async () => { await api("POST", `/api/servers/${id}/activate`); toast("Now using " + s.label, "success"); await refreshOverview(); drawServerChip(); rerender(); });
    } else if (b.dataset.a === "rename") {
      const n = await askText("Rename server", "A name you recognise, e.g. “Shop server”.", s.label); if (!n) return;
      busy(b, async () => { await api("POST", `/api/servers/${id}/rename`, { label: n }); drawServerChip(); rerender(); });
    } else if (b.dataset.a === "remove") {
      if (!await confirmBox(`Remove ${s.label} from Rover?`, `<p>Rover forgets this server. The server itself keeps running at your provider and is not deleted.</p>`, "Remove", true)) return;
      busy(b, async () => { await api("DELETE", `/api/servers/${id}`); drawServerChip(); rerender(); });
    }
  };
  const acc = $("#hAcc");
  if (!accts.length) acc.innerHTML = `<div class="empty"><b>No accounts connected yet.</b><br>Pick your provider to see your servers here.<div class="provgrid" style="margin-top:14px">${pv.providers.map(p => `<button class="provbtn" data-p="${esc(p.id)}">${provLogo(p)}<span>${esc(p.name)}</span></button>`).join("")}</div></div>`;
  else acc.innerHTML = accts.map(a => {
    const p = pv.providers.find(x => x.id === a.provider) || {};
    const used = a.used || {};
    return `<div class="card acct" data-id="${esc(a.id)}"><div class="row nowrap">${provLogo(p, "lg")}<div class="grow"><h3 style="margin:0">${esc(a.nickname)}</h3><div class="small muted">${esc(p.name)}${a.who ? " &middot; " + esc(a.who) : ""} &middot; ${a.auth === "oauth" ? "browser sign-in" : a.auth === "keys" ? "API keys" : "API token"}${a.hint ? " <code>" + esc(a.hint) + "</code>" : ""}</div></div>
        ${a.status === "error" ? `<span class="badge bad" title="${esc(a.error)}">Needs attention</span>` : `<span class="badge ok">Connected</span>`}
        <button class="sm" data-a="refresh" title="Refresh the server list">&#10227; Refresh</button>${p.can_create ? `<button class="sm primary" data-a="create">+ Create server</button>` : p.order_url ? `<a class="btn sm" href="${esc(p.order_url)}" target="_blank" rel="noopener">Order a server &#8599;</a>` : ""}
        <button class="sm ghost" data-a="menu" aria-label="More">&#8943;</button></div>
      ${a.status === "error" ? `<div class="alert bad small" style="margin-top:10px">${esc(a.error)} <button class="sm ask" data-a="why">Ask Jiv</button></div>` : ""}
      <ul class="deploys hsrv">${(a.servers || []).map(s => `<li data-sid="${esc(s.id)}"><span class="sdot ${/run|active|^on|ok/i.test(s.status) ? "ok" : "off"}" title="${esc(s.status)}"></span><span class="grow"><b>${esc(s.name || s.id)}</b> <span class="small muted">${esc(s.ip || "no IP yet")}</span><div class="small muted">${[s.plan, s.region, s.cpus ? s.cpus + " CPU" : "", s.memory_gb ? s.memory_gb + " GB RAM" : "", s.status].filter(Boolean).map(esc).join(" &middot; ")}</div></span>
        ${used[s.id] ? `<span class="badge ok">${srvCacheActive(used[s.id]) ? "Active in Rover" : "In Rover"}</span>${srvCacheActive(used[s.id]) ? "" : `<button class="sm" data-a="use">Switch to it</button>`}` : `<button class="sm primary" data-a="use" ${s.ip ? "" : "disabled"}>Use this server for Rover</button>`}</li>`).join("") || `<li class="muted small"><span>No servers in this account yet.${p.can_create ? " Click <b>Create server</b>." : p.order_url ? ` <a href="${esc(p.order_url)}" target="_blank" rel="noopener">Order one</a>, then click Refresh.` : ""}</span></li>`}</ul>
      ${a.checked ? `<div class="small muted">Checked ${esc(ago(a.checked))}</div>` : ""}</div>`;
  }).join("");
  acc.onclick = async (e) => {
    const pb = e.target.closest(".provbtn"); if (pb) return addAccountWizard(pb.dataset.p);
    const b = e.target.closest("[data-a]"); if (!b) return;
    const card = b.closest(".acct"), id = card.dataset.id, a = accts.find(x => x.id === id);
    const act = b.dataset.a;
    if (act === "refresh") busy(b, async () => { try { await api("POST", `/api/hosting/accounts/${id}/refresh`); toast("Server list updated", "success"); } catch (x) { err(x); } rerender(); });
    else if (act === "why") jivAsk(`Why does my ${a.provider} hosting account show an error?`, a.error);
    else if (act === "create") createServerWizard(a);
    else if (act === "use") { const sid = b.closest("li").dataset.sid; useServer(a, a.servers.find(s => s.id === sid), b); }
    else if (act === "menu") {
      const choice = await modal(`<h2>${esc(a.nickname)}</h2><div class="col"><button data-c="rename">Rename</button><button data-c="remove" class="danger">Remove account from Rover</button><button data-close>Cancel</button></div>`, (m, close) => $$("[data-c]", m).forEach(x => x.onclick = () => close(x.dataset.c)));
      if (choice === "rename") { const n = await askText("Nickname", "For example “Client A - Hetzner”.", a.nickname); if (n) { await api("POST", `/api/hosting/accounts/${id}/rename`, { nickname: n }); rerender(); } }
      if (choice === "remove") {
        if (!await confirmBox(`Remove ${a.nickname}?`, `<p>Rover deletes the saved token for this account from this PC. Your servers keep running and your projects keep working. You can also revoke the token on the provider's website.</p>`, "Remove account", true)) return;
        await api("DELETE", `/api/hosting/accounts/${id}`); toast("Account removed", "success"); rerender();
      }
    }
  };
  if (add !== undefined && add !== null) addAccountWizard(add);
}, "hosting");

async function addAccountWizard(pre) {
  const pv = await loadProviders();
  let p = pv.providers.find(x => x.id === pre);
  const pickHtml = () => `<h2>Connect a hosting account</h2><p class="small muted">Pick where you rent (or want to rent) servers.</p><div class="provgrid">${pv.providers.map(x => `<button class="provbtn" data-p="${esc(x.id)}">${provLogo(x)}<span><b>${esc(x.name)}</b><span class="small muted">${x.auth === "oauth" ? "Sign in with browser" : x.auth === "ssh" ? "IP address + SSH" : x.auth === "keys" ? "API keys" : "API token"}${x.can_create ? " &middot; can create servers" : ""}</span></span></button>`).join("")}</div><div class="row end"><button data-close>Cancel</button></div>`;
  await modal(`<div id="wz" class="wizard"></div>`, (m, close) => {
    m.classList.add("wide");
    const wz = $("#wz", m);
    const pick = () => { wz.innerHTML = pickHtml(); $$(".provbtn", wz).forEach(b => b.onclick = () => { p = pv.providers.find(x => x.id === b.dataset.p); show(); }); $("[data-close]", wz).onclick = () => close(null); };
    const field = (f) => `<div class="field"><label for="f_${esc(f.key)}">${esc(f.label)}${f.optional ? ' <span class="muted">(optional)</span>' : ""}</label>${f.options ? `<select id="f_${esc(f.key)}">${f.options.map(o => `<option>${esc(o)}</option>`).join("")}</select>` : f.multiline ? `<textarea id="f_${esc(f.key)}" rows="4" spellcheck="false" placeholder="${esc(f.placeholder || "")}"></textarea>` : `<input id="f_${esc(f.key)}" ${f.secret ? 'type="password"' : ""} autocomplete="off" spellcheck="false" placeholder="${esc(f.placeholder || "")}">`}${f.hint ? `<div class="hint">${esc(f.hint)}</div>` : ""}</div>`;
    const show = () => {
      if (!p) return pick();
      if (p.auth === "ssh") return sshForm();
      const tokenPart = `${p.token_url ? `<a class="btn sm" href="${esc(p.token_url)}" target="_blank" rel="noopener">Open the ${esc(p.token_name || "token")} page &#8599;</a>` : ""}
        <div class="grid2 fields">${(p.fields || []).map(field).join("")}</div>
        <p class="hint">&#128274; Saved encrypted on this PC (Windows DPAPI). Never shown again, never sent to any AI.</p>
        <div id="wErr"></div><div class="row end"><button id="wGo" class="primary">Check and connect</button></div>`;
      wz.innerHTML = `<div class="row nowrap">${provLogo(p, "lg")}<div><h2 style="margin:0">Connect ${esc(p.name)}</h2><div class="small muted">${p.can_create ? "Lists your servers and can create new ones." : "Lists your servers; order new ones on their website."}</div></div></div>
        <div class="field" style="margin-top:12px"><label for="wNick">Nickname <span class="muted">(optional, e.g. “Client A”)</span></label><input id="wNick" placeholder="${esc(p.name)}"></div>
        ${p.oauth ? `<div class="card flat"><b>Easiest: sign in with your browser</b><ol class="steps-plain">${p.steps.map(s => `<li>${esc(s)}</li>`).join("")}</ol><button id="wOauth" class="primary">Sign in with ${esc(p.name)}</button><div id="wOst"></div></div>
          <details class="card flat"><summary><b>Or use a token</b></summary><p class="small">${esc(p.note || "")}</p>${tokenPart}</details>`
          : `<ol class="steps-plain">${p.steps.map(s => `<li>${esc(s)}</li>`).join("")}</ol>${p.note ? `<p class="hint">${esc(p.note)}</p>` : ""}${tokenPart}`}
        <div class="row"><button id="wBack" class="ghost">&larr; All providers</button><span class="grow"></span><button data-close>Cancel</button></div>`;
      $("#wBack", wz).onclick = () => { p = null; pick(); };
      $("[data-close]", wz).onclick = () => close(null);
      if (p.oauth) $("#wOauth", wz).onclick = (e) => busy(e.target, async () => {
        const st = $("#wOst", wz);
        try {
          const r = await api("POST", "/api/hosting/oauth/digitalocean", { nickname: $("#wNick", wz).value.trim() });
          st.innerHTML = `<p class="small"><span class="spin"></span> Finish signing in in your browser&hellip; <a href="${esc(r.url)}" target="_blank" rel="noopener">Open it again</a></p>`;
          for (let i = 0; i < 300; i++) {
            await new Promise(res => setTimeout(res, 2000));
            if (!document.body.contains(st)) return;
            const s = await api("GET", "/api/hosting/oauth/" + r.flow);
            if (s.done) { if (s.error) { st.innerHTML = `<div class="alert bad small">${esc(s.error)}</div>`; return; } toast("DigitalOcean connected", "success"); close(true); go("#/hosting"); return; }
          }
        } catch (x) { st.innerHTML = `<div class="alert bad small">${esc(x.message)}</div>`; }
      });
      $("#wGo", wz).onclick = (e) => busy(e.target, async () => {
        const vals = {}; (p.fields || []).forEach(f => vals[f.key] = $("#f_" + f.key, wz).value);
        $("#wErr", wz).innerHTML = "";
        try {
          const r = await api("POST", "/api/hosting/accounts", { provider: p.id, nickname: $("#wNick", wz).value.trim(), fields: vals });
          (p.fields || []).forEach(f => { const el = $("#f_" + f.key, wz); if (f.secret || f.multiline) el.value = ""; });
          toast(`${p.name} connected: ${r.servers} server(s) found`, "success"); close(true); go("#/hosting"); rerender();
        } catch (x) { $("#wErr", wz).innerHTML = `<div class="alert bad small">${esc(x.message)} <button class="sm ask">Ask Jiv</button></div>`; $(".ask", wz).onclick = () => { close(null); jivAsk(`Connecting ${p.name} failed. What should I check?`, x.message); }; }
      });
    };
    const sshForm = () => {
      wz.innerHTML = `<div class="row nowrap">${provLogo(p, "lg")}<div><h2 style="margin:0">Any other VPS</h2><div class="small muted">Any Ubuntu 22.04/24.04 server from any company.</div></div></div>
        <ol class="steps-plain"><li>Type the server's IP address (from your provider's panel or welcome email).</li><li>Type the root password <b>once</b>. Rover installs its own key and forgets the password.</li><li>Rover sets up the hosting engine automatically.</li></ol>
        <div class="grid2"><div class="field"><label for="oIp">IP address</label><input id="oIp" placeholder="203.0.113.10" autocomplete="off"></div><div class="field"><label for="oPw">Password <span class="muted">(used once, never saved)</span></label><input id="oPw" type="password" autocomplete="off"></div>
        <div class="field"><label for="oUser">User</label><input id="oUser" value="root"></div><div class="field"><label for="oPort">SSH port</label><input id="oPort" type="number" value="22"></div></div>
        <div id="wErr"></div><div class="row"><button id="wBack" class="ghost">&larr; All providers</button><span class="grow"></span><button data-close>Cancel</button><button id="wGo" class="primary">Connect</button></div>`;
      $("#wBack", wz).onclick = () => { p = null; pick(); };
      $("[data-close]", wz).onclick = () => close(null);
      $("#wGo", wz).onclick = (e) => busy(e.target, async () => {
        $("#wErr", wz).innerHTML = "";
        try {
          await api("POST", "/api/server", { ip: $("#oIp", wz).value.trim(), port: +$("#oPort", wz).value || 22, user: $("#oUser", wz).value.trim() || "root", key_path: "" });
          await api("POST", "/api/ssh/connect", { password: $("#oPw", wz).value, passphrase: "" });
          $("#oPw", wz).value = "";
          close(true); await refreshOverview(); drawServerChip(); toast("Connected", "success"); go(ov.stage === "ready" ? "#/hosting" : "#/setup");
        } catch (x) { $("#wErr", wz).innerHTML = `<div class="alert bad small">${esc(x.message)}</div>`; }
      });
    };
    show();
  });
}

async function useServer(a, s, btn) {
  if (!s) return;
  if (!await confirmBox(`Use ${s.name || s.ip} for Rover?`, `<p>Rover connects to <b>${esc(s.ip)}</b> with its own SSH key and makes it the active server. If the hosting engine isn't installed yet, Rover installs it (about 5-10 minutes). Use a fresh Ubuntu server with nothing else on it.</p>`, "Use this server")) return;
  busy(btn, async () => {
    const r = await api("POST", `/api/hosting/accounts/${a.id}/servers/${encodeURIComponent(s.id)}/use`);
    await refreshOverview(); drawServerChip();
    if (!r.connected) {
      const ok = await modal(`<h2>Sign in to ${esc(s.ip)} once</h2><p class="small muted">Rover's key isn't on this server yet. Type the <b>${esc(r.user)}</b> password from your provider (welcome email or panel) <b>once</b>; Rover installs its own key and forgets the password.</p><div class="field"><label for="uPw">Password</label><input id="uPw" type="password" autocomplete="off" autofocus></div><div id="uErr"></div><div class="row end"><button data-close>Later</button><button id="uGo" class="primary">Connect</button></div>`, (m, close) => {
        $("#uGo", m).onclick = (e) => busy(e.target, async () => { try { await api("POST", "/api/ssh/connect", { password: $("#uPw", m).value, passphrase: "" }); $("#uPw", m).value = ""; close(true); } catch (x) { $("#uErr", m).innerHTML = `<div class="alert bad small">${esc(x.message)}</div>`; } });
      });
      if (!ok) { rerender(); return; }
      await refreshOverview();
    }
    toast(`Now using ${s.name || s.ip}`, "success");
    go(r.engine ? "#/hosting" : "#/setup");
  });
}

async function createServerWizard(a) {
  await modal(`<h2>Create a server at ${esc(a.provider_name)}</h2><div id="cw"><p><span class="spin"></span> Loading sizes and prices&hellip;</p></div>`, async (m, close) => {
    m.classList.add("wide");
    const cw = $("#cw", m);
    const fail = (html) => { cw.innerHTML = html + `<div class="row end"><button data-close>Close</button></div>`; $("[data-close]", cw).onclick = () => close(null); };
    let plans;
    try { plans = (await api("GET", `/api/hosting/accounts/${a.id}/plans`)).plans || []; } catch (x) { return fail(`<div class="alert bad">${esc(x.message)}</div>`); }
    if (!plans.length) return fail(`<div class="alert warn">No suitable sizes found (Rover needs at least 2 CPU / 2 GB RAM).</div>`);
    const rec = plans.find(p => p.memory_gb >= 4) || plans[0];
    cw.innerHTML = `<p class="small muted">Sizes that fit the hosting engine, cheapest first.</p>
      <div class="planlist">${plans.slice(0, 8).map(p => `<label class="plan"><input type="radio" name="pl" value="${esc(p.id)}" ${p === rec ? "checked" : ""}><span class="grow"><b>${esc(p.label)}</b>${p === rec ? ' <span class="badge info">Recommended</span>' : ""}<div class="small muted">${p.cores} CPU &middot; ${p.memory_gb} GB RAM &middot; ${p.disk_gb} GB disk</div></span><b class="price">${fmtMoney(p.monthly, p.currency)}<span class="small muted">/month</span></b></label>`).join("")}</div>
      <div class="field"><label for="cReg">Location</label><select id="cReg"></select></div>
      <p class="hint">The price is what ${esc(a.provider_name)} charges your account (billed by them, usually per hour up to this monthly amount). Delete the server at the provider to stop paying.</p>
      <div class="row end"><button data-close>Cancel</button><button id="cGo" class="primary">Create and set up&hellip;</button></div><div id="cJob"></div>`;
    const regs = () => { const p = plans.find(x => x.id === $("input[name=pl]:checked", cw).value); $("#cReg", cw).innerHTML = (p.regions || []).map(r => `<option>${esc(r)}</option>`).join(""); };
    $$("input[name=pl]", cw).forEach(r => r.onchange = regs); regs();
    $("[data-close]", cw).onclick = () => close(null);
    $("#cGo", cw).onclick = async (e) => {
      const p = plans.find(x => x.id === $("input[name=pl]:checked", cw).value), reg = $("#cReg", cw).value;
      if (!await confirmBox("Create this server? It costs money.", `<p><b>${esc(p.label)}</b> in <b>${esc(reg)}</b> at ${esc(a.provider_name)} (${esc(a.nickname)}).</p><p style="font-size:1.25em"><b>${fmtMoney(p.monthly, p.currency)} per month</b></p><p class="small muted">Billed by ${esc(a.provider_name)} to your account until you delete the server. Rover then installs the hosting engine automatically (5-10 minutes).</p>`, `Yes, create for ${fmtMoney(p.monthly, p.currency)}/month`)) return;
      busy(e.target, async () => {
        const r = await api("POST", `/api/hosting/accounts/${a.id}/create`, { plan: p.id, region: reg, confirmed: true, monthly: p.monthly, auto_updates: true });
        $$("button", cw).forEach(b => b.disabled = true);
        const j = await jobPanel(r.job, $("#cJob", cw), { title: "Creating your server", doneTitle: "Server ready", failTitle: "Server creation failed" });
        await refreshOverview(); drawServerChip();
        if (!j.error) { toast("Your new server is ready", "success"); close(true); go("#/hosting"); rerender(); }
        else $$("button", cw).forEach(b => b.disabled = false);
      });
    };
  });
}

// ---------- project settings: which server ----------
const projectSettingsV04 = projectSettings;
projectSettings = function (s, b) {
  projectSettingsV04(s, b);
  const card = h(`<div class="card"><h2>Server</h2><div id="psSrv" class="small muted">Loading&hellip;</div></div>`);
  b.insertBefore(card, b.querySelector(".danger-zone"));
  api("GET", "/api/servers").then(r => {
    const list = r.servers || [];
    const cur = list.find(x => x.id === s.server) || list.find(x => x.active);
    const others = list.filter(x => x !== cur);
    $("#psSrv").innerHTML = `<p>This project is hosted on <b>${esc(cur ? cur.label : "your server")}</b>${cur ? ` (${esc(cur.ip)})` : ""}.</p>` + (others.length ? `<div class="row"><select id="psTo">${others.map(x => `<option value="${esc(x.id)}">${esc(x.label)} (${esc(x.ip)})</option>`).join("")}</select><button id="psMove">Move to this server&hellip;</button></div><p class="hint">Rover sets the project up on the other server and publishes its last deploy there. Then change the domain's A record to the new IP (Rover shows it). The old copy keeps running until you delete it.</p><div id="psJob"></div>` : `<p class="hint">Connect more servers in <a href="#/hosting">Hosting accounts</a> to move projects between them.</p>`);
    if ($("#psMove")) $("#psMove").onclick = async (e) => {
      const to = others.find(x => x.id === $("#psTo").value);
      if (!await confirmBox(`Move ${s.name} to ${to.label}?`, `<p>Rover switches to <b>${esc(to.label)}</b>, creates the project there and publishes the last deploy from this PC.${s.domain ? ` Afterwards point <b>${esc(s.domain)}</b> to <code>${esc(to.ip)}</code>.` : ""}</p>`, "Move project")) return;
      busy(e.target, async () => { const r = await api("POST", `/api/sites/${s.slug}/server`, { server: to.id }, { noSwitch: true }); const j = await jobPanel(r.job, $("#psJob"), { title: "Moving", doneTitle: "Moved" }); await refreshOverview(); drawServerChip(); if (!j.error) later(rerender, 800); });
    };
  }).catch(e => $("#psSrv").textContent = e.message);
};

// ---------- Jiv 0.5 ----------
let jivSuggestFor = "";
async function jivSuggest() {
  const box = $("#suggest"); if (!box) return;
  const key = curScreen() + "|" + curSite();
  if (chat.length && jivSuggestFor === key) return;
  jivSuggestFor = key;
  try {
    const r = await apiBase("GET", `/api/jiv/suggest?screen=${encodeURIComponent(curScreen())}&site=${encodeURIComponent(curSite())}`);
    $$(".pill", box).forEach(x => x.remove());
    box.insertAdjacentHTML("beforeend", (r.questions || []).map(x => `<button class="pill sugg" type="button">${esc(x)}</button>`).join(""));
    box.onclick = (e) => { if (e.target.matches("button.pill")) sendChat(e.target.textContent); };
  } catch (e) {}
}
window.addEventListener("hashchange", () => { if (!chatAbort) jivSuggest(); });

targets = function () {
  const out = [{ v: "auto|", label: "Jiv · automatic (offline first)", cloud: false }, { v: "builtin|", label: "Jiv · built-in answers only", cloud: false }];
  if (aiInfo && aiInfo.brain) out.splice(1, 0, { v: "brain|", label: "Jiv · offline brain (" + aiInfo.brain + ")", cloud: false });
  if (!aiInfo) return out;
  (aiInfo.local || []).forEach(l => (l.models && l.models.length ? l.models : [""]).forEach(m => out.push({ v: `local:${l.id}|${m}`, label: `${l.name}${m ? " · " + m : ""}`, cloud: false })));
  (aiInfo.providers || []).filter(p => p.connected).forEach(p => out.push({ v: `${p.id}|${p.model || ""}`, label: `${p.custom ? (p.conn_name || p.name) : p.name}${p.model ? " · " + p.model : ""} (cloud)`, cloud: true, company: p.company }));
  return out;
};
let aiLoad05 = null;
ensureAI = function () {
  if (aiInfo) return Promise.resolve(aiInfo);
  if (!aiLoad05) aiLoad05 = Promise.all([apiBase("GET", "/api/ai/providers").catch(() => ({ providers: [], local: [] })), apiBase("GET", "/api/jiv/brain").catch(() => null)]).then(([r, b]) => {
    if (b && b.installed && !b.settings.brain_off) r.brain = b.model.short;
    aiInfo = r; aiLoad05 = null; fillModels(); return r;
  });
  return aiLoad05;
};
fillModels = function () {
  const sel = $("#cModel"); if (!sel) return;
  const ts = targets();
  if (aiInfo) $("#cCtx").checked = !aiInfo.no_context;
  sel.innerHTML = ts.map(t => `<option value="${esc(t.v)}">${esc(t.label)}</option>`).join("");
  const saved = jivPrefs().target;
  sel.value = saved && ts.find(t => t.v === saved) ? saved : "auto|";
  const where = () => { const t = curTarget(); $("#cWhere").innerHTML = t && t.cloud ? `<span class="badge warn">sent to ${esc(t.company)}</span>` : `<span class="badge ok">offline · on this PC</span>`; };
  sel.onchange = () => { where(); store("jivPrefs", Object.assign(jivPrefs(), { target: sel.value })); };
  where();
  if (!chat.length) jivSuggest();
};
suggestions = jivSuggest;

drawChat = function () {
  const box = $("#msgs"); if (!box) return;
  box.innerHTML = chat.length ? "" : `<div class="empty"><b>Hi, I'm Jiv!</b><br>I work offline. Ask how to do anything in Rover, ask <i>“why is my site down?”</i>, or tell me what to do (<i>“roll back bakery”</i>). Changes always wait for your click. <span class="small muted">Hindi/Hinglish works too.</span></div>`;
  chat.forEach((m, i) => box.appendChild(msgEl(m, i)));
  box.scrollTop = box.scrollHeight;
};
function artChips(list, cls) { return `<div class="chips ${cls || ""}">${list.map(a => `<button class="chip" data-art="${esc(a.id)}">${esc(a.title)}</button>`).join("")}</div>`; }
function goButtons(list) { return (list || []).length ? `<div class="row gobtns">${list.map(g => `<a class="btn sm" href="${esc(g.href)}">${esc(g.label)} &rarr;</a>`).join("")}</div>` : ""; }
msgEl = function (m, i) {
  const busyNow = !m.content && !m.error && !m.diag && !(m.actions || []).length && !m.choices && !m.topics;
  const el = h(`<div class="msg ${m.role}"><div class="bubble">${m.role === "user" ? esc(m.content).replace(/\n/g, "<br>") : busyNow ? `<span class="spin"></span> ${esc(m.status || "")}` : md(m.content)}</div></div>`);
  const bub = el.firstElementChild;
  (m.notes || []).forEach(n => bub.insertBefore(h(`<div class="small muted note">${esc(n)}</div>`), bub.firstChild));
  if (m.diag) bub.appendChild(diagEl(m.diag, i));
  if (m.sources) { const seen = new Set(); bub.insertAdjacentHTML("beforeend", goButtons(m.sources.flatMap(a => a.go || []).filter(g => !seen.has(g.href) && seen.add(g.href)).slice(0, 4))); }
  if (m.topics) bub.insertAdjacentHTML("beforeend", artChips(m.topics));
  if (m.related) bub.insertAdjacentHTML("beforeend", `<div class="small muted" style="margin-top:8px">Related:</div>` + artChips(m.related, "rel"));
  if (m.choices) bub.insertAdjacentHTML("beforeend", `<div class="chips">${m.choices.map((c, k) => `<button class="chip pick" data-k="${k}">${esc(c.label)}</button>`).join("")}</div>`);
  if (m.error) bub.appendChild(h(`<div class="alert bad small">${esc(m.error)}</div>`));
  (m.actions || []).forEach((a, k) => el.appendChild(actionCard(a, i, k)));
  if (m.role === "assistant" && m.meta) el.appendChild(h(`<div class="small muted meta">${esc(m.meta)}</div>`));
  el.onclick = (e) => {
    const c = e.target.closest("[data-art]"); if (c) { showArticle(c.dataset.art); return; }
    const p = e.target.closest(".pick");
    if (p) { const card = m.choices[+p.dataset.k]; m.choices = null; (m.actions = m.actions || []).push(card); store("chat", chat); drawChat(); if (card.safe) runCard(card, chat.indexOf(m), m.actions.length - 1); }
  };
  return el;
};
function diagEl(d, i) {
  const icon = { ok: "&#10004;", info: "&#8505;", warn: "&#9888;", fail: "&#10006;" };
  const el = h(`<div class="diag">${d.name ? `<div class="small muted">Checked ${esc(d.name)} just now</div>` : ""}<ul>${(d.findings || []).map((f, k) => `<li class="lv-${esc(f.level)}"><span class="ic" aria-hidden="true">${icon[f.level] || "•"}</span><div class="grow"><b>${esc(f.title)}</b><div class="small">${esc(f.text)}</div><div class="row">${f.card ? `<button class="sm ${f.card.safe ? "" : "primary"}" data-fix="${k}">${esc(f.card.label)}</button>` : ""}${f.href ? `<a class="btn sm" href="${esc(f.href)}">${esc(f.fix_label || "Open")}</a>` : ""}${f.kb ? `<button class="sm ghost" data-art="${esc(f.kb)}">Learn more</button>` : ""}</div></div></li>`).join("")}</ul></div>`);
  el.addEventListener("click", (e) => {
    const b = e.target.closest("[data-fix]"); if (!b) return;
    e.stopPropagation();
    const card = d.findings[+b.dataset.fix].card;
    const msg = chat[i]; (msg.actions = msg.actions || []).push(Object.assign({}, card)); store("chat", chat); drawChat();
    if (card.safe) runCard(msg.actions[msg.actions.length - 1], i, msg.actions.length - 1);
  });
  return el;
}
async function showArticle(id) {
  try {
    const a = await apiBase("GET", "/api/jiv/article/" + encodeURIComponent(id));
    const hi = jivPrefs().lang === "hi" && a.body_hi;
    chat.push({ role: "assistant", content: `**${a.title}**\n\n${hi ? a.body_hi : a.body}`, sources: [{ id: a.id, title: a.title, go: a.go }], meta: "Jiv · built-in help" });
    store("chat", chat.slice(-100)); saveHistory(); drawChat();
  } catch (e) { err(e); }
}
actionCard = function (a, i, k) {
  const done = a.state === "done", dis = a.state === "dismissed", fail = a.state === "failed";
  const el = h(`<div class="action card flat ${a.safe ? "safe" : "confirm"}" role="group" aria-label="Action"><div class="row nowrap"><span class="aic" aria-hidden="true">${a.safe ? "&#10140;" : "&#9888;"}</span><div class="grow"><b>${esc(a.label || a.name)}</b>${a.detail ? `<div class="small muted">${esc(a.detail)}</div>` : ""}</div></div>
    <div class="small ${done ? "okc" : fail ? "badc" : "muted"}">${done ? "&#10004; " + esc(a.result || "Done") : dis ? "Not done" : fail ? "&#10006; " + esc(a.result || "Failed") : a.safe ? "" : "This changes your server or settings. Nothing happens until you click."}</div>
    ${a.state ? "" : `<div class="row">${a.safe ? `<button class="sm primary run">Go</button>` : `<button class="primary sm run">Yes, do it</button><button class="ghost sm dis">Not now</button>`}</div>`}
    ${a.undo && done ? `<div class="row"><button class="sm undo">Undo</button></div>` : ""}<div class="aj"></div></div>`);
  if (!a.state) {
    if ($(".dis", el)) $(".dis", el).onclick = () => { chat[i].actions[k].state = "dismissed"; store("chat", chat); drawChat(); };
    $(".run", el).onclick = (e) => busy(e.target, () => runCard(a, i, k, $(".aj", el)));
  }
  if ($(".undo", el)) $(".undo", el).onclick = (e) => busy(e.target, async () => { await api("POST", `/api/sites/${a.undo.site}/rollback`, { release: a.undo.release }); a.undo = null; a.result = "Undone - the version before is live again"; store("chat", chat); drawChat(); });
  return el;
};
const pageHash = { dashboard: "#/dashboard", projects: "#/projects", "new-project": "#/new", templates: "#/new/template", "git-import": "#/new/git", "netlify-import": "#/new/netlify", deploys: "#/deploys", domains: "#/domains", testing: "#/testing", jiv: "#/jiv", hosting: "#/hosting", settings: "#/settings", server: "#/settings/server", "connect-ai": "#/settings/ai", storage: "#/settings/storage", backups: "#/settings/backups", updates: "#/settings/updates", organise: "#/settings/organise", "jiv-settings": "#/settings/jiv", guide: "#/guide", shortcuts: "#/shortcuts", sites: "#/projects", assistant: "#/jiv", "new-site": "#/new" };
// runCard: client actions navigate; everything else runs on the server
// with the id it issued (state-changing ones only with confirmed=true).
async function runCard(a, i, k, out) {
  const set = (state, result, extra) => { const x = chat[i] && chat[i].actions && chat[i].actions[k]; if (x) { x.state = state; x.result = result; Object.assign(x, extra || {}); store("chat", chat); saveHistory(); } drawChat(); };
  const args = a.args || {};
  try {
    if (a.client) {
      switch (a.name) {
        case "open_page": if (args.page === "shortcuts") showShortcuts(); else go(pageHash[args.page] || "#/dashboard"); break;
        case "open_project": go(`#/project/${encodeURIComponent(args.site)}${args.tab && args.tab !== "overview" ? "/" + args.tab : ""}`); break;
        case "set_domain": if (args.domain) store("prefillDomain-" + args.site, args.domain); go(`#/project/${encodeURIComponent(args.site)}/domain`); break;
        case "add_hosting_account": go("#/hosting" + (args.provider ? "?add=" + args.provider : "")); break;
        case "install_brain": go("#/settings/jiv"); break;
        case "setup_server": go("#/setup"); break;
        case "install_ollama": { const r = await installOllama(); if (r === false) return set("", ""); break; }
      }
      return set("done", "Opened");
    }
    const r = await api("POST", `/api/jiv/actions/${a.id}/run`, { confirmed: true });
    if (r.client) return;
    if (r.diagnosis) { chat.push({ role: "assistant", content: r.message || "", diag: r.diagnosis, meta: "Jiv · checked just now" }); set("done", "Checked"); return; }
    if (r.job) {
      const box = out || h(`<div></div>`);
      if (!out) $("#msgs").appendChild(box);
      const j = await jobPanel(r.job, box, { title: a.label, doneTitle: "Done" });
      if (j.error) return set("failed", j.error);
      await loadSites();
      return set("done", "Done");
    }
    if (r.reload) { await refreshOverview(); drawServerChip(); rerender(); }
    set("done", r.message || "Done", r.undo ? { undo: r.undo } : null);
    if (r.href) go(r.href);
  } catch (e) {
    if (e.data && e.data.href) go(e.data.href);
    set("failed", e.message);
  }
}
runAction = function (a, el) { return runCard(a, chat.length - 1, 0, el); };

function saveHistory() { clearTimeout(saveHistory.t); saveHistory.t = setTimeout(() => apiBase("PUT", "/api/jiv/history", { messages: chat.slice(-200) }).catch(() => {}), 600); }
(async () => { // chat history lives in a file on this PC (mirrored in localStorage)
  try { const r = await apiBase("GET", "/api/jiv/history"); if ((r.messages || []).length && !chat.length) { chat = r.messages; drawChat(); } } catch (e) {}
})();

sendChat = async function (text, aboutOverride) {
  text = String(text || "").trim();
  if (!text || chatAbort) return;
  const about = aboutOverride || $("#cIn").dataset.about || "";
  $("#cIn").value = ""; delete $("#cIn").dataset.about;
  $("#suggest").innerHTML = "";
  await ensureAI();
  chat.push({ role: "user", content: text, about });
  const t = curTarget();
  const [target, model] = (t ? t.v : "auto|").split("|");
  const msg = { role: "assistant", content: "", actions: [], status: "Thinking…" };
  chat.push(msg); drawChat();
  chatAbort = new AbortController();
  $("#cSend").classList.add("hidden"); $("#cStop").classList.remove("hidden");
  const history = chat.slice(0, -2).filter(m => m.content && !m.diag).map(m => ({ role: m.role, content: m.content.slice(0, 3000) })).slice(-12);
  const body = { q: text, screen: curScreen(), site: curSite(), history, target, model, lang: jivPrefs().lang || "auto", about };
  const idx = chat.length - 1;
  const redraw = () => { const all = $$("#msgs .msg"); const b = all[all.length - 1]; if (b) { b.replaceWith(msgEl(msg, idx)); $("#msgs").scrollTop = $("#msgs").scrollHeight; } };
  let autoRun = null, needConsent = null;
  try {
    const r = await fetch("/api/jiv/ask", { method: "POST", headers: { "X-Rover": "1", "Content-Type": "application/json" }, body: JSON.stringify(body), signal: chatAbort.signal });
    if (!r.ok) { let m = "HTTP " + r.status; try { m = (await r.json()).error || m; } catch (e) {} throw new Error(m); }
    const reader = r.body.getReader(), dec = new TextDecoder();
    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl).trim(); buf = buf.slice(nl + 1);
        if (!line) continue;
        let ev; try { ev = JSON.parse(line); } catch (e) { continue; }
        switch (ev.t) {
          case "start": msg.meta = `${ev.label}${ev.model && ev.mode !== "brain" ? " · " + ev.model : ""}${ev.mode === "cloud" ? " · cloud" : " · on this PC"}`; break;
          case "status": msg.status = ev.v; break;
          case "note": (msg.notes = msg.notes || []).push(ev.v); break;
          case "text": msg.content += ev.v; break;
          case "diag": msg.diag = ev.diagnosis; break;
          case "action": msg.actions.push(Object.assign({}, ev.card, { suggested: !!ev.suggested })); if (ev.card.safe && ev.card.client && !ev.suggested && autoRun === null) autoRun = msg.actions.length - 1; break;
          case "choices": msg.choices = ev.options; break;
          case "sources": msg.sources = ev.articles; break;
          case "related": msg.related = ev.articles; break;
          case "topics": msg.topics = ev.articles; break;
          case "error": if (ev.kind === "consent") needConsent = ev; else msg.error = ev.v; break;
          default: if (window.jivEvent) window.jivEvent(msg, ev);
        }
        redraw();
      }
    }
  } catch (e) {
    if (e.name === "AbortError") msg.meta = (msg.meta || "") + " · stopped";
    else msg.error = e.message;
  } finally {
    chatAbort = null; msg.status = "";
    if ($("#cSend")) { $("#cSend").classList.remove("hidden"); $("#cStop").classList.add("hidden"); }
    if (!msg.content && !msg.error && !msg.actions.length && !msg.diag && !msg.choices && !msg.topics && !msg.plan && !msg.guide) msg.content = "_(no answer)_";
    store("chat", chat.slice(-100)); saveHistory(); drawChat();
    if (autoRun !== null) runCard(msg.actions[autoRun], idx, autoRun);
  }
  if (needConsent) { // cloud AI: ask once, like 0.4
    chat.splice(-2, 2); drawChat();
    const ok = await confirmBox(`Send this chat to ${esc(needConsent.company)}?`, `<p>This model runs in the cloud. Your question and a short summary of your Rover status (site names, domains, deploy states) are sent to <b>${esc(needConsent.company)}</b>, and their privacy terms apply.</p><p>Never sent: SSH passwords, API tokens, keys or IP addresses.</p><p class="small muted">Jiv's built-in answers and offline brain never leave this PC.</p>`, "Allow and send");
    if (!ok) { $("#cIn").value = text; return; }
    await api("POST", "/api/ai/consent/" + encodeURIComponent(needConsent.conn), {});
    aiInfo = null; await ensureAI();
    return sendChat(text, about);
  }
};
askAbout = function (text) {
  const about = String(text || "").slice(0, 4000);
  jivOpen(true);
  sendChat("Why did this fail and how do I fix it?", about);
};
(function patchClear() { // clearing also clears the history file
  const b = $("#cClear"); const old = b.onclick;
  b.onclick = async (e) => { await old(e); if (!chat.length) { apiBase("PUT", "/api/jiv/history", { messages: [] }).catch(() => {}); jivSuggestFor = ""; jivSuggest(); } };
})();

// Jiv page: offline status + link to Settings > Jiv
route(/^\/jiv$/, async () => {
  const v = view();
  v.className = "page wide jivpage";
  const b = await apiBase("GET", "/api/jiv/brain").catch(() => ({}));
  v.innerHTML = `<div class="head"><span class="jiv-av lg" aria-hidden="true">J</span><div><h1>Jiv</h1><div class="small muted">Your helper. Works offline: built-in answers for every screen and error${b.installed ? `, plus the offline brain (${esc(b.model.short)})` : ""}. Cloud AI only if you choose it.</div></div><span class="grow"></span>${b.installed ? `<span class="badge ok">Offline brain: ${esc((b.status || {}).state === "ready" ? "ready" : "sleeping")}</span>` : `<a class="btn sm primary" href="#/settings/jiv">Get the offline brain</a>`}<a class="btn ghost sm" href="#/settings/jiv">Jiv settings</a></div><div id="jivSlot"></div>`;
  mountJiv(true);
  await ensureAI(); fillModels(); jivSuggest();
  $("#cIn").focus();
}, "jiv");
routes.unshift(routes.pop()); // the 0.5 Jiv page wins over 0.4's

// ---------- Settings > Jiv ----------
settingsTabs.splice(settingsTabs.findIndex(t => t[0] === "ai"), 0, ["jiv", "Jiv"]);
route(/^\/settings\/jiv$/, async () => {
  await refreshOverview();
  const v = view();
  v.innerHTML = `<div class="head"><h1>Settings</h1><span class="grow"></span><a class="btn ghost sm" href="#/guide">Guide &amp; help</a></div>
    <div class="tabs" role="tablist">${settingsTabs.map(([k, l]) => `<button role="tab" data-t="${k}" aria-selected="${k === "jiv"}">${l}</button>`).join("")}</div><div id="sBody" role="tabpanel"></div>`;
  $$(".tabs button", v).forEach(b => b.onclick = () => go(b.dataset.t === "general" ? "#/settings" : "#/settings/" + b.dataset.t));
  await setJiv($("#sBody"));
}, "settings");

async function setJiv(el) {
  const b = await api("GET", "/api/jiv/brain");
  const st = b.status || {}, js = b.settings || {}, gb = (n) => n < 1073741824 ? Math.round(n / 1048576) + " MB" : (n / 1073741824).toFixed(1) + " GB";
  const ramGB = b.ram_total ? Math.round(b.ram_total / 1073741824) : 0;
  el.innerHTML = `<div class="card"><h2>Offline brain</h2>
    <p>Jiv always answers from its built-in help, offline. The <b>offline brain</b> is a small AI model that runs <b>on this PC</b> (no internet, no account), so Jiv understands any wording and explains things in its own words.</p>
    ${b.installed ? `<div class="alert ${st.state === "error" ? "bad" : "ok"}"><b>Installed: ${esc(b.model.name)}</b><div class="small">${st.state === "ready" ? "Running and ready." : st.state === "starting" ? "Loading into memory…" : st.state === "error" ? esc(st.error) : "Sleeping - starts when you ask something."} &middot; engine llama.cpp ${esc(b.build)}</div></div>
      <div class="row"><button id="jStart">${st.state === "ready" ? "Restart" : "Start now"}</button><button id="jStop" class="ghost">Unload from memory</button><button id="jRm" class="ghost danger">Remove</button></div>` : ""}
    <h3>${b.installed ? "Switch model" : "Choose a model"}</h3>
    <p class="small muted">This PC: ${ramGB ? ramGB + " GB RAM" : "RAM unknown"}, ${b.cpus} CPU threads. Both models are free for any use (Apache-2.0) and understand Hindi.</p>
    <div class="brainlist">${b.models.map(m => `<label class="plan brain ${m.id === b.recommended ? "rec" : ""}"><input type="radio" name="bm" value="${esc(m.id)}" ${m.id === (b.installed ? b.model.id : b.recommended) ? "checked" : ""}><span class="grow"><b>${esc(m.name)}</b> ${m.id === b.recommended ? `<span class="badge info">Recommended for this PC</span>` : ""}<div class="small">${esc(m.notes)}</div><div class="small muted">Download ${gb(m.size + b.runtime.size)} &middot; uses about ${m.ram_gb} GB RAM while chatting (best on PCs with ${m.min_pc_gb}+ GB) &middot; ${esc(m.speed)} &middot; ${esc(m.license)}</div></span></label>`).join("")}</div>
    <label class="check small"><input type="checkbox" id="jGpu" ${js.gpu ? "checked" : ""}> Use the graphics card (Vulkan engine, ${gb(b.runtime_gpu.size)}): faster on PCs with a dedicated GPU</label>
    <div class="row"><button id="jDl" class="primary">${b.installed ? "Download this model" : "Download offline brain"}</button><button id="jImp" class="ghost">Use files I already have&hellip;</button></div>
    <p class="hint">Downloaded from the official llama.cpp release and Hugging Face, checked against fixed sha256 fingerprints, resumed if interrupted, and saved in your local app-data folder. After that Jiv needs no internet.${js.mirror ? " Mirror: <code>" + esc(js.mirror) + "</code>" : ""}</p><div id="jJob"></div></div>
    <div class="card"><h2>How Jiv talks</h2>
      <div class="grid2"><div class="field"><label for="jLang">Language</label><select id="jLang"><option value="auto">Automatic (answer in the language I write)</option><option value="en">English</option><option value="hi">हिंदी (Hindi)</option></select></div>
      <div class="field"><label for="jIdle">Unload the brain after</label><select id="jIdle">${[10, 20, 60, 240].map(n => `<option value="${n}" ${(js.idle_min || 20) === n ? "selected" : ""}>${n < 60 ? n + " minutes" : n / 60 + " hour(s)"} idle</option>`).join("")}</select></div></div>
      <label class="switch"><input type="checkbox" id="jAuto" ${js.no_autostart ? "" : "checked"}> Warm up the brain when Rover starts</label>
      <label class="switch"><input type="checkbox" id="jNudge" ${js.no_nudges ? "" : "checked"}> Offer help when a deploy or domain check fails</label>
      <label class="switch"><input type="checkbox" id="jOff" ${js.brain_off ? "" : "checked"}> Use the offline brain when it's installed</label>
      <div class="row"><button id="jHist" class="ghost">Clear chat history</button><a class="btn ghost" href="#/settings/ai">Connect another AI (optional)</a></div>
      <p class="hint">Chat history is saved only on this PC. Passwords, tokens and keys are never sent to any AI.</p></div>`;
  $("#jLang").value = js.lang || "auto";
  const dlLabel = () => { const id = $("input[name=bm]:checked").value, same = b.installed && id === b.model.id && !!$("#jGpu").checked === !!js.gpu; $("#jDl").disabled = same; $("#jDl").textContent = same ? "Installed" : b.installed ? "Download and switch" : "Download offline brain"; };
  $$("input[name=bm]").forEach(r => r.onchange = dlLabel); $("#jGpu").onchange = dlLabel; dlLabel();
  const save = (body) => api("POST", "/api/jiv/settings", body).then(() => toast("Saved", "success")).catch(err);
  $("#jLang").onchange = (e) => { store("jivPrefs", Object.assign(jivPrefs(), { lang: e.target.value })); save({ lang: e.target.value }); };
  $("#jIdle").onchange = (e) => save({ idle_min: +e.target.value });
  $("#jAuto").onchange = (e) => save({ no_autostart: !e.target.checked });
  $("#jNudge").onchange = (e) => { ov.jiv_no_nudges = !e.target.checked; save({ no_nudges: !e.target.checked }); };
  $("#jOff").onchange = (e) => { aiInfo = null; save({ brain_off: !e.target.checked }); };
  $("#jHist").onclick = async () => { if (!await confirmBox("Clear chat history?", "Deletes the conversation with Jiv on this PC.", "Clear")) return; chat = []; store("chat", chat); apiBase("PUT", "/api/jiv/history", { messages: [] }); drawChat(); toast("Cleared", "success"); };
  $("#jDl").onclick = async (e) => {
    const id = $("input[name=bm]:checked").value, m = b.models.find(x => x.id === id), gpu = $("#jGpu").checked;
    const rt = gpu ? b.runtime_gpu : b.runtime;
    if (!await confirmBox("Download the offline brain?", `<p><b>${esc(m.name)}</b> (${gb(m.size)}) and the llama.cpp engine (${gb(rt.size)}). Total <b>${gb(m.size + rt.size)}</b>.</p><p class="small muted">It uses about ${m.ram_gb} GB of memory while chatting and unloads itself when idle. You can keep using Rover while it downloads.</p>`, "Download")) return;
    busy(e.target, async () => { const r = await api("POST", "/api/jiv/brain/install", { model: id, gpu }); const j = await jobPanel(r.job, $("#jJob"), { title: "Downloading the offline brain", doneTitle: "Offline brain ready", failTitle: "Download stopped" }); aiInfo = null; if (!j.error) { toast("Jiv's offline brain is ready", "success"); later(rerender, 800); } });
  };
  $("#jImp").onclick = () => modal(`<h2>Use files I already have</h2><p class="small muted">No download needed, e.g. from a USB stick or another PC. Rover checks the file and recognises the two recommended models by their fingerprint.</p>
    <div class="field"><label for="iM">Model file (.gguf)</label><input id="iM" placeholder="D:\\AI\\Qwen3-4B-Instruct-2507-Q4_K_M.gguf"></div>
    <div class="field"><label for="iR">llama.cpp engine (the llama-…-bin-win-cpu-x64.zip file or its folder)</label><input id="iR" placeholder="D:\\AI\\llama-${esc(b.build)}-bin-win-cpu-x64.zip"></div><div id="iErr"></div>
    <div class="row end"><button data-close>Cancel</button><button id="iGo" class="primary">Use these files</button></div>`, (m, close) => {
    $("#iGo", m).onclick = (e) => busy(e.target, async () => { try { await api("POST", "/api/jiv/brain/import", { model_path: $("#iM", m).value, runtime_path: $("#iR", m).value }); toast("Offline brain set up", "success"); aiInfo = null; close(true); rerender(); } catch (x) { $("#iErr", m).innerHTML = `<div class="alert bad small">${esc(x.message)}</div>`; } });
  });
  if ($("#jStart")) {
    $("#jStart").onclick = (e) => busy(e.target, async () => { await api("POST", "/api/jiv/brain/stop"); await api("POST", "/api/jiv/brain/start"); toast("Offline brain is ready", "success"); rerender(); });
    $("#jStop").onclick = (e) => busy(e.target, async () => { await api("POST", "/api/jiv/brain/stop"); rerender(); });
    $("#jRm").onclick = async (e) => { if (!await confirmBox("Remove the offline brain?", "Deletes the model and engine files from this PC. Jiv keeps working with its built-in answers.", "Remove", true)) return; busy(e.target, async () => { await api("DELETE", "/api/jiv/brain"); aiInfo = null; rerender(); }); };
  }
}

// ---------- Command palette (Ctrl+K) ----------
const palCommands = [
  ["New project", "#/new", "publish create site upload"], ["Dashboard", "#/dashboard", "home"], ["Projects", "#/projects", "sites list"],
  ["Deploys", "#/deploys", "history versions"], ["Domains", "#/domains", "dns https"], ["Hosting accounts", "#/hosting", "servers vps providers digitalocean hetzner"],
  ["Add a hosting account", "#/hosting?add=", "connect provider vps"], ["App Testing", "#/testing", "test preview"], ["Jiv", "#/jiv", "help assistant chat ai"],
  ["Settings", "#/settings", "options"], ["Jiv settings & offline brain", "#/settings/jiv", "model offline ai brain hindi language"],
  ["Server settings", "#/settings/server", "coolify ssh"], ["Backups & alerts", "#/settings/backups", "backup email"], ["Connect AI", "#/settings/ai", "openai claude ollama"],
  ["Check for updates", "#/settings/updates", "version"], ["Guide & help", "#/guide", "docs manual"], ["Keyboard shortcuts", "#/shortcuts", "keys hotkeys"],
];
function openPalette(initial) {
  if ($("#palette")) return;
  const el = h(`<div id="palette" class="palwrap" role="dialog" aria-modal="true" aria-label="Command palette"><div class="palette"><input id="palIn" placeholder="Type a command, project, server or question…" autocomplete="off" aria-controls="palList"><div id="palList" role="listbox"></div><div class="pfoot small muted"><span><span class="kbd">↑</span><span class="kbd">↓</span> move</span><span><span class="kbd">Enter</span> open</span><span><span class="kbd">Esc</span> close</span><span class="grow"></span><span>Searches commands, projects, servers and Jiv's help</span></div></div></div>`);
  document.body.appendChild(el);
  const inp = $("#palIn"), list = $("#palList");
  let items = [], sel = 0, seq = 0;
  const close = () => { el.remove(); };
  const draw = () => {
    let grp = "";
    list.innerHTML = items.map((it, k) => { const g = it.g !== grp ? `<div class="pgroup">${esc(it.g)}</div>` : ""; grp = it.g; return g + `<div class="pitem ${k === sel ? "sel" : ""}" role="option" aria-selected="${k === sel}" data-k="${k}"><span class="pi" aria-hidden="true">${it.i || "›"}</span><span class="grow">${esc(it.label)}${it.sub ? `<span class="small muted"> · ${esc(it.sub)}</span>` : ""}</span></div>`; }).join("") || `<div class="muted small pad">Nothing found.</div>`;
    const s = $(".pitem.sel", list); s && s.scrollIntoView({ block: "nearest" });
  };
  const pick = (it) => { if (!it) return; close(); it.run(); };
  const update = async () => {
    const q = inp.value.trim().toLowerCase(), my = ++seq;
    const m = (s) => !q || q.split(/\s+/).every(w => s.toLowerCase().includes(w));
    items = [];
    palCommands.filter(c => m(c[0] + " " + c[2])).slice(0, q ? 6 : 8).forEach(c => items.push({ g: "Go to", label: c[0], i: "→", run: () => go(c[1]) }));
    sites.filter(s => m((s.name || "") + " " + (s.slug || "") + " " + (s.domain || ""))).slice(0, 6).forEach(s => items.push({ g: "Projects", label: s.name || s.slug, sub: s.domain || "", i: "▣", run: () => go("#/project/" + encodeURIComponent(s.slug)) }));
    ((srvCache && srvCache.servers) || []).filter(s => m((s.name || "") + " server")).slice(0, 4).forEach(s => items.push({ g: "Servers", label: s.name, sub: s.active ? "active" : "switch to it", i: "▤", run: () => s.active ? go("#/hosting") : api("POST", `/api/servers/${s.id}/activate`).then(() => { toast("Now using " + s.name, "success"); refreshOverview().then(() => { drawServerChip(); rerender(); }); }).catch(err) }));
    if (q) items.push({ g: "Ask Jiv", label: `Ask Jiv: “${inp.value.trim()}”`, i: "J", run: () => { jivOpen(true); sendChat(inp.value.trim()); } });
    sel = 0; draw();
    if (q.length >= 3) {
      try {
        const r = await apiBase("GET", "/api/jiv/search?q=" + encodeURIComponent(q) + "&screen=" + encodeURIComponent(curScreen()));
        if (my !== seq) return;
        const ask = items.pop();
        (r.results || []).slice(0, 4).forEach(a => items.push({ g: "Help", label: a.title, sub: a.snippet, i: "?", run: () => { jivOpen(true); showArticle(a.id); } }));
        items.push(ask); draw();
      } catch (e) {}
    }
  };
  inp.oninput = () => { clearTimeout(update.t); update.t = setTimeout(update, 80); };
  inp.onkeydown = (e) => {
    if (e.key === "ArrowDown") { sel = Math.min(items.length - 1, sel + 1); draw(); e.preventDefault(); }
    else if (e.key === "ArrowUp") { sel = Math.max(0, sel - 1); draw(); e.preventDefault(); }
    else if (e.key === "Enter") { e.preventDefault(); pick(items[sel]); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); }
  };
  list.onclick = (e) => { const p = e.target.closest(".pitem"); if (p) pick(items[+p.dataset.k]); };
  el.onclick = (e) => { if (e.target === el) close(); };
  inp.value = initial || ""; update(); inp.focus();
}

// ---------- Keyboard shortcuts ----------
const shortcutList = [["Ctrl + K", "Command palette: go anywhere, find a project, search help"], ["Ctrl + J", "Open or close Jiv"], ["N", "New project"], ["?", "Show these shortcuts"], ["G then D", "Dashboard"], ["G then P", "Projects"], ["G then H", "Hosting accounts"], ["G then S", "Settings"], ["G then J", "Jiv page"], ["Esc", "Close a window or the palette"], ["Ctrl + Z", "Undo the last change (when a toast offers Undo)"]];
const keysHTML = (k) => k.split(" ").map(x => ["+", "then"].includes(x) ? ` ${x} ` : `<span class="kbd">${esc(x)}</span>`).join("");
function showShortcuts() { modal(`<h2>Keyboard shortcuts</h2><table class="shortcuts">${shortcutList.map(([k, d]) => `<tr><td>${keysHTML(k)}</td><td>${esc(d)}</td></tr>`).join("")}</table><div class="row end"><button data-close class="primary">Got it</button></div>`); }
route(/^\/shortcuts$/, () => {
  const v = view();
  v.innerHTML = `<div class="head"><h1>Keyboard shortcuts</h1></div><div class="card"><table class="shortcuts">${shortcutList.map(([k, d]) => `<tr><td>${keysHTML(k)}</td><td>${esc(d)}</td></tr>`).join("")}</table><p class="hint">Single-letter shortcuts work when you're not typing in a box.</p></div>`;
}, "settings");
let gPending = 0;
document.addEventListener("keydown", (e) => {
  const typing = e.target.closest && e.target.closest("input, textarea, select, [contenteditable]");
  const mod = e.ctrlKey || e.metaKey;
  if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === "k") { e.preventDefault(); $("#palette") ? $("#palette").remove() : openPalette(); return; }
  if (mod && !e.shiftKey && !e.altKey && e.key.toLowerCase() === "j") { e.preventDefault(); const j = $("#jiv"); const on = !(j && j.classList.contains("open")); jivOpen(on); if (on) $("#cIn").focus(); return; }
  if (mod && e.key.toLowerCase() === "z" && !typing && lastUndo) { e.preventDefault(); const u = lastUndo; lastUndo = null; u(); return; }
  if (typing || mod || e.altKey || $(".modal-back, .modal, #palette")) return;
  if (gPending && Date.now() - gPending < 1200) {
    const to = { d: "#/dashboard", p: "#/projects", h: "#/hosting", s: "#/settings", j: "#/jiv" }[e.key.toLowerCase()];
    gPending = 0; if (to) { e.preventDefault(); go(to); } return;
  }
  if (e.key === "g" || e.key === "G") { gPending = Date.now(); return; }
  if (e.key === "?") { e.preventDefault(); showShortcuts(); return; }
  if (e.key === "n" || e.key === "N") { e.preventDefault(); go("#/new"); }
});
(function palButton() {
  const b = h(`<button id="palBtn" class="palbtn" type="button" title="Search everything (Ctrl+K)"><span aria-hidden="true">&#128269;</span><span class="lbl">Search</span><span class="kbd lbl">Ctrl K</span></button>`);
  b.onclick = () => openPalette();
  const nav = $("#nav"); nav.parentElement.insertBefore(b, nav);
})();

// ---------- Inline help ----------
// Every page title gets "Explain this page"; every red error box gets "Ask Jiv why".
function decorate() {
  const head = $("#view > .head, #view-testing > .head");
  if (head && !$(".helpq", head) && !head.closest(".jivpage")) {
    const b = h(`<button class="helpq ghost sm" type="button" title="Jiv explains this page">? Explain this page</button>`);
    b.onclick = () => { jivOpen(true); sendChat("Explain this page: what can I do here?"); };
    const grow = $(".grow", head); grow ? grow.after(b) : head.appendChild(b);
  }
  $$("#view .alert.bad").forEach(a => {
    if (a.dataset.why || $(".ask, .why", a) || a.closest("#jiv, .modal")) return;
    a.dataset.why = "1";
    const txt = a.textContent.trim(); if (txt.length < 8) return;
    const b = h(`<button class="why sm" type="button">Ask Jiv why</button>`);
    b.onclick = () => askAbout(txt);
    a.appendChild(b);
  });
}
new MutationObserver(() => { clearTimeout(decorate.t); decorate.t = setTimeout(decorate, 60); }).observe($("#main"), { childList: true, subtree: true });

// ---------- Proactive help ----------
let nudgesOff = false;
function nudge(text, q, about) {
  if (nudgesOff || $(".nudge")) return;
  const n = h(`<div class="nudge" role="status"><span class="jiv-av" aria-hidden="true">J</span><div class="grow"><b>Jiv:</b> ${esc(text)}</div><button class="sm primary">Help me fix it</button><button class="ghost sm x" aria-label="Dismiss">&times;</button></div>`);
  $(".primary", n).onclick = () => { n.remove(); jivOpen(true); about ? askAbout(about) : sendChat(q); };
  $(".x", n).onclick = () => n.remove();
  document.body.appendChild(n);
  setTimeout(() => n.remove(), 30000);
}
const jobPanel04 = jobPanel;
jobPanel = function (id, el, opt) {
  return jobPanel04(id, el, opt).then(j => {
    if (j && j.error && !(el && el.closest && el.closest("#jiv"))) nudge("That didn't work. I can look at what went wrong.", "", j.error);
    return j;
  });
};
(async () => {
  try { const b = await apiBase("GET", "/api/jiv/brain"); nudgesOff = !!(b.settings && b.settings.no_nudges); if (b.settings && b.settings.lang && !jivPrefs().lang) store("jivPrefs", Object.assign(jivPrefs(), { lang: b.settings.lang })); } catch (e) {}
  decorate();
  if (aiInfo || $("#jiv.open")) { aiInfo = null; ensureAI(); }
})();
