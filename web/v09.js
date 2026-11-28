// Rover 0.9: Integration Centre, first-party analytics, server monitoring,
// optional cloud sync and Google Sign-In.
"use strict";

(function addV09Nav() {
  const host = $('#nav a[data-route="hosting"]') || $('#nav a[data-route="domains"]');
  if (!host || $("#nav a[data-route='integrations']")) return;
  const integ = h(`<a href="#/integrations" data-route="integrations"><span>Integrations</span></a>`);
  const anal = h(`<a href="#/analytics" data-route="analytics"><span>Analytics</span></a>`);
  const mon = h(`<a href="#/monitor" data-route="monitor"><span>Monitoring</span></a>`);
  const parent = host.parentNode;
  parent.insertBefore(integ, host.nextSibling);
  parent.insertBefore(anal, integ.nextSibling);
  parent.insertBefore(mon, anal.nextSibling);
})();

function capPills(caps) {
  if (!caps) return "";
  return Object.keys(caps).sort().map(k => {
    const s = caps[k];
    const cls = s === "supported" ? "ok" : s === "partial" ? "warn" : "muted";
    return `<span class="badge ${cls}" title="${esc(s)}">${esc(k)}</span>`;
  }).join(" ");
}

function integStatus(s) {
  const m = { connected: ["ok", "Connected"], disconnected: ["muted", "Disconnected"], failed: ["bad", "Failed"], unsupported: ["warn", "Unsupported"], error: ["bad", "Failed"], ok: ["ok", "Connected"] };
  const [cls, lab] = m[s] || ["muted", s || "Disconnected"];
  return `<span class="badge ${cls}">${esc(lab)}</span>`;
}

route(/^\/integrations$/, async () => {
  
  let catalog = [], accounts = [];
  try {
    [catalog, accounts] = await Promise.all([api("GET", "/api/integrations/catalog"), api("GET", "/api/integrations")]);
  } catch (e) { return err(e); }
  const v = $("#view");
  v.innerHTML = `<div class="head"><h1>Integration Centre</h1></div>
    <p class="muted">Connect hosting, DNS, databases, analytics and cloud accounts. Credentials are encrypted on this PC and never shown back. No provider is required. Where an API cannot do something, Rover marks it <b>unsupported</b> instead of pretending.</p>
    <div class="card"><h2>Connected</h2>
      ${accounts.length ? `<div class="list">${accounts.map(a => `
        <div class="frow" data-id="${esc(a.id)}" data-src="${esc(a.source || "")}">
          <div class="grow"><div class="fname">${esc(a.nickname || a.provider_name || a.provider)} ${integStatus(a.status)}</div>
            <div class="small muted">${esc(a.provider_name || a.provider)}${a.who ? " · " + esc(a.who) : ""}${a.plan ? " · " + esc(a.plan) : ""}${a.health ? " · health " + esc(a.health) : ""}${a.source && a.source !== "integration" ? " · from " + esc(a.source) : ""}</div>
            <div class="small">${capPills(a.capabilities)}</div>
            ${a.error ? `<div class="alert bad">${esc(a.error)}</div>` : ""}
          </div>
          <div class="row">
            ${a.source === "integration" ? `<button class="sm" data-act="test">Test</button><button class="sm" data-act="edit">Edit</button><button class="sm danger" data-act="off">Disconnect</button>` : `<span class="muted small">Managed in ${esc(a.source === "hosting" ? "Hosting accounts" : a.source)}</span>`}
          </div>
        </div>`).join("")}</div>` : `<p class="muted">None yet. Connect a provider below.</p>`}
    </div>
    <div class="card"><h2>Providers</h2>
      <div class="list">${catalog.map(p => `
        <div class="frow" data-prov="${esc(p.id)}">
          <div class="grow"><div class="fname">${esc(p.name)}</div>
            <div class="small">${capPills(p.capabilities)}</div>
            ${p.notes ? `<div class="small muted">${esc(p.notes)}</div>` : ""}
            ${p.paid_warning ? `<div class="alert warn">${esc(p.paid_warning)}</div>` : ""}
            ${p.scopes && p.scopes.length ? `<details class="small"><summary>Least-privilege scopes</summary><ul>${p.scopes.map(s => `<li>${esc(s)}</li>`).join("")}</ul></details>` : ""}
          </div>
          <div class="row">
            ${p.existing_engine && (p.id === "nginx" || p.id === "local" || p.id === "rover-analytics") ? `<span class="muted small">Built in</span>` : `<button class="sm primary" data-act="connect">Connect</button>`}
            ${p.docs ? `<a class="btn sm ghost" href="${esc(p.docs)}" target="_blank" rel="noopener">Docs</a>` : ""}
          </div>
        </div>`).join("")}</div>
    </div>`;
  v.onclick = ev => {
    const row = ev.target.closest(".frow");
    if (!row) return;
    const act = ev.target.dataset.act;
    if (act === "connect") connectInteg(catalog.find(p => p.id === row.dataset.prov));
    if (act === "test") busy(ev.target, async () => { await api("POST", `/api/integrations/${row.dataset.id}/test`); toast("Connection checked", "success"); rerender(); });
    if (act === "off") busy(ev.target, async () => {
      if (!await confirmBox("Disconnect this account?", "<p>Projects assigned to it will stop using it. Credentials are removed from this PC.</p>", "Disconnect", true)) return;
      await api("DELETE", `/api/integrations/${row.dataset.id}`); toast("Disconnected", "success"); rerender();
    });
    if (act === "edit") connectInteg(catalog.find(p => p.id === accounts.find(a => a.id === row.dataset.id)?.provider), row.dataset.id, accounts.find(a => a.id === row.dataset.id));
  };
});

