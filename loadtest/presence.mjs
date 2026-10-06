// Realtime load test: N phones on WebSockets reporting positions, one dense
// crowd (a venue) plus everyone else spread across a city, while measuring REST
// latency. Run against a STAGING copy, never production.
//   API=http://localhost:8099 USERS=3000 DENSE=200 SECONDS=30 node loadtest/presence.mjs
// Tokens are tok_1..tok_N (see the seed SQL in loadtest/README.md).
const API = process.env.API || "http://localhost:8099";
const WS = API.replace(/^http/, "ws") + "/ws";
const USERS = +(process.env.USERS || 3000), DENSE = +(process.env.DENSE || 200);
const SECONDS = +(process.env.SECONDS || 30), EVERY = +(process.env.EVERY_MS || 5000);
const C = { lat: 12.9716, lng: 77.5946 }, M = 1 / 111320;   // ~degrees per metre
const s = { open: 0, closed: 0, errors: 0, msgs: 0, bytes: 0, cards: 0, denseMsgs: 0 };

function spot(i) {
  const r = i < DENSE ? 10 : 5000;                       // dense: within 10 m; rest: ±5 km
  return { lat: C.lat + (Math.random() * 2 - 1) * r * M, lng: C.lng + (Math.random() * 2 - 1) * r * M };
}
const sockets = [];
for (let i = 1; i <= USERS; i++) {
  const ws = new WebSocket(WS, ["kinjo", "tok_" + i]), p = spot(i - 1), dense = i <= DENSE;
  const send = () => ws.readyState === 1 && ws.send(JSON.stringify({ type: "pos", lat: p.lat + (Math.random() - .5) * 2 * M, lng: p.lng, acc: 8 }));
  ws.onopen = () => { s.open++; send(); setTimeout(() => setInterval(send, EVERY), Math.random() * EVERY); };
  ws.onmessage = e => { s.msgs++; s.bytes += e.data.length; const m = JSON.parse(e.data); if (m.people) { s.cards += m.people.length; if (dense) s.denseMsgs++; } };
  ws.onerror = () => s.errors++;
  ws.onclose = () => s.closed++;
  sockets.push(ws);
  if (i % 200 === 0) await new Promise(r => setTimeout(r, 50));   // ramp, don't stampede
}

// REST latency under load: GET /api/me at concurrency 50.
const lat = [];
async function rest(stop) {
  while (Date.now() < stop) {
    const t = performance.now(), i = 1 + Math.floor(Math.random() * USERS);
    try { const r = await fetch(API + "/api/me", { headers: { Authorization: "Bearer tok_" + i } }); await r.arrayBuffer(); if (!r.ok) s.errors++; }
    catch { s.errors++; }
    lat.push(performance.now() - t);
  }
}
const warm = 5000; await new Promise(r => setTimeout(r, warm));
const m0 = { ...s }, t0 = Date.now();
await Promise.all(Array.from({ length: 50 }, () => rest(t0 + SECONDS * 1000)));
const dt = (Date.now() - t0) / 1000, pct = q => lat.sort((a, b) => a - b)[Math.floor(q * (lat.length - 1))].toFixed(1);
console.log(JSON.stringify({
  sockets_open: s.open - s.closed, socket_errors: s.errors,
  pushes_per_s: ((s.msgs - m0.msgs) / dt).toFixed(0), push_kB_per_s: ((s.bytes - m0.bytes) / dt / 1024).toFixed(0),
  avg_cards_per_dense_push: (s.cards / Math.max(1, s.msgs)).toFixed(1), dense_pushes_per_client_per_s: ((s.denseMsgs - m0.denseMsgs) / dt / DENSE).toFixed(2),
  rest_rps: (lat.length / dt).toFixed(0), rest_p50_ms: pct(.5), rest_p95_ms: pct(.95), rest_p99_ms: pct(.99),
}, null, 1));
process.exit(0);
