// Discovery test: simulated phones that send exactly what the APK's native service
// sends (POST /api/presence, GET /api/ble-token, POST /api/sightings) and hold the
// app's WebSocket for their deck. Idle users (seeded profiles, stationary) are in
// place first; a new user arrives and we time until each sees the other.
// Each scenario runs in its own spot on the map with its own users, all at once.
//   API=http://localhost:8099 MAX_S=300 SEED=1 node loadtest/discovery.mjs
// Seed: loadtest/discovery-seed.sql (tok_1..tok_200, accounts only: each phone saves its card when it starts).
const API = process.env.API || "http://localhost:8099", WS = API.replace(/^http/, "ws") + "/ws";
const MAX = +(process.env.MAX_S || 300) * 1000, ONLY = process.env.ONLY;
let seed = +(process.env.SEED || 1);
const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
const gauss = () => Math.sqrt(-2 * Math.log(rnd() || 1e-9)) * Math.cos(2 * Math.PI * rnd());
const M = 1 / 111320, sleep = ms => new Promise(r => setTimeout(r, ms));
const OUT = { acc: [5, 12], err: 1 }, IN = { acc: [15, 45], err: 1.8 };   // indoors a fix claiming 20 m is often 40-60 m off

class Phone {
  constructor(name, tok, base, x, y, env, ble, fg) {
    Object.assign(this, { name, tok, base, x, y, env, ble, fg, deck: new Set(), offline: false, on: true, seen: [] });
  }
  async http(method, path, body) {
    if (this.offline) return null;   // no signal: the request never reaches the server
    try {
      const r = await fetch(API + path, { method, headers: { Authorization: "Bearer " + this.tok, "Content-Type": "application/json" }, body: body && JSON.stringify(body) });
      return r.ok && r.status === 200 ? r.json() : null;
    } catch { return null; }
  }
  fix() {   // claimed accuracy plus a true error that is often bigger (accuracy is 1 sigma)
    const [lo, hi] = this.env.acc, acc = Math.round(lo + rnd() * (hi - lo)), s = acc / 1.5 * this.env.err;
    const ex = this.x + gauss() * s, ey = this.y + gauss() * s;
    return this.http("POST", "/api/presence", { lat: this.base.lat + ey * M, lng: this.base.lng + ex * M / Math.cos(this.base.lat * Math.PI / 180), acc, age: 0 });
  }
  socket() {
    if (!this.on) return;
    const ws = this.ws = new WebSocket(WS, ["kinjo", this.tok]);
    ws.onmessage = e => { const m = JSON.parse(e.data); if (m.type === "nearby") { this.deck = new Set(m.people.map(p => p.name)); this.lastRole = m.people.find(p => p.role === "Edited role")?.role ?? this.lastRole; this.seen.push([Date.now(), [...this.deck]]); } };
    ws.onclose = e => { if (process.env.DEBUG) console.error(Date.now() % 1e6, this.name, "socket closed", e.code, e.reason); if (this.ws === ws && this.on && !this.offline) setTimeout(() => this.ws === ws && this.socket(), 1000); };
  }
  async start(all) {
    await this.http("PUT", "/api/profile", { name: this.name, role: "Tester", look: "Testing discovery", photo: "data:image/jpeg;base64,AAAA" });   // onboarding: the card is saved, then location
    this.socket();
    if (this.ble) this.token = (await this.http("GET", "/api/ble-token"))?.token;
    const fixMs = this.fg ? 10e3 : 30e3, bleMs = this.fg ? 5e3 : 10e3;   // Gps.FG_MS/BG_MS, Ble.REPORT_FG_MS/REPORT_MS
    this.fix();
    this.t1 = setInterval(() => this.fix(), fixMs);
    if (this.ble) this.t2 = setInterval(() => {
      if (!this.token) return;
      const seen = all.filter(o => o !== this && o.on && o.ble && o.token && o.base === this.base)
        .map(o => ({ o, d: Math.hypot(o.x - this.x, o.y - this.y) }))
        .map(({ o, d }) => ({ tok: o.token, rssi: Math.round(o.rssiAt?.(d) ?? (-55 - 25 * Math.log10(Math.max(d, 1)) + gauss() * 4)) }))
        .filter(s => s.rssi > -100);
      for (let i = 0; i < seen.length; i += 32) this.http("POST", "/api/sightings", { seen: seen.slice(i, i + 32) });   // Ble.report: 32 per request (the server's maxSightings)
    }, bleMs);
  }
  die() { this.on = false; clearInterval(this.t1); clearInterval(this.t2); const ws = this.ws; this.ws = null; try { ws.close(); } catch {} }   // battery dead / force-stop: no goodbye
  closeApp() { const ws = this.ws; this.ws = null; try { ws.close(); } catch {} this.deck = new Set(); }   // swiped away: the service keeps reporting
  reopen() { this.socket(); }
  send(m) { this.ws?.readyState === 1 && this.ws.send(JSON.stringify(m)); }
  stop() { this.on = false; clearInterval(this.t1); clearInterval(this.t2); try { this.ws.close(); } catch {} }
  goOffline(on) { this.offline = on; if (on) { try { this.ws.close(); } catch {} } else this.socket(); }
}

