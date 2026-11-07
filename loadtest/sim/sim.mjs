// 50-phones-per-scenario proximity simulation against a REAL Kinjo backend.
// Server side is real (fixes, BLE tokens, sightings, WebSocket decks). Phone side (radios, OS limits,
// GPS error, battery) is a model, parameterised below. VARIANT: A v1, B v1 + phase 1+2 fixes, C B + GPS only when open/moving, D v2 phones (docs/proximity.md).
const API = process.env.API, WS = API.replace(/^http/, "ws") + "/ws", V = process.env.VARIANT || "A";
const DUR = +(process.env.DUR_S || 180) * 1000, ONLY = process.env.ONLY;
let seed = 7 + V.charCodeAt(0) * 0;   // same seed for all variants: same phones, same luck
const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
const gauss = () => Math.sqrt(-2 * Math.log(rnd() || 1e-9)) * Math.cos(2 * Math.PI * rnd());
const M = 1 / 111320, sleep = ms => new Promise(r => setTimeout(r, ms));
const T0 = Date.now(), now = () => Date.now() - T0;

// ---- phone-side model parameters ----
const P = {
  scan: {   // per scan step: step period (ms), chance an in-range advert is caught in that step
    android_open: [1000, 0.9], android_pocket: [5000, 0.6], ios_open: [1000, 0.85], ios_bg: [5000, 0.3],
    android_pocket_D: [4000, 0.8],   // D: SCAN_MODE_BALANCED in the background (1024 ms of every 4096 ms)
  },
  rssi: d => -55 - 25 * Math.log10(Math.max(d, 1)) + gauss() * 4,   // 1 m ≈ -55 dBm, n=2.5
  sens: -100, pocketDb: 10, wallDb: 10, floorDb: 15,
  gattMs: [1000, 3000], gattFail: 0.3, gattRetryA: 60e3, gattRetryB: [5e3, 10e3, 20e3, 40e3], gattRetryD: [2e3, 5e3, 10e3, 20e3],
  connAdvA: 0.85,   // Android phones with the 2nd (connectable, iOS-visible) advert: needs CONNECT + a spare slot
  reportOpen: 5e3, reportClosed: 10e3, newMs: 8e3, newDelay: 300,
  fix: { open: 10e3, android_pocket: 30e3, ios_bg: 45e3, moving: 10e3 },
  OUT: { acc: [5, 12], err: 1 }, IN: { acc: [15, 45], err: 1.8 }, wifiLie: 0.1,   // 10% indoor fixes claim 8-15 m but are ~40 m off
  APPROX: { acc: [1200, 2000], err: 0.5 },
};
// battery model (mAh): screen excluded (an open app's screen is the user's choice)
const B = { fix: 0.1, scan: { android_open: 9, android_pocket: 2, android_pocket_D: 4.5, ios_open: 6, ios_bg: 1 }, adv: 1.2, adv2: 0.5, svc: 1, radioWake: 0.07, radioTail: 0.01, tailMs: 10e3, gatt: 0.03, cap: 4500 };