function connectInteg(p, editId, existing) {
  if (!p) return;
  const fields = (p.fields || []).map(f => {
    const id = "f_" + f.key;
    if (f.multiline) return `<div class="field"><label for="${id}">${esc(f.label)}${f.optional ? " <span class='muted'>(optional)</span>" : ""}</label><textarea id="${id}" rows="6" placeholder="${esc(f.placeholder || "")}"></textarea>${f.hint ? `<div class="small muted">${esc(f.hint)}</div>` : ""}</div>`;
    if (f.options && f.options.length) return `<div class="field"><label for="${id}">${esc(f.label)}</label><select id="${id}">${f.options.map(o => `<option>${esc(o)}</option>`).join("")}</select></div>`;
    return `<div class="field"><label for="${id}">${esc(f.label)}${f.optional ? " <span class='muted'>(optional)</span>" : ""}</label><input id="${id}" ${f.secret ? "type='password'" : "type='text'"} placeholder="${esc(f.placeholder || "")}" autocomplete="off">${f.hint ? `<div class="small muted">${esc(f.hint)}</div>` : ""}</div>`;
  }).join("");
  modal(`<h2>${editId ? "Edit" : "Connect"} ${esc(p.name)}</h2>
    ${p.notes ? `<p class="muted">${esc(p.notes)}</p>` : ""}
    <div class="field"><label for="f_nick">Nickname</label><input id="f_nick" value="${esc(existing && existing.nickname || p.name)}"></div>
    ${fields || "<p class='muted'>No credentials needed.</p>"}
    <div class="row end"><button data-close>Cancel</button><button class="primary" id="f_go">${editId ? "Save" : "Connect and test"}</button></div>`, (m, close) => {
    $("#f_go", m).onclick = () => busy($("#f_go", m), async () => {
      const cred = {};
      (p.fields || []).forEach(f => { const el = m.querySelector("#f_" + f.key); if (el) cred[f.key] = el.value; });
      if (editId) await api("POST", "/api/integrations/" + editId, { nickname: $("#f_nick", m).value, cred });
      else await api("POST", "/api/integrations", { provider: p.id, nickname: $("#f_nick", m).value, cred });
      toast(editId ? "Saved" : "Connected", "success");
      close(true); rerender();
    });
  });
}