let uid = 0;
function crowd(k, env, ble, idle = 3) {   // scenario k: its own spot ~11 km from the others
  const base = { lat: 12.9716 + k * 0.1, lng: 77.5946 };
  const ph = Array.from({ length: idle }, (_, i) => new Phone("P" + (uid + 1), "tok_" + ++uid, base, 4 * i, 3, env, ble, false));
  const nu = new Phone("P" + (uid + 1), "tok_" + ++uid, base, 6, 6, env, ble, true);   // new user: app open, a few metres from the idle ones
  return { base, idle: ph, nu, all: [...ph, nu] };
}
const until = async (cond, ms) => { const t = Date.now(); while (Date.now() - t < ms) { if (cond()) return Date.now() - t; await sleep(250); } return null; };
const mutual = (a, b) => a.deck.has(b.name) && b.deck.has(a.name);
const s = ms => ms == null ? "never" : (ms / 1000).toFixed(1) + " s";

// Each returns {pass, ...}: discovery time is counted from the new user's first fix.
const SCENARIOS = {
  async outdoor_ble(k) {
    const c = crowd(k, OUT, true); c.idle.forEach(p => p.start(c.all)); await sleep(20e3);
    c.nu.start(c.all); const t = await until(() => mutual(c.nu, c.idle[0]), MAX);
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 20e3, discovery: s(t) };
  },
  async indoor_ble(k) {   // the reported bug: same room, GPS 15-45 m, Bluetooth on
    const c = crowd(k, IN, true); c.idle.forEach(p => p.start(c.all)); await sleep(20e3);
    c.nu.start(c.all); const t = await until(() => mutual(c.nu, c.idle[0]), MAX);
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 30e3, discovery: s(t) };
  },
  async indoor_gps_only(k) {   // Bluetooth off: by design only 30 m-accurate fixes can match (informational)
    const c = crowd(k, IN, false); c.idle.forEach(p => p.start(c.all)); await sleep(20e3);
    c.nu.start(c.all); const t = await until(() => mutual(c.nu, c.idle[0]), MAX);
    c.all.forEach(p => p.stop()); return { pass: true, discovery: s(t), note: "informational" };
  },
  async wall_80m(k) {   // next building: good fixes 80 m apart, radio still heard at -84 dBm: must never match
    const c = crowd(k, OUT, true, 1); c.nu.x = 80; c.idle[0].rssiAt = () => -84; c.nu.rssiAt = () => -84;
    c.idle[0].start(c.all); await sleep(5e3); c.nu.start(c.all);
    const t = await until(() => c.nu.deck.has(c.idle[0].name) || c.idle[0].deck.has(c.nu.name), 90e3);
    c.all.forEach(p => p.stop()); return { pass: t == null, shown_after: s(t) };
  },
  async leave_reenter(k) {   // walk out past 40 m (and out of radio range), then come back
    const c = crowd(k, OUT, true, 1), a = c.idle[0]; a.start(c.all); await sleep(5e3); c.nu.start(c.all);
    const t1 = await until(() => mutual(c.nu, a), 60e3);
    c.nu.x = 150; c.nu.rssiAt = () => -110;   // gone: GPS far, radio silent
    const t2 = await until(() => !a.deck.has(c.nu.name) && !c.nu.deck.has(a.name), 180e3);
    c.nu.x = 6; delete c.nu.rssiAt;
    const t3 = await until(() => mutual(c.nu, a), 60e3);
    c.all.forEach(p => p.stop()); return { pass: t1 != null && t2 != null && t3 != null, discovery: s(t1), removed_after: s(t2), back_after: s(t3) };
  },
  async network_drop(k) {   // signal lost 40 s (posts fail, socket drops), then back: stays visible, deck refills
    const c = crowd(k, IN, true, 1), a = c.idle[0]; a.start(c.all); await sleep(5e3); c.nu.start(c.all);
    const t1 = await until(() => mutual(c.nu, a), 60e3);
    c.nu.goOffline(true); await sleep(40e3);
    const stayed = a.deck.has(c.nu.name);
    c.nu.goOffline(false); const t2 = await until(() => c.nu.deck.has(a.name), 30e3);
    c.all.forEach(p => p.stop()); return { pass: t1 != null && stayed && t2 != null, discovery: s(t1), still_shown_while_offline: stayed, deck_back_after: s(t2) };
  },
  async idle_no_flicker(k) {   // 4 idle phones in the background, indoors, 3 min: once matched, nobody drops off
    const c = crowd(k, IN, true, 4); c.all.forEach(p => p.start(c.all));
    const t = await until(() => c.all.every(p => p.deck.size === c.all.length - 1), 90e3);
    const from = Date.now(); await sleep(180e3);
    let drops = 0;
    for (const p of c.all) { let prev = null; for (const [at, d] of p.seen) { if (at < from) { prev = d; continue; } if (prev) drops += prev.filter(n => !d.includes(n)).length; prev = d; } }
    c.all.forEach(p => p.stop()); return { pass: t != null && drops === 0, all_matched_after: s(t), drops_in_3_min: drops };
  },
};

