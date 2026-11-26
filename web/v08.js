// Rover 0.8: Jiv guides ("Show me how"), plans ("Do it for me"),
// offline packs and graphics-card detection.
"use strict";

// ---------- guide runner ----------
const G8 = { g: null, i: 0, site: "", base: null, poll: null, pos: null, clickOff: null, extTimer: null, wait: 0, lastCheck: 0, done: false };
const g8hi = () => (jivPrefs().lang || "") === "hi";
const g8say = (s) => esc(g8hi() && s.say_hi ? s.say_hi : s.say || "");

async function startGuide(id, site) {
  let r;
  try { r = await api("GET", `/api/jiv/guides/${encodeURIComponent(id)}${site ? "?site=" + encodeURIComponent(site) : ""}`); } catch (e) { return err(e); }
  if (r.need_site) {
    if (!(r.sites || []).length) { toast("Add a project first - then Jiv can show you this.", "warn"); return startGuide("guide-add-project"); }
    return modal(`<h2>${esc(r.title)}</h2><p>Which project?</p><div class="chips">${r.sites.map(s => `<button class="chip" data-s="${esc(s.slug)}">${esc(s.name)}</button>`).join("")}</div><div class="row end"><button data-close>Cancel</button></div>`, (m, close) => {
      $$("[data-s]", m).forEach(b => b.onclick = () => { close(true); startGuide(id, b.dataset.s); });
    });
  }
  stopGuide(true);
  G8.g = r.guide; G8.i = 0; G8.site = r.guide.site || site || "";
  store("guide8", { id, site: G8.site, i: 0 });
  if ($("#jiv") && !$("#jiv").classList.contains("full")) jivOpen(false);
  if ($("#jiv") && $("#jiv").classList.contains("full")) go(r.guide.steps[0] && r.guide.steps[0].go || "#/dashboard");
  if (!G8.g.steps.length) { toast("Nothing left to do here - it's already set up.", "success"); return stopGuide(); }
  g8step();
}

function stopGuide(silent) {
  clearInterval(G8.poll); clearInterval(G8.pos); clearInterval(G8.extTimer);
  if (G8.clickOff) G8.clickOff();
  document.removeEventListener("visibilitychange", g8vis); window.removeEventListener("focus", g8vis);
  ["#g8spot", "#g8bub", "#g8ext"].forEach(s => { const e = $(s); if (e) e.remove(); });
  if (!silent) store("guide8", null);
  if (!silent) G8.g = null;
}

function g8bubble(html, target) {
  let b = $("#g8bub");
  if (!b) { b = h(`<div id="g8bub" class="g8bub" role="dialog" aria-live="polite"></div>`); document.body.appendChild(b); }
  b.innerHTML = html;
  b.dataset.free = target ? "" : "1";
  g8place(target);
  return b;
}

function g8place(target) {
  const b = $("#g8bub"), spot = $("#g8spot");
  if (!b) return;
  if (!target || !document.contains(target)) {
    if (spot) spot.remove();
    Object.assign(b.style, { left: "", top: "", right: "24px", bottom: "24px" });
    return;
  }
  const r = target.getBoundingClientRect(), pad = 6;
  let s = spot; if (!s) { s = h(`<div id="g8spot" class="g8spot" aria-hidden="true"></div>`); document.body.appendChild(s); }
  Object.assign(s.style, { left: (r.left - pad) + "px", top: (r.top - pad) + "px", width: (r.width + pad * 2) + "px", height: (r.height + pad * 2) + "px" });
  const bw = Math.min(360, innerWidth - 24), bh = b.offsetHeight || 140;
  let top = r.bottom + 14, left = Math.max(12, Math.min(innerWidth - bw - 12, r.left));
  if (top + bh > innerHeight - 12) top = Math.max(12, r.top - bh - 14);
  Object.assign(b.style, { left: left + "px", top: top + "px", right: "", bottom: "", width: bw + "px" });
}

