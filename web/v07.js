// Rover 0.7: one project on several servers (automatic failover), home
// servers (Linux at home, Raspberry Pi) and hosting on this PC.
"use strict";

const netKindName = (k) => ({ wifi: "Wi-Fi", ethernet: "cable (Ethernet)", "usb-tether": "phone over USB", hotspot: "phone hotspot", mobile: "mobile data", vpn: "VPN" })[k] || k || "network";
const kindIcon = (k) => k === "home" ? "&#127968;" : k === "thispc" ? "&#128187;" : "&#9729;&#65039;";
const kindName = (k) => k === "home" ? "Home server" : k === "thispc" ? "This PC" : "Rented server";
const healthBadge = (h) => h === "up" ? `<span class="badge ok">&#9679; Answering</span>` : h === "down" ? `<span class="badge bad">&#9679; Not answering</span>` : `<span class="badge">Not checked yet</span>`;
const repBadge = (v) => v.status === "live" ? `<span class="badge ok">Live</span>` : v.status === "failed" ? `<span class="badge bad" title="${esc(v.error || "")}">Not updated</span>` : v.status === "setting-up" ? `<span class="badge info">Setting up&hellip;</span>` : `<span class="badge">${esc(v.status || "unknown")}</span>`;

// ---------- Hosting tab: servers of this project ----------
const _hostingTab06 = hostingTab;
hostingTab = async function (s, b) {
  await _hostingTab06(s, b);
  const card = h(`<div class="card ms7" id="msCard"><div class="card-head"><h2>Servers for this project</h2><span class="grow"></span><span id="msMode"></span></div><div id="msBody" class="small muted"><span class="spin"></span> Loading&hellip;</div></div>`);
  const first = b.querySelector(".card");
  if (first && first.nextSibling) b.insertBefore(card, first.nextSibling); else b.appendChild(card);
  drawServersCard(s, false);
};