// Closed devices and account actions: nobody may stay visible after they stop reporting,
// hide, log out or delete; in-flight or replayed data must not bring them back.
const pair = async (k, env = OUT, ble = true) => { const c = crowd(k, env, ble, 1), a = c.idle[0]; a.start(c.all); await sleep(5e3); c.nu.start(c.all); const t = await until(() => mutual(c.nu, a), 60e3); return { ...c, a, b: c.nu, t }; };
const gone = (viewer, p) => () => !viewer.deck.has(p.name);
const never = async (cond, ms) => (await until(cond, ms)) == null;   // true if cond stayed false the whole time
Object.assign(SCENARIOS, {
  async dead_phone_gps(k) {   // open-sky pair; one phone dies without a word: must leave within freshFor (120 s) + ticks
    const c = await pair(k); c.b.die();
    const t = await until(gone(c.a, c.b), 180e3), back = !(await never(() => c.a.deck.has(c.b.name), 120e3));
    c.all.forEach(p => p.stop()); return { pass: c.t != null && t != null && t <= 135e3 && !back, removed_after: s(t), came_back: back };
  },
  async dead_phone_ble(k) {   // indoors, matched over Bluetooth; one dies: must leave too
    const c = await pair(k, IN); c.b.die();
    const t = await until(gone(c.a, c.b), 180e3), back = !(await never(() => c.a.deck.has(c.b.name), 120e3));
    c.all.forEach(p => p.stop()); return { pass: c.t != null && t != null && t <= 135e3 && !back, removed_after: s(t), came_back: back };
  },
  async app_closed_service_on(k) {   // swiped from recents: the service still reports, so they stay visible (by design); reopening shows the deck at once
    const c = await pair(k); c.b.closeApp(); c.b.fg = false;
    const stayed = await never(gone(c.a, c.b) , 150e3);
    c.b.reopen(); const t = await until(() => c.b.deck.has(c.a.name), 10e3);
    c.all.forEach(p => p.stop()); return { pass: c.t != null && stayed && t != null, stayed_visible_150s: stayed, deck_on_reopen: s(t) };
  },
  async sleeping_phone_ble_only(k) {   // asleep: no GPS fixes, only advertising; the open phone hears it. Visible while heard, gone once it dies
    const c = crowd(k, IN, true, 1), a = c.idle[0], b = c.nu; a.start(c.all); await sleep(5e3);
    await b.start(c.all); clearInterval(b.t1); clearInterval(b.t2);   // b: token only, no fixes, no reports (a does the listening)
    const t1 = await until(() => a.deck.has(b.name), 60e3); b.die();
    const t2 = await until(gone(a, b), 180e3);
    c.all.forEach(p => p.stop()); return { pass: t1 != null && t2 != null && t2 <= 135e3, shown_after: s(t1), removed_after_death: s(t2) };
  },
  async closed_long_reopen(k) {   // phone off 3 min, reopened with no fix yet: no blank list, and the others don't see it until it reports
    const c = await pair(k); c.b.die(); await sleep(180e3);
    const shownWhileOff = c.a.deck.has(c.b.name); c.b.on = true; c.b.seen = []; c.b.socket(); await sleep(3e3);
    const emptyList = c.b.seen.some(([, d]) => d.length === 0);
    c.b.fix(); const t = await until(() => mutual(c.a, c.b), 30e3);
    c.all.forEach(p => p.stop()); return { pass: !shownWhileOff && !emptyList && t != null, shown_while_off: shownWhileOff, empty_list_on_reopen: emptyList, rematched_after_fix: s(t) };
  },
  async hide(k) {   // Settings > hide: gone from decks at once; Bluetooth sightings and fixes taken before the hide can't bring them back
    const c = await pair(k); const before = Date.now() - 3e3;
    clearInterval(c.b.t1); clearInterval(c.b.t2); c.b.send({ type: "hide" });
    const t = await until(gone(c.a, c.b), 5e3);
    await c.b.http("POST", "/api/presence", { lat: c.b.base.lat, lng: c.b.base.lng, acc: 8, age: Date.now() - before });   // the service's last post, still in flight
    await c.b.http("POST", "/api/sightings", { seen: [{ tok: c.a.token, rssi: -60 }] });
    const back = !(await never(() => c.a.deck.has(c.b.name), 20e3));
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 2e3 && !back, removed_after: s(t), came_back: back };
  },
  async logout(k) {   // sign out: gone at once, and the old token is dead (the phone service gets 401 and stops)
    const c = await pair(k); clearInterval(c.b.t1); clearInterval(c.b.t2);
    await c.b.http("POST", "/api/logout");
    const t = await until(gone(c.a, c.b), 5e3);
    const r = await fetch(API + "/api/presence", { method: "POST", headers: { Authorization: "Bearer " + c.b.tok, "Content-Type": "application/json" }, body: JSON.stringify({ lat: c.b.base.lat, lng: c.b.base.lng, acc: 8, age: 0 }) });
    const back = !(await never(() => c.a.deck.has(c.b.name), 15e3));
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 2e3 && r.status === 401 && !back, removed_after: s(t), old_token_status: r.status, came_back: back };
  },
  async delete_account(k) {   // delete: gone at once, socket closed, photo URL dead
    const c = await pair(k); clearInterval(c.b.t1); clearInterval(c.b.t2);
    const uidRes = await c.b.http("GET", "/api/me"); await c.b.http("DELETE", "/api/account");
    const t = await until(gone(c.a, c.b), 5e3);
    const photo = await fetch(API + "/api/photo/" + uidRes?.uid);
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 2e3 && photo.status === 404, removed_after: s(t), photo_status: photo.status };
  },
  async replayed_old_fix(k) {   // a fix 3 min old (queued while offline) must not make someone visible
    const c = crowd(k, OUT, false, 1), a = c.idle[0], b = c.nu; a.start(c.all); await sleep(5e3);
    await b.http("PUT", "/api/profile", { name: b.name, role: "Tester", look: "Testing", photo: "data:image/jpeg;base64,AAAA" });
    await b.http("POST", "/api/presence", { lat: a.base.lat + 3 * M, lng: a.base.lng, acc: 8, age: 180e3 });
    const ok = await never(() => a.deck.has(b.name), 20e3);
    c.all.forEach(p => p.stop()); return { pass: ok, shown: !ok };
  },
  async edit_profile_live(k) {   // a card edit reaches people already looking, quickly (not after the 5 min cache)
    const c = await pair(k);
    await c.b.http("PUT", "/api/profile", { name: c.b.name, role: "Edited role", look: "Testing", photo: "data:image/jpeg;base64,AAAA" });
    const t = await until(() => c.a.lastRole === "Edited role", 15e3);
    c.all.forEach(p => p.stop()); return { pass: t != null, updated_after: s(t) };
  },
  async crowd_60_same_room(k) {   // 60 phones indoors in one room: deck capped at 50, everyone sees someone, no errors
    const base = { lat: 12.9716 + k * 0.1, lng: 77.5946 }, ph = [];
    for (let i = 0; i < 60; i++) ph.push(new Phone("P" + (uid + 1), "tok_" + ++uid, base, (i % 8) * 1.5, Math.floor(i / 8) * 1.5, IN, true, i < 10));
    for (const p of ph) { p.start(ph); await sleep(50); }
    await sleep(45e3);
    const sizes = ph.map(p => p.deck.size), max = Math.max(...sizes), min = Math.min(...sizes);
    ph.forEach(p => p.stop()); return { pass: max <= 50 && min >= 40, deck_min: min, deck_max: max, deck_sizes: sizes.join(",") };
  },
});

