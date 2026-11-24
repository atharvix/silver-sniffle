// Unit test for the one piece of maths left in the app, dist(): it only throttles position reports
// (the 30 m matching itself runs on the server: backend/prox.go, tested in backend/prox_test.go).
// Pulls dist() straight out of app.fb.js. Run: node frontend/web/tests/logic.test.js
const fs = require('fs');
const src = fs.readFileSync(__dirname + '/../src/app.fb.js', 'utf8');
const pick = (re) => { const m = src.match(re); if (!m) throw new Error('missing ' + re); return m[0]; };
const code = [
  pick(/function dist\(a, b\)[\s\S]*?\n}\n/),
  'module.exports={dist};'
].join('\n');
const m = {}; new Function('module', code)(m); const L = m.exports;
let pass = 0, fail = 0; const ok = (c, msg) => { if (c) pass++; else { fail++; console.log('FAIL', msg); } };
const offset = (p, n, e) => ({ lat: p.lat + n / 111320, lng: p.lng + e / (111320 * Math.cos(p.lat * Math.PI / 180)) });

// distance accuracy vs known offsets (metres) at several latitudes
for (const lat of [0, 12.97, 19.07, 26.91, 28.6, 40.7, 51.5, 60.1, -33.9]) {
  const p = { lat, lng: 77.5 };
  for (const [n, e] of [[30, 0], [0, 30], [21.2, 21.2], [-30, 0], [0, -29.9], [5, 3], [100, 0]]) {
    const d = L.dist(p, offset(p, n, e)), want = Math.hypot(n, e);
    ok(Math.abs(d - want) < 0.25, `dist lat ${lat} (${n},${e}) got ${d.toFixed(2)} want ${want.toFixed(2)}`);
  }
}
console.log(`logic: ${pass} passed, ${fail} failed`);
process.exit(fail ? 1 : 0);