route(/^\/analytics(?:\/([a-z0-9-]+))?$/, async (slug) => {
  
  let sites = [];
  try { const st = await api("GET", "/api/sites"); sites = st.sites || st || []; } catch (e) {}
  if (!Array.isArray(sites)) sites = [];
  slug = slug || (sites[0] && (sites[0].slug || sites[0].Slug));
  const v = $("#view");
  if (!slug) {
    v.innerHTML = `<div class="head"><h1>Analytics</h1></div><p class="muted">Add a project first. Rover Analytics is first-party and optional: a tiny beacon or the site's nginx log. No cookies, no Google account required.</p>`;
    return;
  }
  let sum = {};
  try { sum = await api("GET", `/api/analytics/${encodeURIComponent(slug)}?days=28`); } catch (e) { return err(e); }
  const chips = sites.map(s => `<a class="chip${(s.slug||s.Slug)===slug?" on":""}" href="#/analytics/${esc(s.slug||s.Slug)}">${esc(s.name||s.Name||s.slug)}</a>`).join("");
  const list = (rows) => (rows && rows.length ? rows.map(r => `<div class="frow"><span class="grow">${esc(r.name)}</span><b>${r.count}</b></div>`).join("") : `<p class="muted small">Nothing yet.</p>`);
  v.innerHTML = `<div class="head"><h1>Analytics</h1></div>
    <div class="chips">${chips}</div>
    <p class="muted small">${esc(sum.geo_note || "Country comes from the Cloudflare CF-IPCountry header, or from a GeoLite-style database you provide. Rover does not bundle one.")}</p>
    <div class="cards four">
      <div class="card"><div class="muted small">Pageviews</div><div class="stat">${sum.pageviews||0}</div></div>
      <div class="card"><div class="muted small">Estimated uniques</div><div class="stat">${sum.uniques||0}</div></div>
      <div class="card"><div class="muted small">Bandwidth</div><div class="stat">${fmtBytes(sum.bandwidth||0)}</div></div>
      <div class="card"><div class="muted small">Errors (4xx/5xx)</div><div class="stat">${sum.errors||0}</div></div>
    </div>
    <div class="grid2">
      <div class="card"><h2>Top pages</h2>${list(sum.pages)}</div>
      <div class="card"><h2>Countries</h2>${list(sum.countries)}</div>
      <div class="card"><h2>Devices</h2>${list(sum.devices)}</div>
      <div class="card"><h2>Browsers</h2>${list(sum.browsers)}</div>
      <div class="card"><h2>Referrers</h2>${list(sum.referrers)}</div>
      <div class="card"><h2>Engagement</h2><p>Average time to interactive (beacon): <b>${sum.engagement_ms_avg||0} ms</b>. Bots filtered: ${sum.bots_filtered||0}.</p></div>
    </div>
    <div id="gReports"><p class="small muted">Loading Google reports…</p></div>
    <p class="small muted">Optional Google Analytics, Search Console and AdSense reports use read-only official APIs. Connect the accounts in the Integration Centre (and assign them to the project if you have more than one). Add this snippet to a site (or let Rover's nginx include it): <code>&lt;script src="/__rover/r.js" defer&gt;&lt;/script&gt;</code></p>`;
  try {
    const g = await api("GET", `/api/analytics/${encodeURIComponent(slug)}/google?days=28`);
    const box = $("#gReports");
    if (box) box.innerHTML = renderGoogleReports(g);
  } catch (e) {
    const box = $("#gReports");
    if (box) box.innerHTML = `<p class="muted small">Google reports could not be loaded: ${esc(e.message || e)}</p>`;
  }
});

function renderGoogleReports(g) {
  if (!g) return "";
  const table = (title, block) => {
    if (!block) return "";
    if (block.error) return `<div class="card"><h2>${esc(title)}</h2><p class="muted small">${esc(block.error)}</p></div>`;
    const t = block.table || {};
    const headers = t.headers || [];
    const rows = t.rows || [];
    if (!rows.length) return `<div class="card"><h2>${esc(title)}</h2><p class="muted small">No rows in this period${block.property || block.site || block.account ? " (" + esc(block.property || block.site || block.account) + ")" : ""}.</p></div>`;
    const head = `<tr>${headers.map(h => `<th>${esc(h)}</th>`).join("")}</tr>`;
    const body = rows.slice(0, 15).map(r => `<tr>${(r || []).map(c => `<td>${esc(c)}</td>`).join("")}</tr>`).join("");
    return `<div class="card"><h2>${esc(title)}</h2><div class="table wrap"><table>${head}${body}</table></div></div>`;
  };
  const parts = [
    table("Google Analytics", g.ga4),
    table("Search Console", g.search_console),
    table("AdSense", g.adsense)
  ].filter(Boolean).join("");
  if (!parts) return "";
  return `<div class="grid2" style="margin-top:16px">${parts}</div>`;
}

function fmtBytes(n) {
  if (n < 1024) return n + " B";
  if (n < 1024*1024) return (n/1024).toFixed(1) + " KB";
  if (n < 1024*1024*1024) return (n/1024/1024).toFixed(1) + " MB";
  return (n/1024/1024/1024).toFixed(1) + " GB";
}