async function drawServersCard(s, check) {
  const body = $("#msBody"); if (!body) return;
  let r;
  try { r = await api("GET", `/api/sites/${s.slug}/replicas${check ? "?check=1" : ""}`); } catch (e) { body.innerHTML = `<div class="alert warn">${esc(e.message)}</div>`; return; }
  const multi = r.servers.length > 1;
  $("#msMode").innerHTML = multi ? `<span class="badge accent">${r.mode === "together" ? "Together (Cloudflare routes visitors)" : "Primary + backup"}</span>` : "";
  if (!multi) {
    body.innerHTML = `<div class="ms7intro"><div class="ms7art" aria-hidden="true">&#9729;&#65039;&#8596;&#127968;</div><div><p><b>Keep this site online even if a server goes down.</b> Run it on a second server too: a rented one, a machine at home, or this PC. Every deploy then goes to all of them, and Rover sends visitors to a server that works.</p>
      <ul class="hlist"><li>&#10004; Same version on every server (each switches only when all files arrived)</li><li>&#10004; Automatic switching with your domain on Cloudflare (free plan)</li><li>&#10004; A note in Rover when a server stops or comes back</li></ul>
      <div class="row"><button class="primary" id="msAdd">+ Add a backup server</button><a class="btn ghost" href="#/hosting?add=">Get another server</a><a class="btn ghost" href="#/hosting/home">Use a machine at home</a></div></div></div>`;
    $("#msAdd").onclick = () => addServerWizard(s, r);
    return;
  }
  const rows = r.servers.map(v => `<tr data-id="${esc(v.id)}"><td><div class="row nowrap"><span class="kic" title="${esc(kindName(v.kind))}">${kindIcon(v.kind)}</span><div><b>${esc(v.label)}</b><div class="small muted">${esc(kindName(v.kind))}${v.engine === "lite" ? " &middot; lightweight" : ""}${v.ip && v.kind !== "thispc" ? " &middot; " + esc(v.ip) : ""}</div></div></div></td>
      <td>${v.role === "main" ? `<span class="badge info">Main</span>` : v.role === "backup" ? `<span class="badge">Backup</span>` : `<span class="badge">Also serving</span>`}${v.serving ? ` <span class="badge ok" title="Visitors are sent here now">&#128101; Visitors</span>` : ""}</td>
      <td>${repBadge(v)}<div class="small muted">${v.in_sync === false ? `<span class="warnc">still has ${v.actual ? "version " + esc(v.actual) : "nothing"}</span>` : v.release ? "version " + esc(v.release) : ""}${v.in_sync === true ? " &middot; &#10004; in sync" : ""}${v.check_err ? ` &middot; <span class="warnc">${esc(v.check_err)}</span>` : ""}</div>${v.error && v.status === "failed" ? `<div class="small warnc clip2" title="${esc(v.error)}">${esc(v.error)}</div>` : ""}</td>
      <td>${healthBadge(v.health)}</td>
      <td class="nowrap">${v.status !== "live" || v.in_sync === false ? `<button class="sm primary" data-a="retry">Retry</button>` : ""}${r.mode === "backup" && !v.serving && v.health !== "down" ? `<button class="sm" data-a="switch">Switch now</button>` : ""}${v.role !== "main" ? `<button class="sm ghost" data-a="menu" title="More">&#8943;</button>` : ""}</td></tr>`).join("");
  const anyBad = r.servers.some(v => v.status !== "live" || v.in_sync === false);
  const fo = r.failover || {};
  body.classList.remove("muted");
  body.innerHTML = `${anyBad ? `<div class="alert warn">Not every server has the live version. Click <b>Retry</b> to send it again (from the copy on this PC). Visitors keep getting the site from the servers that have it.</div>` : ""}
    <div class="tblwrap"><table class="tbl ms7t"><thead><tr><th>Server</th><th>Role</th><th>Version</th><th>Health</th><th></th></tr></thead><tbody>${rows}</tbody></table></div>
    <div class="row" style="margin-top:10px"><button id="msCheck" class="sm">&#10227; Check health &amp; sync</button><button id="msSyncAll" class="sm ${anyBad ? "primary" : ""}">Send live version to all</button><button id="msAdd" class="sm ghost">+ Add another server</button>${check && r.in_sync ? `<span class="badge ok">&#10004; All servers have version ${esc(r.release)}</span>` : ""}</div>
    <div class="ms7grid">
      <div class="card flat"><h3>How visitors find a working server</h3>
        <label class="radio"><input type="radio" name="msMode" value="backup" ${r.mode !== "together" ? "checked" : ""}> <b>Primary + backup</b> <span class="muted">(recommended)</span><div class="hint">Visitors go to the main server. Rover checks every server each minute; if the main one stops answering 3 times in a row, it points your domain at the backup through Cloudflare (live in seconds). Needs Rover running.</div></label>
        <label class="radio"><input type="radio" name="msMode" value="together" ${r.mode === "together" ? "checked" : ""}> <b>Together</b><div class="hint">All servers answer at once through one Cloudflare tunnel. If one is off, Cloudflare uses the others by itself, even while this PC is off. Needs the domain on Cloudflare.</div></label>
        ${r.mode !== "together" ? `<label class="chk"><input type="checkbox" id="msAuto" ${fo.auto ? "checked" : ""}> Switch automatically (otherwise Rover asks you)</label><label class="chk"><input type="checkbox" id="msBack" ${fo.switch_back ? "checked" : ""}> Go back to the main server when it is healthy again</label>` : ""}
      </div>
      <div class="card flat"><h3>Automatic switching</h3>${r.can_switch ? `<p class="okc">&#10004; Ready. ${esc(r.domain)} is on Cloudflare, so Rover can switch it.</p>` : `<div class="alert info small">${esc(r.switch_note)}</div><div class="row">${r.cloudflare ? "" : `<a class="btn sm" href="#/domains">Connect Cloudflare (free)</a>`}<a class="btn sm ghost" href="#/project/${esc(s.slug)}/domain">Domain settings</a></div><p class="hint">Not on Cloudflare? You can still switch by hand: change the A record of ${esc(r.domain || "your domain")} at your registrar to the backup server's IP. Rover tells you when.</p>`}
        ${(fo.events || []).length ? `<h4>Recent switches</h4><ul class="small evlist">${fo.events.slice(0, 5).map(e => `<li><span class="muted">${esc(ago(e.time))}</span> ${esc(e.text)}</li>`).join("")}</ul>` : ""}
        ${r.tunnel ? `<h4>Tunnel connections</h4><p class="small">${esc(r.tunnel.status)} &middot; ${(r.tunnel.connections || []).length} connection(s)${(r.tunnel.connections || []).length ? ": " + esc([...new Set(r.tunnel.connections.map(c => c.colo_name))].join(", ")) : ""}</p>` : ""}${r.tunnel_error ? `<p class="small warnc">${esc(r.tunnel_error)}</p>` : ""}
      </div></div><div id="msJob"></div>`;
  $("#msCheck").onclick = (e) => busy(e.target, async () => { await api("POST", `/api/sites/${s.slug}/failover/check`); await drawServersCard(s, true); });
  $("#msAdd").onclick = () => addServerWizard(s, r);
  $("#msSyncAll").onclick = (e) => syncServers(s, [], e.target);
  b07modeHandlers(s, r);
  body.querySelector("tbody").onclick = async (e) => {
    const btn = e.target.closest("button[data-a]"); if (!btn) return;
    const id = btn.closest("tr").dataset.id, v = r.servers.find(x => x.id === id);
    if (btn.dataset.a === "retry") return syncServers(s, [id], btn);
    if (btn.dataset.a === "switch") {
      if (!await confirmBox(`Send visitors to ${v.label}?`, `<p>Rover points <b>${esc(r.domain || "the domain")}</b> at <b>${esc(v.label)}</b> through Cloudflare. Visitors see it within seconds.${v.health === "down" ? ` <b class="badc">This server is not answering right now.</b>` : ""}</p>`, "Switch now")) return;
      return busy(btn, async () => { await api("POST", `/api/sites/${s.slug}/failover/switch`, { to: id }); toast("Visitors now go to " + v.label, "success"); drawServersCard(s, false); });
    }
    if (btn.dataset.a === "menu") {
      modal(`<h2>${esc(v.label)}</h2><p class="small muted">${esc(kindName(v.kind))} &middot; ${esc(v.role)}</p><div class="col" style="gap:8px">${v.kind === "thispc" ? "" : `<button data-m="main">Make this the main server</button>`}<button data-m="remove" class="danger">Stop using it for this project</button><button data-m="close" class="ghost">Close</button></div>`, (m, close) => {
        m.onclick = async (ev) => {
          const x = ev.target.closest("button[data-m]"); if (!x) return;
          if (x.dataset.m === "close") return close();
          if (x.dataset.m === "main") { close(); if (!await confirmBox(`Make ${v.label} the main server?`, `<p>The project's settings and deploys use ${esc(v.label)} as the main server; the old main becomes a backup. Nothing is deleted.</p>`, "Make main")) return; await api("POST", `/api/sites/${s.slug}/servers/${id}/main`).catch(err); await loadSites(); rerender(); }
          if (x.dataset.m === "remove") { close(); if (!await confirmBox(`Stop using ${v.label} for ${s.name}?`, `<p>New deploys no longer go there. The copy on ${esc(v.label)} keeps running until you delete it there; Rover does not delete anything.</p>`, "Stop using it", true)) return; await api("POST", `/api/sites/${s.slug}/servers/${id}/remove`).catch(err); drawServersCard(s, false); }
        };
      });
    }
  };
}