let uidN = 0;
class Phone {
  constructor(sc, o) {
    Object.assign(this, { platform: "android", state: "open", bt: true, loc: "precise", env: "in", x: 0, y: 0, floor: 0, room: 0, stale: false, startAt: 0 }, o);
    this.sc = sc; this.n = ++uidN; this.tok = "tok_" + this.n; this.name = sc.name + "_" + this.n;
    this.deck = new Map();   // name -> shown since
    this.hist = [];          // [t, name, +1/-1]
    this.known = new Map();  // peripheral (phone) -> token read over GATT
    this.failed = new Map(); // peripheral -> [failedAt, tries]
    this.gattQ = []; this.gattBusy = 0;
    this.batch = new Map(); this.heardAt = new Map(); this.sentAt = 0;
    this.e = { fixes: 0, posts: 0, radio: 0, gatt: 0, lastRadio: -1e9 };
    this.conn2 = rnd() < P.connAdvA;
  }
  pos(t) { return this.move ? this.move(t) : { x: this.x, y: this.y }; }
  moving(t) { return !!this.move && this.move(t + 1000).x !== this.move(t).x; }
  get android() { return this.platform === "android"; }
  get mode() { return this.android ? (this.state === "open" ? "android_open" : V === "D" ? "android_pocket_D" : "android_pocket") : (this.state === "open" ? "ios_open" : "ios_bg"); }
  // capabilities per variant
  alive() {
    if (this.state === "killed" || this.state === "swiped") return false;
    if (V === "A" && this.android && this.loc === "none") return false;   // FGS type location can't start without location
    return true;
  }
  advertises() { return this.alive() && this.bt && !!this.bleTok; }
  scans() { return this.alive() && this.bt && (V !== "A" || !this.android || this.loc === "precise"); }   // A: no neverForLocation -> Android needs FINE
  fixes(t) { return this.alive() && this.loc !== "none" && (V === "D" ? this.state === "open" : V !== "C" || this.state === "open" || this.moving(t)); }
  fixEvery(t) { return this.state === "open" ? P.fix.open : this.moving(t) && V === "C" ? P.fix.moving : this.android ? P.fix.android_pocket : P.fix.ios_bg; }
  radio() { const t = now(); this.e.radio += t - this.e.lastRadio > B.tailMs ? B.radioWake : B.radioTail; this.e.lastRadio = t; }
  async http(method, path, body) {
    this.radio(); this.e.posts++;
    try {
      const r = await fetch(API + path, { method, headers: { Authorization: "Bearer " + this.tok, "Content-Type": "application/json" }, body: body && JSON.stringify(body) });
      return r.status === 200 ? r.json() : null;
    } catch { return null; }
  }
  async fetchTok() { this.bleTok = (await this.http("GET", "/api/ble-token"))?.token; }
  fix(t) {
    this.e.fixes++;
    const p = this.pos(t), env = this.loc === "approx" ? P.APPROX : this.env === "out" ? P.OUT : P.IN;
    let [lo, hi] = env.acc, acc = Math.round(lo + rnd() * (hi - lo)), s = acc / 1.5 * env.err, ex, ey;
    if (env === P.IN && rnd() < P.wifiLie) { acc = 8 + Math.round(rnd() * 7); const a = rnd() * 6.283; ex = p.x + 40 * Math.cos(a); ey = p.y + 40 * Math.sin(a); }
    else { ex = p.x + gauss() * s; ey = p.y + gauss() * s; }
    const b = this.sc.base;
    return this.http("POST", "/api/presence", { lat: b.lat + ey * M, lng: b.lng + ex * M / Math.cos(b.lat * Math.PI / 180), acc, age: 0 });
  }
  socket() {
    const ws = this.ws = new WebSocket(WS, ["kinjo", this.tok]);
    ws.onmessage = e => {
      const m = JSON.parse(e.data); if (m.type !== "nearby") return;
      const t = now(), names = new Set(m.people.map(p => p.name));
      for (const n of names) if (!this.deck.has(n)) { this.deck.set(n, t); this.hist.push([t, n, 1]); }
      for (const n of [...this.deck.keys()]) if (!names.has(n)) { this.deck.delete(n); this.hist.push([t, n, -1]); }
    };
    ws.onclose = () => { if (this.ws === ws && !this.done) setTimeout(() => this.ws === ws && this.socket(), 1000); };
    ws.onerror = () => {};
  }
  stop() { this.done = true; clearInterval(this.ti); try { this.ws.close(); } catch {} }

