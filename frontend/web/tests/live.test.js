// End-to-end scenarios for the live 30 m circle, run against the preview build (same app code, simulated Firestore + GPS)
const { chromium } = require('playwright');
const SP = process.env.SP || '/tmp';
(async () => {
  const b = await chromium.launch(); const p = await b.newPage({ viewport: { width: 1440, height: 960 } });
  const errs = []; p.on('pageerror', e => errs.push(e.message));
  await p.goto('http://localhost:8777/'); await p.evaluate(() => { Object.keys(localStorage).forEach(k => localStorage.removeItem(k)); }); await p.reload(); await p.waitForTimeout(3300);
  await p.click('[data-jump=nearby]'); await p.waitForTimeout(3500);
  let pass = 0, fail = 0;
  const ok = (c, msg, extra) => { if (c) { pass++; console.log('  ok  ', msg); } else { fail++; console.log('  FAIL', msg, extra ? JSON.stringify(extra) : ''); } };
  const S = () => p.evaluate(() => window.__kinjo.deckState());
  const D = (fn, ...a) => p.evaluate(([fn, a]) => window.__demo[fn](...a), [fn, a]);
  const topName = () => p.evaluate(() => { const cs = [...document.querySelectorAll('#deck .card')].filter(c => c.style.opacity !== '0'); cs.sort((a, b) => b.style.zIndex - a.style.zIndex); return cs[0] && cs[0].querySelector('.card-name').textContent; });
  const settle = (ms = 700) => p.waitForTimeout(ms);
  const box = await p.locator('#deck').boundingBox(); const cx = box.x + box.width / 2, cy = box.y + box.height / 2;
  const swipe = async dx => { await p.mouse.move(cx, cy); await p.mouse.down(); for (let i = 1; i <= 10; i++) { await p.mouse.move(cx + dx * i / 10, cy); await p.waitForTimeout(14); } await p.mouse.up(); await settle(900); };
  const consistent = async (label) => { const s = await S(); const t = await topName(); ok(s.cards === s.names.length && s.dom === s.cards && (s.names.length === 0 || (s.idx >= 0 && s.idx < s.names.length && t === s.names[s.idx])), label + ' — deck consistent', { s, t }); return s; };

  console.log('1. Start: 7 people spread 4–29 m');
  let s = await consistent('start'); ok(s.names.length === 7, 'all 7 inside 30 m are shown', s.names);

  console.log('2. Walking out of the circle');
  await D('place', 'd7', 36, 0, 5); await settle(300); s = await S(); ok(s.names.includes('James Carter'), 'James at 36 m (already shown) stays — no flicker at the edge');
  await D('place', 'd7', 42, 0, 5); await settle(300); s = await S(); ok(!s.names.includes('James Carter'), 'James at 42 m disappears straight away');
  await settle(500); await consistent('after leave');

  console.log('3. Walking into the circle');
  await D('place', 'd7', 35, 0, 5); await settle(300); s = await S(); ok(!s.names.includes('James Carter'), 'James at 35 m (good GPS) not shown yet');
  await D('place', 'd7', 31, 0, 5); await settle(600); s = await S(); ok(s.names.includes('James Carter'), 'James at 31 m appears straight away');
  ok(s.names[s.names.length - 1] === 'James Carter', 'new arrival joins the end of the upcoming pile', s.names);
  await consistent('after enter');

  console.log('4. Someone arrives while your finger is on a card');
  await D('place', 'd6', 0, 80, 5); await settle(600);
  await p.mouse.move(cx, cy); await p.mouse.down(); await p.mouse.move(cx - 60, cy, { steps: 5 });
  const topBefore = (await S()).names[(await S()).idx];
  await D('place', 'd6', 0, 12, 5);
  s = await S(); ok(s.pending && !s.names.includes('Sophie Laurent'), 'deck does not change under your finger (queued)', s);
  await p.mouse.move(cx - 20, cy, { steps: 4 }); await p.mouse.up(); await settle(900);
  s = await S(); ok(!s.pending && s.names.includes('Sophie Laurent'), 'queued arrival applied right after release', s); ok(s.names[s.idx] === topBefore, 'the card in front stays the same person');

  console.log('5. Swipe to the 3rd card, then the person in front leaves');
  await swipe(-160); await swipe(-160); s = await S(); const front = s.names[s.idx], nextUp = s.names[s.idx + 1];
  const uidOf = { 'Elena Rostova': 'd1', 'Marco Reyes': 'd2', 'Noah Bennett': 'd3', 'Amara Okafor': 'd4', 'Daniel Ferreira': 'd5', 'Sophie Laurent': 'd6', 'James Carter': 'd7' };
  await D('place', uidOf[front], 60, 0, 5); await settle(700); s = await consistent('front left');
  ok(s.names[s.idx] === nextUp, `${front} left, ${nextUp} steps up to the front`, s);

  console.log('6. Someone already swiped (on the left pile) leaves');
  const passed = s.names[0], idxBefore = s.idx, frontNow = s.names[s.idx];
  await D('place', uidOf[passed], 0, 70, 5); await settle(700); s = await consistent('passed left');
  ok(s.names[s.idx] === frontNow && s.idx === idxBefore - 1, 'front card unchanged, position shifts correctly', s);

  console.log('7. Someone closes the app while standing in the circle');
  const victim = s.names[s.names.length - 1];
  await D('age', uidOf[victim], 5 * 60e3); await settle(4500); s = await S(); ok(s.names.includes(victim), `${victim}: app closed 5 min ago, still in range -> still shown`);
  await D('age', uidOf[victim], 45 * 60e3); await settle(4500); s = await S(); ok(s.names.includes(victim), `${victim}: app closed 45 min ago -> still shown`);
  await D('age', uidOf[victim], 61 * 60e3); await settle(4500); s = await S(); ok(!s.names.includes(victim), `${victim}: no update for over 1 hour -> removed`);
  await consistent('after stale');

  console.log('8. Everyone leaves, then someone walks in');
  await p.evaluate(() => { for (const uid of [...window.__demo.others.keys()]) window.__demo.place(uid, 200, 200, 5); });
  await settle(800); s = await S(); ok(s.nobody && s.names.length === 0, '"Here is no one?" shows when the circle is empty');
  await D('place', 'd2', 8, 0, 5); await settle(900); s = await consistent('someone back'); ok(!s.nobody && s.names[0] === 'Marco Reyes', 'Marco walks in -> his card appears', s);

  console.log('9. You walk: people come into range as you move');
  await p.evaluate(() => { const o = window.__demo.others; let i = 0; for (const uid of o.keys()) { window.__demo.place(uid, 100 + i * 3, 0, 5); i++; } });
  await settle(500); s = await S(); ok(s.names.length === 0, 'group 100 m north: nobody shown', s.names);
  await D('moveMe', 80, 0, 6); await settle(900); s = await S(); ok(s.names.length >= 5, 'you walk 80 m north -> the group appears (' + s.names.length + ')', s.names);
  await D('moveMe', 0, 0, 6); await settle(900); s = await S(); ok(s.names.length === 0, 'you walk back -> they disappear', s.names);
  await D('moveMe', 0, 0, 12);

  console.log('10. Random churn while swiping (40 events)');
  await p.evaluate(() => { const o = window.__demo.others; let i = 0; for (const uid of o.keys()) { window.__demo.place(uid, 5 + i * 3, 2, 8); i++; } });
  await settle(900);
  const ids = ['d1', 'd2', 'd3', 'd4', 'd5', 'd6', 'd7'];
  for (let k = 0; k < 40; k++) {
    const uid = ids[Math.floor(Math.random() * ids.length)], inCircle = Math.random() < .6;
    await D('place', uid, inCircle ? Math.random() * 28 : 45 + Math.random() * 40, Math.random() * 5, 8);
    if (k % 3 === 0) await swipe(Math.random() < .5 ? -150 : 150); else await settle(120);
  }
  await settle(1200);
  const expected = await p.evaluate(() => { const o = window.__demo.others, me = { lat: 26.9124, lng: 75.7873 }; const R = 6371000, t = Math.PI / 180; const out = []; o.forEach((v, uid) => { const dLa = (v.lat - me.lat) * t, dLo = (v.lng - me.lng) * t; const x = Math.sin(dLa / 2) ** 2 + Math.cos(me.lat * t) * Math.cos(v.lat * t) * Math.sin(dLo / 2) ** 2; out.push([uid, 2 * R * Math.asin(Math.sqrt(x))]); }); return out; });
  s = await consistent('after churn');
  const uidToName = Object.fromEntries(Object.entries(uidOf).map(([n, u]) => [u, n]));
  const mustIn = expected.filter(([, d]) => d <= 30).map(([u]) => uidToName[u]), mustOut = expected.filter(([, d]) => d > 48).map(([u]) => uidToName[u]);
  ok(mustIn.every(n => s.names.includes(n)), 'everyone within 30 m is on the deck', { mustIn, names: s.names });
  ok(mustOut.every(n => !s.names.includes(n)), 'nobody beyond the circle is on the deck', { mustOut, names: s.names });

  console.log('11. Hide my profile');
  await p.evaluate(() => window.__kinjo.go('settings')); await settle(900);
  await p.click('#visSwitch'); await settle(600);
  const presenceGone = await p.evaluate(() => !Object.keys(window.__demo.mem.docs).some(k => k.startsWith('presence/')));
  ok(presenceGone, 'hiding deletes your position -> you vanish for others');
  await p.evaluate(() => window.__kinjo.go('nearby')); await settle(900); s = await S();
  ok(s.hiddenMode && s.names.length === 0, 'nearby shows "You’re hidden." and no cards');
  await p.screenshot({ path: SP + '/t_hidden.png', clip: await p.evaluate(() => { const r = document.querySelector('.device').getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height }; }) });
  await p.click('#visOn'); await settle(1500); s = await S();
  const presenceBack = await p.evaluate(() => Object.keys(window.__demo.mem.docs).some(k => k.startsWith('presence/')));
  ok(!s.hiddenMode && s.names.length > 0 && presenceBack, 'showing again: you are visible and people are back', s);
  await p.evaluate(() => window.__kinjo.go('settings')); await settle(900);
  await p.screenshot({ path: SP + '/t_settings.png', clip: await p.evaluate(() => { const r = document.querySelector('.device').getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height }; }) });

  console.log(`\nlive scenarios: ${pass} passed, ${fail} failed; page errors: ${JSON.stringify(errs)}`);
  await b.close(); process.exit(fail || errs.length ? 1 : 0);
})();