function b07modeHandlers(s, r) {
  document.querySelectorAll("input[name=msMode]").forEach(el => el.onchange = async () => {
    const mode = el.value;
    const ok = await confirmBox(mode === "together" ? "Switch to Together mode?" : "Switch to Primary + backup?", mode === "together"
      ? `<p>Rover creates a Cloudflare tunnel for ${esc(s.name)}, starts it on every server and points ${esc(r.domain || "the domain")} at it. Visitors are then served by whichever server is up.</p><p class="small muted">Needs Cloudflare connected with the permissions <b>Cloudflare Tunnel: Edit</b> and <b>DNS: Edit</b>. This PC is not used in this mode.</p>`
      : `<p>Visitors go to the main server; Rover switches to a backup when it goes down.</p>`, "Switch mode");
    if (!ok) { drawServersCard(s, false); return; }
    try { const x = await api("POST", `/api/sites/${s.slug}/ha`, { mode }); if (x.job) await jobPanel(x.job, $("#msJob"), { title: "Changing mode", doneTitle: "Mode changed" }); } catch (e) { err(e); }
    drawServersCard(s, false);
  });
  const save = async (ev, body, label) => {
    if (!await confirmBox(label, `<p>${ev.target.checked ? "Turn this on" : "Turn this off"} for ${esc(s.name)}?</p>`, "Save")) { ev.target.checked = !ev.target.checked; return; }
    await api("POST", `/api/sites/${s.slug}/ha`, body).catch(err); toast("Saved", "success");
  };
  if ($("#msAuto")) $("#msAuto").onchange = (ev) => save(ev, { auto: ev.target.checked }, "Automatic switching");
  if ($("#msBack")) $("#msBack").onchange = (ev) => save(ev, { switch_back: ev.target.checked }, "Switch back automatically");
}