  // can this phone (scanner) get other's token from one caught advert? returns "direct" | "gatt" | null
  path(o) {
    if (this.android && o.android) return "direct";
    if (this.android && !o.android) return V !== "A" && o.state === "open" ? "direct" : "gatt";   // B: read an open iPhone's local name
    if (!this.android && o.android) return V !== "A" || o.conn2 ? "direct" : null;              // B: KINJO UUID in Android's main advert
    if (o.state === "open") return "direct";                                                      // iPhone <- open iPhone: local name
    return this.state === "open" ? "gatt" : null;                                                 // overflow area: only a screen-on scanner sees it
  }
  rssiFrom(o, t) {
    const a = this.pos(t), b = o.pos(t), d = Math.hypot(a.x - b.x, a.y - b.y, (this.floor - o.floor) * 4);
    let r = P.rssi(d);
    if (this.state !== "open") r -= P.pocketDb; if (o.state !== "open") r -= P.pocketDb;
    if (this.room !== o.room) r -= P.wallDb * Math.abs(this.room - o.room);
    if (this.floor !== o.floor) r -= P.floorDb * Math.abs(this.floor - o.floor);
    return Math.round(r);
  }
  scanStep(t) {
    const [, p] = P.scan[this.mode];
    for (const o of this.sc.phones) {
      if (o === this || !o.advertises()) continue;
      const r = this.rssiFrom(o, t);
      if (r < P.sens || rnd() > p) continue;
      const how = this.path(o);
      if (!how) continue;
      let tok;
      if (how === "direct") tok = o.bleTok;
      else {
        const k = this.known.get(o);
        if (k && k === o.bleTok) tok = k;
        else { this.queueGatt(o, t); continue; }
      }
      if (!(this.batch.get(tok) >= r)) this.batch.set(tok, r);
      const last = this.heardAt.get(tok); this.heardAt.set(tok, t);
      if (last === undefined || t - last > P.newMs) this.newHeard = true;
    }
  }
  queueGatt(o, t) {
    const f = this.failed.get(o);
    if (f) { const wait = V === "A" ? P.gattRetryA : (V === "D" ? P.gattRetryD : P.gattRetryB)[Math.min(f[1] - 1, 3)]; if (t - f[0] < wait) return; }
    if (!this.gattQ.includes(o)) this.gattQ.push(o);
  }
  gattStep(t) {   // one connection at a time (D: up to 3, Ble.MAX_CONNS)
    this.slots = (this.slots || []).filter(e => e > t);
    while (this.slots.length < (V === "D" ? 3 : 1) && this.gattQ.length) {
      const o = this.gattQ.shift(), ms = P.gattMs[0] + rnd() * (P.gattMs[1] - P.gattMs[0]);
      this.e.gatt++;
      if (rnd() < P.gattFail) { this.slots.push(t + 8000); const f = this.failed.get(o); this.failed.set(o, [t + 8000, (f?.[1] || 0) + 1]); continue; }   // 8 s connect timeout
      this.slots.push(t + ms); this.failed.delete(o);
      setTimeout(() => { if (o.advertises()) this.known.set(o, o.bleTok); }, ms);
    }
  }
  report(t) {
    const every = (this.state === "open" ? P.reportOpen : P.reportClosed) * (V === "D" ? 2 : 1);   // D: 10 s open / 20 s closed
    if (!this.newHeard && t - this.sentAt < every) return;
    if (!this.batch.size) { this.sentAt = t; return; }
    const isNew = this.newHeard;
    const seen = [...this.batch].map(([tok, rssi]) => ({ tok, rssi })); this.batch = new Map(); this.newHeard = false; this.sentAt = t;
    const go = async () => {
      for (let i = 0; i < seen.length; i += 32) await this.http("POST", "/api/sightings", { seen: seen.slice(i, i + 32) });
      if (this.stale && V !== "A") { this.stale = false; await this.fetchTok(); }   // B/C: server answers 409, phone fetches a token now
    };
    setTimeout(go, isNew ? P.newDelay : 0);
  }
  async start() {
    await sleep(this.startAt);
    this.t0 = now();
    await this.http("PUT", "/api/profile", { name: this.name, role: "Tester", look: "sim", photo: "data:image/jpeg;base64,AAAA" });
    this.socket();   // probe socket for every phone: "would the card be there if they looked now"
    if (this.bt && this.alive()) { if (this.stale) this.bleTok = [...Array(16)].map(() => "0123456789abcdef"[Math.floor(rnd() * 16)]).join(""); else await this.fetchTok(); }
    this.e = { fixes: 0, posts: 0, radio: 0, gatt: 0, lastRadio: now() }; this.t0 = now();   // battery counted from here (onboarding excluded)
    const scanGap = V === "A" && this.state === "open" ? rnd() * 10e3 : 0;   // A: Ble.foreground(true) stops the scan until the next 10 s tick
    let nextFix = now(), nextScan = now() + scanGap;
    this.ti = setInterval(() => {
      const t = now();
      if (this.fixes(t) && t >= nextFix) { this.fix(t); nextFix = t + this.fixEvery(t); }
      if (this.scans() && this.bleTok) {
        if (t >= nextScan) { this.scanStep(t); nextScan = t + P.scan[this.mode][0]; }
        this.gattStep(t); this.report(t);
      }
    }, 250);
  }
  battery(ms) {   // mAh per hour
    const h = ms / 3600e3; let mah = 0;
    if (this.alive()) mah += B.svc * h;
    if (this.scans() && this.bleTok) mah += B.scan[this.mode] * h;
    if (this.advertises()) mah += (B.adv + (this.android && (V !== "A" || this.conn2) ? B.adv2 : 0)) * h;
    mah += this.e.fixes * B.fix + this.e.radio + this.e.gatt * B.gatt;
    return mah / h;
  }
}