function g8find(st) {
  for (const sel of String(st.sel || "").split(/\s*,\s*/)) {
    if (!sel) continue;
    let els = [];
    try { els = $$(sel); } catch (e) { continue; }
    els = els.filter(e => e.offsetParent !== null || e.getClientRects().length);
    if (st.text) els = els.filter(e => e.textContent.toLowerCase().includes(st.text.toLowerCase()));
    if (els.length) return els[0];
  }
  return null;
}

function g8routeOK(st) {
  const at = st.at || (st.go ? "^" + st.go.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/\\\?.*$/, "") + "$" : "");
  if (!at) return true;
  try { return new RegExp(at).test(location.hash); } catch (e) { return location.hash === st.go; }
}

async function g8check(spec) {
  try { return await api("POST", "/api/jiv/guides/check", { check: spec }); } catch (e) { return { ok: false, message: e.message }; }
}

function g8head() {
  const n = G8.g.steps.length;
  return `<div class="row nowrap g8top"><span class="jiv-av sm" aria-hidden="true">J</span><b class="grow">${esc(g8hi() && G8.g.title_hi ? G8.g.title_hi : G8.g.title)}</b><span class="small muted">Step ${G8.i + 1} of ${n}</span><button class="ghost sm g8x" aria-label="Exit walkthrough" title="Exit">&times;</button></div><div class="g8bar"><i style="width:${Math.round(G8.i / n * 100)}%"></i></div>`;
}

function g8wire(b) {
  const q = (s) => $(s, b);
  if (q(".g8x")) q(".g8x").onclick = () => { stopGuide(); toast("Walkthrough closed. Ask Jiv any time.", "info"); };
  if (q(".g8back")) q(".g8back").onclick = () => { if (G8.i > 0) { G8.i--; g8step(); } };
  if (q(".g8skip")) q(".g8skip").onclick = () => g8next();
  if (q(".g8next")) q(".g8next").onclick = () => g8next();
  if (q(".g8go")) q(".g8go").onclick = () => { const st = G8.g.steps[G8.i]; if (st.go) go(st.go); later(() => g8step(), 400); };
}

function g8next() {
  G8.i++;
  store("guide8", { id: G8.g.id, site: G8.site, i: G8.i });
  g8step();
}

function g8finish() {
  const g = G8.g;
  stopGuide();
  const b = g8bubble(`<div class="row nowrap g8top"><span class="jiv-av sm" aria-hidden="true">J</span><b class="grow">${esc(g.title)}: done</b><button class="ghost sm g8x" aria-label="Close">&times;</button></div><p>&#10004; Nice work. ${g.do ? "Next time you can also ask Jiv to <b>do it for you</b>." : ""}</p><div class="row end"><button class="primary sm g8x">Close</button></div>`, null);
  $$(".g8x", b).forEach(x => x.onclick = () => b.remove());
  later(() => { const x = $("#g8bub"); if (x) x.remove(); }, 9000);
}