async function syncServers(s, ids, btn) {
  if (!await confirmBox("Send the live version again?", `<p>Rover sends the live version (from the copy saved on this PC) to ${ids.length ? "this server" : "every server that does not have it"}, then switches it over. Servers that already have it are not touched.</p>`, "Send it")) return;
  busy(btn, async () => { const x = await api("POST", `/api/sites/${s.slug}/replicas/sync`, { servers: ids }); if (x.nothing) { toast("Every server already has the live version", "success"); return; } await jobPanel(x.job, $("#msJob"), { title: "Sending the live version", doneTitle: "Servers in sync" }); drawServersCard(s, true); });
}

function addServerWizard(s, r) {
  const opts = (r.addable || []);
  modal(`<h2>Run ${esc(s.name)} on another server</h2>
    ${opts.length ? `<div class="field"><label>Which server?</label><div class="col" id="awList">${opts.map((o, i) => `<label class="awpick ${o.ready ? "" : "dis"}"><input type="radio" name="aw" value="${esc(o.id)}" ${o.ready ? "" : "disabled"} ${i === opts.findIndex(x => x.ready) ? "checked" : ""}> <span class="kic">${kindIcon(o.kind)}</span> <b>${esc(o.label)}</b> <span class="small muted">${esc(kindName(o.kind))}${o.engine === "lite" ? ", lightweight" : ""}</span>${o.ready ? "" : `<div class="small warnc">${esc(o.why)}</div>`}</label>`).join("")}</div></div>`
      : `<div class="alert info">You have no other server yet.</div>`}
    <div class="row aw7more"><span class="small muted">Or add a new one:</span><a class="btn sm" href="#/hosting?add=">Rent a server</a><a class="btn sm" href="#/hosting/home">Machine at home</a><a class="btn sm" href="#/thispc">This PC</a></div>
    ${r.servers.length < 2 ? `<div class="field"><label>How should visitors be split?</label><label class="radio"><input type="radio" name="am" value="backup" checked> <b>Primary + backup</b>: the new server waits as a backup (recommended)</label><label class="radio"><input type="radio" name="am" value="together"> <b>Together</b>: both answer at once through Cloudflare</label></div>` : ""}
    <p class="hint">Rover sets the project up there, sends the live version, and from then on every deploy goes to all servers. Your site stays online the whole time.</p>
    <div class="row end"><button class="ghost" data-x>Cancel</button><button class="primary" id="awGo" ${opts.some(o => o.ready) ? "" : "disabled"}>Add server</button></div><div id="awJob"></div>`, (m, close) => {
    $("[data-x]", m).onclick = () => close();
    $("#awGo", m).onclick = (e) => busy(e.target, async () => {
      const sel = $("input[name=aw]:checked", m); if (!sel) return;
      const mode = ($("input[name=am]:checked", m) || {}).value || r.mode || "backup";
      const o = opts.find(x => x.id === sel.value);
      if (!await confirmBox(`Add ${o.label}?`, `<p>Rover creates ${esc(s.name)} on <b>${esc(o.label)}</b> and sends it the live version.</p>`, "Add server")) return;
      const x = await api("POST", `/api/sites/${s.slug}/servers`, { server: sel.value, mode });
      const j = await jobPanel(x.job, $("#awJob", m), { title: "Adding the server", doneTitle: "Server added" });
      if (!j.error) later(() => { close(); rerender(); }, 900);
    });
  });
}