route(/^\/monitor$/, async () => {
  
  let data = {};
  try { data = await api("GET", "/api/monitor"); } catch (e) { return err(e); }
  const a = data.alerts || {};
  const ms = data.metrics || [];
  const health = data.health || {};
  const pct = (u,t) => t ? Math.round(u*100/t) + "%" : "—";
  const v = $("#view");
  v.innerHTML = `<div class="head"><h1>Monitoring</h1><button class="sm" id="mCheck">Check now</button></div>
    <div class="card"><h2>Servers</h2>
      ${ms.length ? ms.map(m => `<div class="frow"><div class="grow"><div class="fname">${esc(m.label||m.server)}</div>
        <div class="small muted">CPU ${m.cpu_pct||0}% · RAM ${pct(m.ram_used,m.ram_total)} · Disk ${pct(m.disk_used,m.disk_total)} · Swap ${pct(m.swap_used,m.swap_total)} · up ${Math.round((m.uptime_sec||0)/3600)}h</div>
        ${m.error ? `<div class="alert bad">${esc(m.error)}</div>` : ""}</div></div>`).join("") : `<p class="muted">No samples yet. Connect a server and click Check now. Rover also samples about every 10 minutes while it runs.</p>`}
    </div>
    <div class="card"><h2>Site availability</h2>
      ${Object.keys(health).length ? Object.entries(health).map(([k,h]) => `<div class="frow"><span class="grow">${esc(k)}</span>${h && h.ok ? `<span class="badge ok">Up</span>` : `<span class="badge bad">Down</span>`}</div>`).join("") : `<p class="muted">Health checks run in the background.</p>`}
    </div>
    <div class="card"><h2>Alerts</h2>
      <div class="field"><label><input type="checkbox" id="aIn" ${a.in_app!==false?"checked":""}> In-app notices</label></div>
      <div class="field"><label><input type="checkbox" id="aWin" ${a.windows!==false?"checked":""}> Flash the Rover window</label></div>
      <div class="grid2">
        <div class="field"><label>CPU % threshold (0 = off)</label><input id="aCPU" type="number" value="${a.cpu||90}"></div>
        <div class="field"><label>RAM %</label><input id="aRAM" type="number" value="${a.ram||90}"></div>
        <div class="field"><label>Disk %</label><input id="aDisk" type="number" value="${a.disk||90}"></div>
        <div class="field"><label>SSL expiry warning (days)</label><input id="aSSL" type="number" value="${a.ssl_days||21}"></div>
      </div>
      <div class="field"><label>Optional webhook (https)</label><input id="aHook" value="${esc(a.webhook||"")}" placeholder="https://"></div>
      <h3>Email (SMTP)</h3>
      <p class="muted small">Sent through <em>your</em> mail server. The password is encrypted on this PC (DPAPI on Windows) and is never shown back. Leave the password blank to keep one that is already saved.</p>
      <div class="grid2">
        <div class="field"><label>To</label><input id="aMail" value="${esc(a.email||"")}" placeholder="you@example.com"></div>
        <div class="field"><label>From</label><input id="aFrom" value="${esc(a.smtp_from||"")}" placeholder="rover@example.com"></div>
        <div class="field"><label>SMTP host</label><input id="aHost" value="${esc(a.smtp_host||"")}" placeholder="smtp.example.com"></div>
        <div class="field"><label>Port</label><input id="aPort" type="number" value="${a.smtp_port||587}"></div>
        <div class="field"><label>Username</label><input id="aUser" value="${esc(a.smtp_user||"")}"></div>
        <div class="field"><label>Password ${a.smtp_pass_set?"(saved)":""}</label><input id="aPass" type="password" autocomplete="new-password" placeholder="${a.smtp_pass_set?"unchanged":""}"></div>
      </div>
      <div class="field"><label>TLS</label>
        <select id="aTLS"><option value="starttls"${(a.smtp_tls||"starttls")==="starttls"?" selected":""}>STARTTLS (default, port 587)</option><option value="tls"${a.smtp_tls==="tls"?" selected":""}>TLS from connect (port 465)</option><option value="none"${a.smtp_tls==="none"?" selected":""}>None (127.0.0.1 only)</option></select>
      </div>
      <button class="primary" id="aSave">Save alerts</button>
    </div>`;
  $("#mCheck").onclick = () => busy($("#mCheck"), async () => { await api("POST", "/api/monitor/check"); toast("Sample taken", "success"); rerender(); });
  $("#aSave").onclick = () => busy($("#aSave"), async () => {
    await api("POST", "/api/monitor/alerts", {
      in_app: $("#aIn").checked, windows: $("#aWin").checked,
      cpu: +$("#aCPU").value, ram: +$("#aRAM").value, disk: +$("#aDisk").value, ssl_days: +$("#aSSL").value,
      webhook: $("#aHook").value, email: $("#aMail").value,
      smtp_from: $("#aFrom").value, smtp_host: $("#aHost").value, smtp_port: +$("#aPort").value,
      smtp_user: $("#aUser").value, smtp_pass: $("#aPass").value, smtp_tls: $("#aTLS").value
    });
    toast("Alerts saved", "success");
  });
});