// ---- scenarios: each gets its own spot (11 km apart) and 50 phones ----
let spotK = 0;
const mk = (name, phones) => ({ name, base: { lat: 12.9716 + ++spotK * 0.1, lng: 77.5946 }, phones: [], spec: phones });
const realistic = ios => {   // a phone as found in the wild
  const r = rnd();
  const state = r < 0.35 ? "open" : r < 0.85 ? "pocket" : ios ? "swiped" : "killed";
  const lr = rnd(), loc = ios ? (lr < 0.85 ? "precise" : lr < 0.95 ? "approx" : "none") : (lr < 0.75 ? "precise" : lr < 0.9 ? "approx" : "none");
  return { platform: ios ? "ios" : "android", state, loc, bt: rnd() > 0.08 };
};
const plat = i => (i % 5 < 3 ? "android" : "ios");
const pairs = (n, f) => Array.from({ length: n }, (_, i) => { const [a, b] = f(i); return [{ ...a, x: i * 400, y: 0 }, { ...b, x: i * 400 + (b.dx ?? 5), y: b.dy ?? 0 }]; }).flat();

const SC = [
  mk("same_room_mix", Array.from({ length: 50 }, (_, i) => ({ ...realistic(plat(i) === "ios"), x: Math.floor(i / 10) * 400 + (i % 5) * 2, y: (i % 10 >= 5 ? 6 : 0) + rnd() * 2, room: 0 }))),
  mk("pairs_25m", pairs(25, i => [{ platform: plat(i), state: "open", env: "out" }, { platform: plat(i + 2), state: i % 2 ? "open" : "pocket", env: "out", dx: 25 }])),
  mk("pairs_40m_NO", pairs(25, i => [{ platform: plat(i), state: "open", env: "out" }, { platform: plat(i + 2), state: "open", env: "out", dx: 40 }])),
  mk("pairs_80m_NO", pairs(25, i => [{ platform: plat(i), state: "open", env: "out" }, { platform: plat(i + 2), state: "open", env: "out", dx: 80 }])),
  mk("wall_or_floor", pairs(25, i => i % 2 ? [{ platform: plat(i), floor: 0 }, { platform: plat(i + 2), floor: 1, dx: 0 }] : [{ platform: plat(i), room: 0 }, { platform: plat(i + 2), room: 2, dx: 12 }])),
  mk("walk_past", [
    ...Array.from({ length: 25 }, (_, i) => ({ platform: plat(i), state: "open", env: "out", x: i * 400, y: 0 })),
    ...Array.from({ length: 25 }, (_, i) => ({ platform: plat(i + 1), state: "pocket", env: "out", x: i * 400, y: 5, walk: true })),
  ]),
  mk("crowd50_all_alive", Array.from({ length: 50 }, (_, i) => ({ platform: plat(i), state: i % 3 ? "pocket" : "open", x: rnd() * 14, y: rnd() * 14 }))),   // everyone findable: no killed/swiped, Bluetooth on
  mk("crowd50_all_pocket", Array.from({ length: 50 }, (_, i) => ({ platform: plat(i), state: "pocket", x: rnd() * 14, y: rnd() * 14 }))),
  mk("crowd_20m", Array.from({ length: 50 }, (_, i) => ({ ...realistic(plat(i) === "ios"), x: rnd() * 14, y: rnd() * 14 }))),
  mk("platform_pairs_open", pairs(25, i => i < 8 ? [{ platform: "android" }, { platform: "android" }] : i < 16 ? [{ platform: "android" }, { platform: "ios" }] : [{ platform: "ios" }, { platform: "ios" }])),
  mk("open_vs_pocket", pairs(25, i => { const a = i % 4 < 2 ? "android" : "ios", b = i % 2 ? "ios" : "android"; return [{ platform: a, state: "open" }, { platform: b, state: "pocket" }]; })),
  mk("ios_both_bg", pairs(25, () => [{ platform: "ios", state: "pocket" }, { platform: "ios", state: "pocket" }])),
  mk("bt_off_one", pairs(25, i => [{ platform: plat(i), env: i < 12 ? "in" : "out" }, { platform: plat(i + 1), bt: false, env: i < 12 ? "in" : "out" }])),
  mk("approx_location", pairs(25, i => [{ platform: "android", loc: "approx" }, { platform: i % 2 ? "ios" : "android" }])),
  mk("stale_token", pairs(25, i => [{ platform: plat(i), stale: true }, { platform: plat(i + 1) }])),
].filter(s => !ONLY || ONLY.split(",").includes(s.name));
for (const s of SC) for (const o of s.spec) {
  const p = new Phone(s, { ...o, startAt: rnd() * 5e3 });
  if (o.walk) { const x0 = o.x - 150; p.move = t => ({ x: x0 + Math.max(0, t - 10e3) * 0.0014, y: o.y }); }   // walks past at 1.4 m/s, 10 s in
  s.phones.push(p);
}