// ---------- Hosting accounts: "My own server (at home)" ----------
{
  const hr = routes.find(x => String(x.re).includes("hosting(?:"));
  if (hr) {
    const orig = hr.fn;
    hr.fn = async (...a) => {
      await orig(...a);
      const v = view(); const at = $("#hAcc", v); if (!at) return;
      const sec = h(`<div><h2 class="sect">My own server (at home)</h2><div class="srvgrid home7">
        <div class="card flat"><div class="row nowrap"><span class="kic big">&#127968;</span><div><b>A Linux machine at home</b><div class="small muted">Old PC, mini PC or Raspberry Pi on your home network</div></div></div><p class="small">Rover logs in once with the password, installs what is needed (a light mode for small machines) and connects it to the internet through Cloudflare: no router changes, works behind any home internet.</p><a class="btn primary sm" href="#/hosting/home">Add a home server</a></div>
        <div class="card flat"><div class="row nowrap"><span class="kic big">&#128187;</span><div><b>This PC</b><div class="small muted">Rover serves sites from this Windows computer</div></div></div><p class="small">Good as a backup or for small sites. Rover keeps the PC awake, starts with Windows and warns you when the internet or power changes.</p><a class="btn sm" href="#/thispc">Set up This PC</a></div>
      </div></div>`);
      at.parentNode.insertBefore(sec, at.previousElementSibling);
      api("GET", "/api/servers").then(sv => (sv.servers || []).forEach(x => {
        const card = $(`.srvcard[data-id="${CSS.escape(x.id)}"]`, v); if (!card) return;
        const logo = $(".plogo", card), sub = $(".grow .small", card);
        if (logo && (x.kind === "home" || x.kind === "thispc")) { logo.textContent = ""; logo.insertAdjacentHTML("afterbegin", kindIcon(x.kind)); logo.classList.add("plogo7"); logo.title = kindName(x.kind); }
        if (sub && x.kind === "thispc") sub.textContent = "Built-in web server on this Windows PC";
        const tags = [x.lite ? `<span class="badge">lightweight</span>` : "", x.tunnel ? `<span class="badge info">Cloudflare tunnel</span>` : ""].join(" ").trim();
        if (sub && tags) sub.insertAdjacentHTML("afterend", `<div class="tags7">${tags}</div>`);
        if (x.kind === "thispc") { const act = $("[data-a=activate]", card); if (act) act.outerHTML = `<a class="btn sm primary" href="#/thispc">Open This PC</a>`; const ns = $(".small .warnc", card); if (ns) ns.textContent = "backup or extra server only"; }
      })).catch(() => {});
    };
  }
}