async function g8step() {
  clearInterval(G8.poll); clearInterval(G8.pos); clearInterval(G8.extTimer);
  if (G8.clickOff) { G8.clickOff(); G8.clickOff = null; }
  const ext = $("#g8ext"); if (ext) ext.remove();
  if (!G8.g) return;
  const st = G8.g.steps[G8.i];
  if (!st) return g8finish();
  if (st.kind === "external") return g8external(st);
  if (st.kind === "info" || !st.sel) {
    const b = g8bubble(`${g8head()}<p>${g8say(st)}</p><div class="row"><button class="ghost sm g8back" ${G8.i ? "" : "disabled"}>Back</button><span class="grow"></span><button class="primary sm g8next">Next</button></div>`, null);
    g8wire(b);
    if (st.go && !g8routeOK(st)) go(st.go);
    return;
  }
  if (!g8routeOK(st) && st.go) go(st.go);
  // wait for the element (screens render asynchronously)
  const t0 = Date.now();
  let el = null;
  while (G8.g && G8.g.steps[G8.i] === st && Date.now() - t0 < 8000) {
    el = g8find(st);
    if (el) break;
    await new Promise(r => setTimeout(r, 250));
  }
  if (!G8.g || G8.g.steps[G8.i] !== st) return;
  if (!el) {
    const b = g8bubble(`${g8head()}<p>${g8say(st)}</p><div class="alert warn small">I can't find this on the current screen.${st.optional ? " It may not be needed for your setup." : ""}</div><div class="row"><button class="ghost sm g8back" ${G8.i ? "" : "disabled"}>Back</button><span class="grow"></span>${st.go ? `<button class="sm g8go">Open the right page</button>` : ""}<button class="sm g8skip">Skip</button></div>`, null);
    return g8wire(b);
  }
  el.scrollIntoView({ block: "center", behavior: "smooth" });
  const d = st.done || {};
  const waitTxt = d.click ? "Click it when you're ready." : d.filled ? "Type it in; I'll continue when it's filled." : d.route ? "I'll continue when the page opens." : d.exists ? "I'll continue when it appears." : d.check ? "I'll check by myself and continue." : "";
  const b = g8bubble(`${g8head()}<p>${g8say(st)}</p>${waitTxt ? `<div class="small muted g8wait"><span class="spin"></span> ${esc(waitTxt)}</div>` : ""}<div class="row"><button class="ghost sm g8back" ${G8.i ? "" : "disabled"}>Back</button><span class="grow"></span><button class="ghost sm g8skip">Skip</button>${Object.keys(d).length && !st.optional ? "" : `<button class="primary sm g8next">Next</button>`}</div>`, el);
  g8wire(b);
  later(() => g8place(el), 350);
  G8.pos = setInterval(() => { const cur = g8find(st) || el; el = cur; g8place(cur); }, 300);
  if (d.check && d.changed) { const c = await g8check(d.check); G8.base = c.value || ""; }
  let fired = false;
  const advance = () => { if (fired) return; fired = true; clearInterval(G8.poll); setTimeout(() => { if (G8.g && G8.g.steps[G8.i] === st) g8next(); }, 450); };
  if (d.click) {
    const fn = (e) => { const cur = g8find(st) || el; if (cur && (cur === e.target || cur.contains(e.target))) advance(); };
    document.addEventListener("click", fn, true);
    G8.clickOff = () => document.removeEventListener("click", fn, true);
  }
  let lastCheck = 0;
  G8.poll = setInterval(async () => {
    if (!G8.g || G8.g.steps[G8.i] !== st) return clearInterval(G8.poll);
    if (d.route) { try { if (new RegExp(d.route).test(location.hash)) return advance(); } catch (e) {} }
    if (d.exists) { const x = g8find({ sel: d.exists }); if (x) return advance(); }
    if (d.filled) { const x = $(d.filled); if (x && String(x.value || "").trim().length >= 3 && document.activeElement !== x) return advance(); }
    if (d.check && Date.now() - lastCheck > 4000) {
      lastCheck = Date.now();
      const c = await g8check(d.check);
      if (c.ok && (!d.changed || (c.value || "") !== G8.base)) return advance();
      const w = $("#g8bub .g8wait"); if (w && c.message) w.innerHTML = `<span class="spin"></span> ${esc(c.message)}`;
    }
  }, 400);
}