const all = SC.flatMap(s => s.phones);
await Promise.all(all.map(p => p.start()));
await sleep(DUR - now());
const END = now();
all.forEach(p => p.stop());

// ---- metrics ----
const pct = (a, q) => { if (!a.length) return null; const b = [...a].sort((x, y) => x - y); return b[Math.min(b.length - 1, Math.floor(q * b.length))]; };
const out = { variant: V, dur_s: END / 1000, scenarios: {} };
for (const s of SC) {
  const ps = s.phones;
  // O = viewer has the app open (what a person sees); A = any viewer, probe socket ("if they opened it now")
  const m = { O: { near: 0, found: 0, t: [] }, A: { near: 0, found: 0, t: [] } };
  let far = 0, fp = 0, flick = 0; const slow = {};   // first cards > 5 s, by "viewer -> other" kind
  const kind = (v, u, d) => `${v.platform}/${v.state}->${u.platform}/${u.state} ${Math.round(d / 10) * 10}m${v.bt && u.bt ? "" : " btoff"}`;
  for (const v of ps) for (const u of ps) {
    if (u === v) continue;
    const ks = v.state === "open" ? ["O", "A"] : ["A"];
    let lastOff = null;
    for (const [t, n, k] of v.hist) if (n === u.name) { if (k < 0) lastOff = t; else if (lastOff != null && t - lastOff <= 60e3) { flick++; lastOff = null; } }
    if (u.move || v.move) {   // walk past: near while within 30 m
      let enter = null; for (let t = 0; t < END; t += 500) { const a = v.pos(t), b = u.pos(t); if (Math.hypot(a.x - b.x, a.y - b.y) <= 30) { enter = t; break; } }
      if (enter == null) continue;
      const hit = v.hist.find(([t, n, k]) => n === u.name && k === 1);   // shown before they reach 30 m counts as 0 s
      for (const k of ks) { m[k].near++; if (hit) { m[k].found++; m[k].t.push(Math.max(0, hit[0] - enter)); } }
      continue;
    }
    const firstShown = v.hist.find(([, n, k]) => n === u.name && k === 1)?.[0];
    const a = v.pos(0), b = u.pos(0), d = Math.hypot(a.x - b.x, a.y - b.y, (v.floor - u.floor) * 4);
    if (d <= 30) {
      for (const k of ks) { m[k].near++; if (firstShown != null) { m[k].found++; m[k].t.push(firstShown - Math.max(v.t0, u.t0)); } }
      const ms = firstShown == null ? Infinity : firstShown - Math.max(v.t0, u.t0);
      if (ms > 5e3 && v.alive() && u.alive()) { const k = kind(v, u, d) + (ms === Infinity ? " never" : ""); slow[k] = (slow[k] || 0) + 1; }
    }
    else if (d >= 39 && d < 300) { far++; if (firstShown != null) fp++; }
  }
  const f = k => ({ near: m[k].near, found_pct: m[k].near ? +(100 * m[k].found / m[k].near).toFixed(1) : null,
    p50_s: m[k].t.length ? +(pct(m[k].t, 0.5) / 1000).toFixed(1) : null, p95_s: m[k].t.length ? +(pct(m[k].t, 0.95) / 1000).toFixed(1) : null });
  const batt = ps.filter(p => p.alive()).map(p => p.battery(END - p.t0)), bp = ps.filter(p => p.alive() && p.state !== "open").map(p => p.battery(END - p.t0));
  const avg = a => a.length ? +(a.reduce((x, y) => x + y, 0) / a.length).toFixed(1) : null;
  out.scenarios[s.name] = {
    phones: ps.length, slow, open_view: f("O"), any_view: f("A"), far_pairs: far, false_pos: fp, flicker: flick,
    mAh_h_all: avg(batt), mAh_h_background: avg(bp),
    fixes: ps.reduce((a, p) => a + p.e.fixes, 0), posts: ps.reduce((a, p) => a + p.e.posts, 0), gatt: ps.reduce((a, p) => a + p.e.gatt, 0),
  };
}
console.log(JSON.stringify(out, null, 1));
process.exit(0);
