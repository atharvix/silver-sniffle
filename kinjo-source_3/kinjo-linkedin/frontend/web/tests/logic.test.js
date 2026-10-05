// Unit tests for the 30 m matching maths, pulled straight out of app.fb.js
const fs = require('fs');
const src = fs.readFileSync(__dirname + '/../src/app.fb.js', 'utf8');
const pick = (re) => { const m = src.match(re); if (!m) throw new Error('missing ' + re); return m[0]; };
const code = [
  pick(/const RADIUS[\s\S]*?const CELL = [^\n]*\n/),
  pick(/const cellOf = [^\n]*\n/),
  pick(/function cellsAround[\s\S]*?\n}\n/),
  pick(/function dist\(a, b\)[\s\S]*?\n}\n/),
  pick(/function inside[\s\S]*?\n}\n/),
  pick(/const clampN = [^\n]*\n/),
  'module.exports={RADIUS,SLACK_MIN,SLACK_MAX,EXIT_GAP,STALE_MS,CELL,cellOf,cellsAround,dist,inside};'
].join('\n');
const m = {}; new Function('module', code)(m); const L = m.exports;
let pass = 0, fail = 0; const ok = (c, msg) => { if (c) pass++; else { fail++; console.log('FAIL', msg); } };
const offset = (p, n, e) => ({ lat: p.lat + n / 111320, lng: p.lng + e / (111320 * Math.cos(p.lat * Math.PI / 180)) });

// 1. distance accuracy vs known offsets (metres) at several latitudes
for (const lat of [0, 12.97, 19.07, 26.91, 28.6, 40.7, 51.5, 60.1, -33.9]) {
  const p = { lat, lng: 77.5 };
  for (const [n, e] of [[30, 0], [0, 30], [21.2, 21.2], [-30, 0], [0, -29.9], [5, 3], [100, 0]]) {
    const d = L.dist(p, offset(p, n, e)), want = Math.hypot(n, e);
    ok(Math.abs(d - want) < 0.25, `dist lat ${lat} (${n},${e}) got ${d.toFixed(2)} want ${want.toFixed(2)}`);
  }
}
// 2. thresholds with good GPS (5 m each): in ≤ 33 m when new, stays until 40 m
ok(L.inside(29.9, 5, 5, false), 'good gps 29.9 new -> in');
ok(L.inside(32.9, 5, 5, false), 'good gps 32.9 new -> in (3 m slack)');
ok(!L.inside(33.5, 5, 5, false), 'good gps 33.5 new -> out');
ok(L.inside(39, 5, 5, true), 'good gps 39 shown -> stays (hysteresis)');
ok(!L.inside(40.5, 5, 5, true), 'good gps 40.5 shown -> leaves');
// poor GPS widens slack to at most 10 m
ok(L.inside(39.5, 30, 30, false), 'poor gps 39.5 new -> in');
ok(!L.inside(40.5, 60, 60, false), 'very poor gps capped: 40.5 new -> out');
ok(!L.inside(47.5, 60, 60, true), 'very poor gps shown capped: 47.5 -> out');
ok(!L.inside(31, 5, 5, false) === false, 'sanity');
ok(!L.inside(80, 5, 5, true), 'far away never in');
// 3. grid coverage: anyone within reach must be in one of the queried cells, at any latitude/position
let cov = 0, maxCells = 0;
for (let k = 0; k < 20000; k++) {
  const lat = -65 + Math.random() * 130, lng = -179 + Math.random() * 358;
  const me = { lat, lng }, cells = new Set(L.cellsAround(lat, lng)); maxCells = Math.max(maxCells, cells.size);
  const r = Math.random() * (L.RADIUS + L.SLACK_MAX + L.EXIT_GAP), a = Math.random() * Math.PI * 2;
  const o = offset(me, r * Math.cos(a), r * Math.sin(a));
  if (Math.abs(lat) <= 60) { ok(cells.has(L.cellOf(o.lat, o.lng)), `coverage lat ${lat.toFixed(2)} r ${r.toFixed(1)}`); cov++; }
}
ok(maxCells <= 30, 'query stays within Firestore 30-value limit (max ' + maxCells + ')');
console.log(`logic: ${pass} passed, ${fail} failed (coverage samples ${cov}, max cells ${maxCells})`);
process.exit(fail ? 1 : 0);