// Closed app / screen off: the card must stay while the phone is alive, and still leave once it isn't.
Object.assign(SCENARIOS, {
  async pocket_rssi(k) {   // same room, both phones in pockets: -92 dBm
    const c = crowd(k, IN, true, 1); c.idle[0].rssiAt = () => -92; c.nu.rssiAt = () => -92;
    c.idle[0].start(c.all); await sleep(5e3); c.nu.start(c.all);
    const t = await until(() => mutual(c.nu, c.idle[0]), 60e3);
    c.all.forEach(p => p.stop()); return { pass: t != null && t < 20e3, discovery: s(t) };
  },
  async screen_off_vague_fixes(k) {   // GPS only, then the screen goes off: wifi/cell fixes (200 m) around the same spot. Stays until the good fix expires, never longer
    const c = crowd(k, OUT, false, 1), a = c.idle[0], b = c.nu; a.start(c.all); await sleep(5e3); b.start(c.all);
    const t1 = await until(() => mutual(a, b), 30e3);
    clearInterval(b.t1); b.env = { acc: [150, 250], err: 0.3 }; b.t1 = setInterval(() => b.fix(), 10e3);
    const goodAt = Date.now(), dropped = await until(gone(a, b), 150e3), keptFor = dropped == null ? null : Date.now() - goodAt;
    c.all.forEach(p => p.stop()); return { pass: t1 != null && dropped != null && dropped >= 90e3 && dropped <= 135e3, kept_after_screen_off: s(dropped) };
  },
  async sleeping_long(k) {   // screen off for SLEEP_S (default 10 min): no fixes, no reports, only advertising. The open phone must show it the whole time
    const SL = +(process.env.SLEEP_S || 600) * 1000;
    const c = crowd(k, IN, true, 1), a = c.idle[0], b = c.nu; a.fg = true; a.start(c.all); await sleep(5e3);
    await b.start(c.all); clearInterval(b.t1); clearInterval(b.t2); b.closeApp();
    const t1 = await until(() => a.deck.has(b.name), 30e3), from = Date.now();
    let off = 0; while (Date.now() - from < SL) { if (!a.deck.has(b.name)) off++; await sleep(1000); }
    c.all.forEach(p => p.stop()); return { pass: t1 != null && off === 0, shown_after: s(t1), seconds_missing: off, slept_s: SL / 1000 };
  },
  async two_devices_one_account(k) {   // same account on two phones, both open: no kicking each other off, both get the deck
    const c = crowd(k, OUT, true, 1), a = c.idle[0], b = c.nu; a.start(c.all); await sleep(5e3); await b.start(c.all);
    const twin = new Phone(b.name, b.tok, b.base, b.x + 1, b.y, b.env, false, true); twin.fix = () => null;   // second phone: socket only
    let opens = 0; const orig = twin.socket.bind(twin); twin.socket = () => { opens++; orig(); };
    let bOpens = 0; const ob = b.socket.bind(b); b.socket = () => { bOpens++; ob(); };
    twin.socket(); await sleep(30e3);
    const ok = b.deck.has(a.name) && twin.deck.has(a.name) && a.deck.has(b.name);
    c.all.forEach(p => p.stop()); twin.stop(); return { pass: ok && opens === 1 && bOpens === 0, both_decks: ok, twin_reconnects: opens - 1, phone1_reconnects: bOpens };
  },
});
if (process.env.RESTART_AT_S) SCENARIOS.ble_after_restart = async k => {   // deploy mid-run: a sleeping phone (advertising its old token, never refetching) must stay or come straight back
  const at = +process.env.RESTART_AT_S * 1000, t0 = Date.now();
  const c = crowd(k, IN, true, 1), a = c.idle[0], b = c.nu; a.fg = true; a.start(c.all); await sleep(5e3);
  await b.start(c.all); clearInterval(b.t1); clearInterval(b.t2); b.closeApp();
  const t1 = await until(() => a.deck.has(b.name), 30e3);
  await sleep(Math.max(0, at - (Date.now() - t0)) + 3e3);   // the backend is restarting
  const t2 = await until(() => a.deck.has(b.name) && a.ws?.readyState === 1 && a.seen.at(-1)[0] > t0 + at, 60e3);
  c.all.forEach(p => p.stop()); return { pass: t1 != null && t2 != null && t2 < 20e3, shown_before: s(t1), back_after_restart: s(t2) };
};

const names = Object.keys(SCENARIOS).filter(n => !ONLY || ONLY.split(",").includes(n));
const res = await Promise.all(names.map((n, k) => SCENARIOS[n](k).catch(e => ({ pass: false, error: String(e) }))));
const report = Object.fromEntries(names.map((n, i) => [n, res[i]]));
console.log(JSON.stringify(report, null, 1));
process.exit(res.every(r => r.pass) ? 0 : 1);