// external website steps: checklist panel with picture, copy buttons and
// automatic verification when the user comes back to Rover
function g8vis() { if (document.visibilityState === "visible") g8verify(true); }
async function g8verify(auto) {
  const st = G8.g && G8.g.steps[G8.i];
  if (!st || st.kind !== "external" || !st.verify) return;
  const box = $("#g8ext .g8v"); if (!box) return;
  box.innerHTML = `<span class="spin"></span> Checking…`;
  const c = await g8check(st.verify);
  if (!G8.g || G8.g.steps[G8.i] !== st) return;
  if (c.ok) { box.innerHTML = `<div class="alert ok small">&#10004; ${esc(c.message || "Verified")}</div>`; setTimeout(() => { if (G8.g && G8.g.steps[G8.i] === st) g8next(); }, 1600); }
  else box.innerHTML = `<div class="small muted">${auto ? "Not yet: " : ""}${esc(c.message || "Not done yet")}</div>`;
}
function g8external(st) {
  const b = $("#g8bub"); if (b) b.remove();
  const s = $("#g8spot"); if (s) s.remove();
  const p = h(`<aside id="g8ext" class="g8ext" role="dialog" aria-label="Steps on ${esc(st.site || "another website")}">
    ${g8head()}
    <div class="g8site"><span class="badge info">On ${esc(st.site || "another website")}</span>${st.url ? ` <a class="btn sm" href="${esc(st.url)}" target="_blank" rel="noopener">Open ${esc(st.site || "the website")} &#8599;</a>` : ""}</div>
    <p>${g8say(st)}</p>
    ${st.image ? `<img class="g8img" src="/static/guides/${esc(st.image)}" alt="What the ${esc(st.site || "")} page looks like">` : ""}
    <ol class="g8items">${(st.items || []).map(it => `<li><span>${esc(it.text)}</span>${it.copy ? ` <code>${esc(it.copy)}</code> <button class="sm copy" data-copy="${esc(it.copy)}">${esc(it.label || "Copy")}</button>` : ""}</li>`).join("")}</ol>
    <div class="g8v small muted">${st.verify ? "When you come back, Rover checks this automatically." : ""}</div>
    <div class="row"><button class="ghost sm g8back" ${G8.i ? "" : "disabled"}>Back</button><span class="grow"></span><button class="ghost sm g8skip">Skip</button>${st.verify ? `<button class="primary sm g8chk">I've done it - check</button>` : `<button class="primary sm g8next">Done, next</button>`}</div>
    <p class="hint">Rover never types your passwords on other websites. You sign in yourself.</p></aside>`);
  document.body.appendChild(p);
  g8wire(p);
  if ($(".g8chk", p)) $(".g8chk", p).onclick = () => g8verify(false);
  if (st.verify) {
    document.addEventListener("visibilitychange", g8vis); window.addEventListener("focus", g8vis);
    G8.extTimer = setInterval(() => { if (document.visibilityState === "visible") g8verify(true); }, 15000);
  }
}

// resume after a reload
later(() => { const s = store("guide8"); if (s && s.id) { api("GET", `/api/jiv/guides/${encodeURIComponent(s.id)}${s.site ? "?site=" + encodeURIComponent(s.site) : ""}`).then(r => { if (r.guide) { G8.g = r.guide; G8.site = s.site; G8.i = Math.min(s.i || 0, r.guide.steps.length); g8step(); } }).catch(() => store("guide8", null)); } }, 900);
document.addEventListener("keydown", (e) => { if (e.key === "Escape" && G8.g && !document.querySelector(".modal")) { stopGuide(); } });

// ---------- chat: guide / plan events ----------
window.jivEvent = function (msg, ev) {
  if (ev.t === "guide") { msg.guide = ev.guide; if (ev.guide.auto) setTimeout(() => startGuide(ev.guide.id, ev.guide.site || curSite()), 600); }
  if (ev.t === "plan") { msg.plan = ev.plan; setTimeout(() => plan8Poll(ev.plan.id), 300); }
  if (ev.t === "plansite") msg.plansite = { q: ev.q, options: ev.options };
};

const _msgEl08 = msgEl;
msgEl = function (m, i) {
  const el = _msgEl08(m, i);
  const bub = el.firstElementChild;
  const guides = [];
  if (m.guide) guides.push(m.guide);
  (m.sources || []).forEach(a => (a.guides || []).forEach(g => { if (!guides.some(x => x.id === g.id)) guides.push(g); }));
  if (guides.length && bub) {
    bub.insertAdjacentHTML("beforeend", `<div class="row g8btns">${guides.slice(0, 2).map(g => `<button class="sm primary" data-guide="${esc(g.id)}">&#9758; Show me how${guides.length > 1 ? ": " + esc(g.title) : ""}</button>`).join("")}${m.guide && m.guide.do ? "" : ""}</div>`);
  }
  if (m.plansite && bub) bub.insertAdjacentHTML("beforeend", `<div class="chips">${m.plansite.options.map(o => `<button class="chip" data-ps="${esc(o.slug)}">${esc(o.name)}</button>`).join("")}</div>`);
  if (m.plan) el.appendChild(planEl(m.plan));
  el.addEventListener("click", (e) => {
    const g = e.target.closest("[data-guide]"); if (g) { e.stopPropagation(); startGuide(g.dataset.guide, curSite()); return; }
    const ps = e.target.closest("[data-ps]"); if (ps) { e.stopPropagation(); const q = m.plansite.q; m.plansite = null; store("chat", chat); g8askFor(q, ps.dataset.ps); }
  });
  return el;
};
function g8askFor(q, site) { // re-ask with a project chosen
  const old = curSite;
  curSite = () => site;
  sendChat(q).finally(() => { curSite = old; });
}

// ---------- plan card ----------
const P8 = {};
const stIcon = { pending: "&#9675;", running: '<span class="spin"></span>', needs_confirm: "&#9888;", waiting: "&#8987;", done: "&#10004;", failed: "&#10006;", skipped: "&#8631;", confirmed: "&#9675;" };
function planEl(p) {
  const running = p.state === "running" || p.state === "waiting";
  const el = h(`<div class="plan8 card flat" data-id="${esc(p.id)}" role="group" aria-label="Jiv's plan">
    <div class="row nowrap"><span class="aic" aria-hidden="true">&#9776;</span><div class="grow"><b>${p.auto ? "Checking" : "Plan"}: ${esc(p.goal)}</b><div class="small muted">${p.steps.length} steps &middot; ${p.source === "model" ? "planned by the offline brain, checked by Rover" : "built-in plan"}${p.state === "done" ? " &middot; finished" : p.state === "cancelled" ? " &middot; cancelled" : p.state === "expired" ? " &middot; expired" : ""}</div></div></div>
    <ol class="p8steps">${p.steps.map(s => `<li class="p8 st-${esc(s.state)}" data-n="${s.n}"><span class="p8ic" aria-label="${esc(s.state)}">${stIcon[s.state] || "&#9675;"}</span><div class="grow"><div>${s.risky ? `<span class="badge warn" title="Asks you again before it runs">&#9888; confirm</span> ` : ""}${s.read_only ? `<span class="badge info">read-only</span> ` : ""}${s.added ? `<span class="badge">added by Jiv</span> ` : ""}<b>${esc(s.label)}</b></div>
      ${s.detail && s.state !== "done" ? `<div class="small muted">${esc(s.detail)}</div>` : ""}
      ${s.result ? `<div class="small okc">${esc(s.result)}</div>` : ""}
      ${s.state === "running" && (s.log || []).length ? `<div class="small muted p8log">${esc(s.log[s.log.length - 1])}</div>` : ""}
      ${s.state === "waiting" && (s.log || []).length ? `<div class="small muted p8log">Latest check: ${esc(s.log[s.log.length - 1])}</div>` : ""}
      ${s.state === "waiting" && s.guide ? `<div class="row"><button class="sm" data-guide="${esc(s.guide)}">&#9758; Show me how</button><button class="sm ghost" data-p8="skip">Skip this step</button></div>` : ""}
      ${s.state === "needs_confirm" ? `<div class="row"><button class="sm primary" data-p8="confirm" data-n="${s.n}" data-tok="${esc(s.token)}">Yes, run this step</button><button class="sm ghost" data-p8="skip">Skip</button><button class="sm ghost" data-p8="cancel">Stop the plan</button></div>` : ""}
      ${s.state === "failed" ? `<div class="alert bad small"><b>${esc((s.fix && s.fix.title) || "This step failed")}</b><div>${esc(s.error || "")}</div>${s.fix && s.fix.text && s.fix.text !== s.error ? `<div class="p8fix">Fix: ${esc(s.fix.text)}</div>` : ""}<div class="row">${s.fix && s.fix.card ? `<button class="sm" data-p8card="${s.n}">${esc(s.fix.card.label)}</button>` : ""}${s.fix && s.fix.guide ? `<button class="sm" data-guide="${esc(s.fix.guide)}">&#9758; Show me how</button>` : ""}${s.fix && s.fix.kb ? `<button class="sm ghost" data-art="${esc(s.fix.kb)}">Learn more</button>` : ""}<span class="grow"></span><button class="sm primary" data-p8="retry">Retry</button><button class="sm ghost" data-p8="skip">Skip</button></div></div>` : ""}
      <div class="p8diag"></div></div></li>`).join("")}</ol>
    ${p.state === "ready" ? `<div class="small muted">Nothing changes until you press Run. Steps marked &#9888; ask again before they run.</div><div class="row"><button class="primary sm" data-p8="run">Run the plan</button><button class="ghost sm" data-p8="cancel">Not now</button></div>` : ""}
    ${running && !p.steps.some(s => s.state === "needs_confirm") ? `<div class="row"><span class="small muted grow">${p.state === "waiting" ? "Waiting - Jiv keeps checking by itself." : "Working…"}</span><button class="ghost sm" data-p8="cancel">Stop</button></div>` : ""}
    ${p.state === "done" ? `<div class="alert ok small">&#10004; All done.</div>` : ""}
  </div>`);
  p.steps.forEach(s => { if (s.diagnosis) { const slot = $(`li[data-n="${s.n}"] .p8diag`, el); if (slot) slot.appendChild(diagEl(s.diagnosis, -1)); } });
  el.addEventListener("click", async (e) => {
    const b = e.target.closest("[data-p8]");
    const cb = e.target.closest("[data-p8card]");
    if (cb) {
      e.stopPropagation();
      const s = p.steps.find(x => x.n === +cb.dataset.p8card), card = s.fix.card;
      if (!card.safe && !await confirmBox(card.label, esc(card.detail || "This changes your server or settings."), "Yes, do it")) return;
      busy(cb, async () => { try { const r = await api("POST", `/api/jiv/actions/${card.id}/run`, { confirmed: true }); if (r.job) await jobPanel(r.job, $(`li[data-n="${s.n}"] .p8diag`, el), { title: card.label, doneTitle: "Done" }); toast(r.message || "Done", "success"); } catch (x) { err(x); } });
      return;
    }
    if (!b) return;
    e.stopPropagation();
    const act = b.dataset.p8;
    const body = { confirmed: true };
    let path = `/api/jiv/plans/${p.id}/${act}`;
    if (act === "confirm") { path = `/api/jiv/plans/${p.id}/steps/${b.dataset.n}/confirm`; body.token = b.dataset.tok; }
    if (act === "cancel") body.confirmed = false;
    busy(b, async () => {
      try { const r = await api("POST", path, body); plan8Set(r.plan); plan8Poll(p.id); }
      catch (x) { if (x.data && x.data.plan) plan8Set(x.data.plan); err(x); }
    });
  });
  return el;
}
function plan8Set(p) {
  if (!p) return;
  for (const m of chat) if (m.plan && m.plan.id === p.id) m.plan = p;
  store("chat", chat.slice(-100));
  const old = document.querySelector(`.plan8[data-id="${p.id}"]`);
  if (old) old.replaceWith(planEl(p));
}
async function plan8Poll(id) {
  if (P8[id]) return;
  P8[id] = true;
  try {
    for (let n = 0; n < 4000; n++) {
      let r;
      try { r = await api("GET", `/api/jiv/plans/${id}`); } catch (e) {
        if (e.status === 404 || /expired/.test(e.message)) { for (const m of chat) if (m.plan && m.plan.id === id && !["done", "cancelled"].includes(m.plan.state)) { m.plan.state = "expired"; plan8Set(m.plan); } }
        return;
      }
      plan8Set(r.plan);
      if (!["running", "waiting"].includes(r.plan.state) || r.plan.steps.some(s => s.state === "needs_confirm")) {
        if (r.plan.state === "done") { saveHistory(); loadSites && loadSites().catch(() => {}); }
        return;
      }
      await new Promise(res => setTimeout(res, r.plan.state === "waiting" ? 3000 : 1200));
    }
  } finally { delete P8[id]; }
}

// ---------- Walkthroughs page ----------
route(/^\/guides$/, async () => {
  const v = view();
  const r = await api("GET", "/api/jiv/guides");
  const groups = {};
  (r.guides || []).forEach(g => (groups[g.group || "More"] = groups[g.group || "More"] || []).push(g));
  v.innerHTML = `<div class="head"><span class="jiv-av lg" aria-hidden="true">J</span><div><h1>Walkthroughs</h1><div class="small muted">Jiv highlights each button and moves on when you've done the step. Works offline.</div></div></div>
    ${Object.entries(groups).map(([k, list]) => `<div class="card"><h2>${esc(k)}</h2><div class="g8list">${list.map(g => `<div class="g8item"><div class="grow"><b>${esc(g8hi() && g.title_hi ? g.title_hi : g.title)}</b><div class="small muted">${esc(g.intro || "")}</div><div class="small muted">${g.steps} steps${g.external ? ` &middot; ${g.external} on another website` : ""}</div></div><div class="row nowrap"><button class="sm primary" data-start="${esc(g.id)}">Start</button>${g.do ? `<button class="sm ghost" data-do="${esc(g.do)}" title="Let Jiv plan it">Do it for me</button>` : ""}</div></div>`).join("")}</div></div>`).join("")}
    <p class="hint">Ask Jiv "show me how to…" for any of these. With the offline brain, Jiv can also build a walkthrough for other questions.</p>`;
  $$("[data-start]", v).forEach(b => b.onclick = () => startGuide(b.dataset.start, curSite()));
  $$("[data-do]", v).forEach(b => b.onclick = () => { jivOpen(true); const t = b.dataset.do.replace("{site}", (sites[0] && sites[0].name) || "my site").replace("{domain}", "mydomain.com"); $("#cIn").value = t; $("#cIn").focus(); $("#cIn").setSelectionRange(0, t.length); });
}, "jiv");

// ---------- Settings > Jiv: graphics card + offline pack ----------
const _setJiv08 = setJiv;
setJiv = async function (el) {
  await _setJiv08(el);
  const b = await api("GET", "/api/jiv/brain");
  const hw = b.hardware || { gpus: [] }, adv = b.advice || {}, st = b.status || {}, pk = b.pack || {}, js = b.settings || {};
  const gb = (n) => n < 1073741824 ? Math.round(n / 1048576) + " MB" : (n / 1073741824).toFixed(1) + " GB";
  const cards = h(`<div>
   <div class="card" id="g8Gpu"><h2>Graphics card</h2>
    ${hw.gpus.length ? `<ul class="g8gpus">${hw.gpus.map(g => `<li><b>${esc(g.name)}</b> <span class="small muted">${g.vram ? gb(g.vram) + " · " : ""}${g.dedicated ? "dedicated" : "integrated / shared memory"}</span></li>`).join("")}</ul>` : `<p class="small muted">No graphics card found - Jiv uses the processor.</p>`}
    <div class="small">Vulkan driver: ${hw.vulkan ? `<span class="okc">&#10004; installed</span>` : `<span class="muted">not found (update the graphics driver to use the card)</span>`}${hw.simulated ? ` <span class="badge warn">simulated for this demo</span>` : ""}</div>
    <div class="alert info small"><b>Recommended: ${esc(adv.model === "qwen3-4b" ? "Standard" : "Light")} on the ${adv.gpu ? "graphics card" : "processor"}</b> (${esc(adv.speed || "")})<div>${esc(adv.why || "")}</div></div>
    ${b.installed ? `<div class="small">Now: ${esc(b.model.short)} on <b>${st.state === "ready" ? (st.gpu ? "the graphics card" : "the processor") : js.gpu ? "the graphics card (when it starts)" : "the processor"}</b>.</div>${st.note ? `<div class="alert warn small">${esc(st.note)}</div>` : ""}` : ""}
    <label class="switch"><input type="checkbox" id="g8GpuOn" ${js.gpu ? "checked" : ""} ${hw.vulkan ? "" : "disabled"}> Use the graphics card for the offline brain</label>
    <p class="hint">One Vulkan engine works for NVIDIA, AMD and Intel. llama.cpp puts as much of the model on the card as fits (keeping 512 MB free) and the rest in RAM. If the card fails, Jiv switches to the processor by itself.</p>
    <div class="row"><button class="ghost sm" id="g8Re">Detect again</button></div></div>
   <div class="card" id="g8Pack"><h2>Offline pack</h2>
    <p>Everything Jiv needs, without internet: <b>${pk.kb_articles || 0} help articles</b> and <b>${pk.guides || 0} walkthroughs</b> are built in. The brain can come from the <b>Rover Offline (full)</b> installer or an offline pack file on a USB stick.</p>
    <div class="small">Installed here: ${(pk.models || []).length ? (pk.models || []).map(esc).join(", ") : "no model"} &middot; engines: ${(pk.runtimes || []).length ? (pk.runtimes || []).map(x => x === "vulkan" ? "Vulkan (GPU)" : "CPU").join(", ") : "none"}</div>
    ${(pk.found || []).length ? `<div class="alert ok small">Found in the app's offline folder: ${pk.found.map(f => `<b>${esc(f.title || f.path)}</b> (${gb(f.size)}${f.parts > 1 ? ", " + f.parts + " parts" : ""})`).join(", ")}</div>` : ""}
    <div class="field"><label for="g8Path">Pack file, its folder or USB drive</label><input id="g8Path" placeholder="E:\\ or D:\\Downloads\\Rover-Offline-Light-b12360.dlpack"></div>
    <div class="row"><button class="primary" id="g8Imp">Import offline pack</button><button class="ghost" id="g8Exp" ${b.installed ? "" : "disabled"}>Export to USB…</button></div>
    <p class="hint">Split packs (.dlpack.001, .002…) must be in the same folder. Only files whose SHA-256 fingerprint is built into Rover are installed.</p><div id="g8Job"></div></div></div>`);
  el.insertBefore(cards, el.children[1] || null);
  $("#g8GpuOn").onchange = (e) => api("POST", "/api/jiv/settings", { gpu: e.target.checked }).then(() => { toast(e.target.checked ? "Jiv will use the graphics card" : "Jiv will use the processor", "success"); aiInfo = null; }).catch(err);
  $("#g8Re").onclick = (e) => busy(e.target, async () => { await api("GET", "/api/jiv/hardware?refresh=1"); rerender(); });
  $("#g8Imp").onclick = (e) => { const p = $("#g8Path").value.trim(); if (!p) { $("#g8Path").focus(); return toast("Type where the pack is (for example E:\\)", "warn"); } busy(e.target, async () => { const r = await api("POST", "/api/jiv/pack/import", { path: p }); const j = await jobPanel(r.job, $("#g8Job"), { title: "Importing the offline pack", doneTitle: "Offline pack installed", failTitle: "Import stopped" }); aiInfo = null; if (!j.error) { toast("Jiv's offline brain is ready", "success"); later(rerender, 800); } }); };
  $("#g8Exp").onclick = () => modal(`<h2>Export to USB</h2><p class="small muted">Writes the installed brain as an offline pack (parts under 2 GB) so another PC can import it without internet.</p><div class="field"><label for="g8Dir">Folder or drive</label><input id="g8Dir" placeholder="E:\\"></div><div id="g8EJ"></div><div class="row end"><button data-close>Close</button><button id="g8EGo" class="primary">Export</button></div>`, (m) => {
    $("#g8EGo", m).onclick = (e) => busy(e.target, async () => { const r = await api("POST", "/api/jiv/pack/export", { dir: $("#g8Dir", m).value }); await jobPanel(r.job, $("#g8EJ", m), { title: "Writing the pack", doneTitle: "Pack written" }); });
  });
};

// palette + guide links
palCommands.push(["Walkthroughs (show me how)", "#/guides", "guide tutorial tour show me how help steps"], ["Offline pack & graphics card", "#/settings/jiv", "usb offline pack gpu vulkan nvidia"]);