route(/^\/hosting\/home$/, async () => {
  const v = view(); v.className = "page";
  const g = await api("GET", "/api/home/guide").catch(() => ({ tradeoffs: [], portforward: [] }));
  v.innerHTML = `<div class="head"><a class="btn ghost sm" href="#/hosting">&larr; Hosting accounts</a><h1>Add a server at home</h1></div>
    <div class="card"><h2>Before you start</h2><ul class="tradeoffs">${g.tradeoffs.map(t => `<li>${esc(t)}</li>`).join("")}</ul></div>
    <div class="card"><h2>1. The machine</h2><p class="small muted">Any 64-bit Linux works best (Ubuntu, Debian, Raspberry Pi OS). Turn on SSH (Raspberry Pi Imager: <i>Enable SSH</i>; Ubuntu: <code>sudo apt install openssh-server</code>).</p>
      <div class="grid2"><div class="field"><label for="hmHost">Address on your network</label><input id="hmHost" placeholder="192.168.1.50 or raspberrypi.local"><p class="hint">Your router's device list shows it.</p></div><div class="field"><label for="hmLabel">Name</label><input id="hmLabel" placeholder="Raspberry Pi in the study"></div>
      <div class="field"><label for="hmUser">User</label><input id="hmUser" value="pi" placeholder="root, pi, ubuntu&hellip;"></div><div class="field"><label for="hmPort">SSH port</label><input id="hmPort" value="22" inputmode="numeric"></div>
      <div class="field"><label for="hmPw">Password</label><input id="hmPw" type="password" autocomplete="off"><p class="hint">&#128274; Used once to log in and install Rover's own key. Never saved, never sent to any AI.</p></div></div>
      <div class="row"><button class="primary" id="hmConn">Connect</button><span id="hmInfo" class="small muted"></span></div></div>
    <div class="card" id="hmStep2" hidden><h2>2. How it should run</h2>
      <div class="field"><label>Hosting mode</label><label class="radio"><input type="radio" name="hmEng" value="auto" checked> <b>Choose for me</b>: lightweight on small machines (under 2 GB memory or 32-bit), full engine otherwise</label><label class="radio"><input type="radio" name="hmEng" value="lite"> <b>Lightweight</b>: plain nginx, about 10 MB of memory. Static sites only.</label><label class="radio"><input type="radio" name="hmEng" value="coolify"> <b>Full</b>: Coolify, like on rented servers (needs 2 GB+)</label></div>
      <div class="field"><label>How visitors reach it</label>
        <label class="radio"><input type="radio" name="hmReach" value="tunnel" checked> <b>Cloudflare tunnel</b> (recommended): works behind any router, CGNAT and mobile data. Needs a free Cloudflare account with your domain.</label>
        <label class="radio"><input type="radio" name="hmReach" value="portforward"> <b>Port forwarding</b> (advanced): open ports 80 and 443 on your router.</label>
        <label class="radio"><input type="radio" name="hmReach" value="lan"> <b>Only my home network</b> for now</label></div>
      <div id="hmPF" class="alert info small" hidden><b>Port forwarding steps</b><ol>${g.portforward.map(t => `<li>${esc(t)}</li>`).join("")}</ol></div>
      <label class="chk"><input type="checkbox" id="hmKey" checked> Remember this server (Rover's own key, so the password is not needed again)</label>
      <label class="chk"><input type="checkbox" id="hmUpd" checked> Install security updates automatically</label>
      <div class="row" style="margin-top:10px"><button class="primary" id="hmGo">Set up the server</button></div><div id="hmJob"></div></div>`;
  document.querySelectorAll("input[name=hmReach]").forEach(r => r.onchange = () => { $("#hmPF").hidden = $("input[name=hmReach]:checked").value !== "portforward"; });
  $("#hmConn").onclick = (e) => busy(e.target, async () => {
    const host = $("#hmHost").value.trim(); if (!host) { toast("Enter the machine's address", "error"); return; }
    if (!await confirmBox(`Connect to ${host}?`, `<p>Rover logs in to <b>${esc(host)}</b> over SSH as <b>${esc($("#hmUser").value || "root")}</b>. It becomes the active server.</p>`, "Connect")) return;
    await api("POST", "/api/home/add", { host, label: $("#hmLabel").value, user: $("#hmUser").value, port: +$("#hmPort").value || 22 });
    const c = await api("POST", "/api/ssh/connect", { password: $("#hmPw").value, passphrase: "" });
    $("#hmPw").value = "";
    $("#hmInfo").innerHTML = `&#10004; Connected. ${esc((c.info || "").split("\n").slice(0, 3).join(" · "))}`;
    $("#hmStep2").hidden = false; drawServerChip();
  });
  $("#hmGo").onclick = (e) => busy(e.target, async () => {
    const engine = $("input[name=hmEng]:checked").value, reach = $("input[name=hmReach]:checked").value;
    if (!await confirmBox("Set up this machine?", `<p>Rover installs ${engine === "coolify" ? "Coolify" : engine === "lite" ? "nginx" : "the right hosting engine"} on it${reach === "tunnel" ? " and connects it to Cloudflare with a tunnel (cloudflared " + "from Cloudflare's official release, checksum checked)" : ""}. Takes 1-10 minutes.</p>`, "Set up")) return;
    const x = await api("POST", "/api/home/setup", { engine, reach, remember_key: $("#hmKey").checked, auto_updates: $("#hmUpd").checked });
    await jobPanel(x.job, $("#hmJob"), { title: "Setting up", doneTitle: "Home server ready" });
    drawServerChip();
  });
}, "hosting");

// ---------- This PC ----------
route(/^\/thispc$/, async () => {
  const v = view(); v.className = "page wide";
  v.innerHTML = `<div class="muted"><span class="spin"></span> Loading&hellip;</div>`;
  let st; try { st = await api("GET", "/api/thispc"); } catch (e) { v.innerHTML = `<div class="alert bad">${esc(e.message)}</div>`; return; }
  const c = st.config, pw = st.power || {}, net = st.network || {};
  const act = net.active;
  v.innerHTML = `<div class="head"><a class="btn ghost sm" href="#/hosting">&larr; Hosting accounts</a><h1>&#128187; Host on this PC</h1><span class="grow"></span>${c.enabled ? (st.running ? `<span class="badge ok">&#9679; Serving</span>` : `<span class="badge bad">Not running</span>`) : `<span class="badge">Off</span>`}</div>
    ${st.error ? `<div class="alert bad">${esc(st.error)}</div>` : ""}
    <div class="ms7grid">
      <div class="card"><h2>Settings</h2>
        <label class="chk big"><input type="checkbox" id="pcOn" ${c.enabled ? "checked" : ""}> <b>Serve websites from this PC</b></label>
        <div class="field"><label for="pcPort">Port of the built-in web server</label><input id="pcPort" value="${c.port}" inputmode="numeric" style="max-width:120px"></div>
        <label class="chk"><input type="checkbox" id="pcLan" ${c.lan ? "checked" : ""}> Also answer phones and other computers on my home network</label>
        <label class="chk"><input type="checkbox" id="pcAuto" ${c.start_with_windows ? "checked" : ""}> Start with Windows (runs in the background)</label>
        <label class="chk"><input type="checkbox" id="pcAwake" ${c.prevent_sleep ? "checked" : ""}> Keep this PC awake while it hosts sites</label>
        <div class="row" style="margin-top:10px"><button class="primary" id="pcSave">Save</button></div></div>
      <div class="card"><h2>Reach from the internet</h2>${st.tunnel ? `<p class="okc">&#10004; Connected to Cloudflare with a tunnel.</p>${Object.values(st.tunnels || {}).map(t => `<p class="small">${t.running ? "&#9679; running" : "stopped"} since ${esc(ago(t.since))}${t.error ? ` &middot; <span class="warnc">${esc(t.error)}</span>` : ""}</p>`).join("")}`
        : `<p class="small">Rover downloads <b>cloudflared ${esc(st.cloudflared_version)}</b> from Cloudflare's official release (checked against a fixed checksum) and opens a tunnel. No router settings, works on Wi-Fi and on phone mobile data.</p>${st.cloudflare ? "" : `<div class="alert info small">Connect Cloudflare first (free): Domains &gt; Cloudflare.</div>`}`}
        <div class="row"><button id="pcTun" ${c.enabled && st.cloudflare ? "" : "disabled"}>${st.tunnel ? "Reconnect" : "Connect to the internet"}</button></div><div id="pcJob"></div></div>
    </div>
    <div class="ms7grid">
      <div class="card"><h2>Internet connection</h2><p>${net.online ? `<span class="badge ok">&#9679; Online</span>` : `<span class="badge bad">&#9679; Offline</span>`} ${act ? `via <b>${esc(({ wifi: "Wi-Fi", ethernet: "cable", "usb-tether": "phone over USB", mobile: "mobile data" })[act.kind] || act.name)}</b>${act.ssid ? " (" + esc(act.ssid) + ")" : ""}` : ""}</p>
        <ul class="small adapters">${(net.adapters || []).filter(a => a.up).map(a => { const on = a === act || (act && a.name === act.name); return `<li>${on ? "&#9654;" : "&#9675;"} ${esc(a.name)} <span class="muted">&middot; ${esc(netKindName(a.kind))}${a.ssid ? " " + esc(a.ssid) : ""}${on ? " &middot; in use" : a.gateway ? " &middot; ready as backup" : " &middot; no internet"}</span></li>`; }).join("") || `<li class="muted">No connections found.</li>`}</ul>
        <p class="hint">Plan: home Wi-Fi first, your phone's mobile data as backup (USB tethering or hotspot). When Windows switches, the Cloudflare tunnel reconnects by itself within seconds and Rover tells you. Watch your data plan.</p>
        ${(net.events || []).length ? `<h4>Recent changes</h4><ul class="small evlist">${net.events.slice(0, 6).map(e => `<li><span class="muted">${esc(ago(e.time))}</span> ${esc(e.text)}</li>`).join("")}</ul>` : ""}</div>
      <div class="card"><h2>Power</h2>${pw.known ? `<p>${pw.on_ac ? `<span class="badge ok">&#128268; Plugged in</span>` : `<span class="badge warn">&#128267; On battery${pw.percent != null ? " " + pw.percent + "%" : ""}</span>`} ${st.keep_awake ? `<span class="badge info">Kept awake</span>` : ""}</p>` : `<p class="muted small">Power information is shown on Windows.</p>`}
        <ul class="small"><li>Laptops: set <b>When I close the lid</b> to <b>Do nothing</b> (Control Panel &gt; Power Options &gt; Choose what closing the lid does), or keep the lid open.</li><li>Windows Update may restart the PC at night: set <b>Active hours</b> or pause updates while you rely on it.</li><li>A small UPS keeps the PC and router running during short power cuts.</li></ul></div>
    </div>
    <div class="card"><h2>Sites on this PC</h2>${(st.sites || []).length ? `<ul class="deploys">${st.sites.map(x => `<li><span class="grow"><b><a href="#/project/${esc(x.slug)}/hosting">${esc(x.name)}</a></b><div class="small muted">${x.domain ? esc(x.domain) + " &middot; " : ""}version ${esc(x.release || "none yet")}</div></span><a class="btn sm" target="_blank" rel="noopener" href="${esc(x.url)}">Open locally</a></li>`).join("")}</ul>` : `<p class="small muted">None yet. Open a project &gt; <b>Hosting</b> &gt; <b>Add a backup server</b> and choose <b>This PC</b>.</p>`}</div>
    <div class="card"><h2>Good to know</h2><ul class="tradeoffs">${(st.tradeoffs || []).map(t => `<li>${esc(t)}</li>`).join("")}</ul></div>`;
  $("#pcSave").onclick = (e) => busy(e.target, async () => {
    const body = { enabled: $("#pcOn").checked, port: +$("#pcPort").value || 8080, lan: $("#pcLan").checked, start_with_windows: $("#pcAuto").checked, prevent_sleep: $("#pcAwake").checked };
    if (!await confirmBox("Save hosting settings?", `<p>${body.enabled ? `Rover ${c.enabled ? "keeps serving" : "starts serving"} sites on port <b>${body.port}</b>${body.lan ? " (also to your home network)" : ""}.` : "Rover stops serving sites from this PC."}${body.start_with_windows ? " It starts with Windows." : ""}${body.prevent_sleep ? " The PC is kept awake." : ""}</p>`, "Save")) return;
    await api("POST", "/api/thispc", body); toast("Saved", "success"); rerender();
  });
  $("#pcTun").onclick = (e) => busy(e.target, async () => {
    if (!await confirmBox("Connect this PC to the internet?", `<p>Rover downloads cloudflared (about 55 MB, checksum checked), creates a Cloudflare tunnel named after this PC and runs it while Rover runs.</p>`, "Connect")) return;
    const x = await api("POST", "/api/thispc/connect"); await jobPanel(x.job, $("#pcJob"), { title: "Connecting", doneTitle: "Connected" }); later(rerender, 1200);
  });
}, "hosting");

// ---------- palette, Jiv screens ----------
palCommands.push(["Host on this PC", "#/thispc", "this pc laptop computer home host local server windows"], ["Add a home server (Raspberry Pi, old PC)", "#/hosting/home", "home server raspberry pi own server lan tunnel"]);
const _curScreen06 = curScreen;
curScreen = function () { const hsh = location.hash || ""; if (hsh.startsWith("#/thispc")) return "thispc"; if (hsh.startsWith("#/hosting/home")) return "home-server"; return _curScreen06(); };

// Cloudflare card: tunnels need one more permission.
const _cfCard06 = cloudflareCard;
cloudflareCard = function (el) {
  _cfCard06(el);
  el.insertAdjacentHTML("beforeend", `<p class="hint">&#127968; For backup servers, home servers and This PC, give the token one more permission: <b>Account &gt; Cloudflare Tunnel &gt; Edit</b>. Everything works on Cloudflare's free plan.</p>`);
};
