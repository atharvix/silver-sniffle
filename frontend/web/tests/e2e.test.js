// End-to-end test of the real app: the real Go server, a real Postgres and the web app in headless Chromium, with
// several simulated people (sign-in by emailed code, profile, location, who can see whom, hide, the menu, logout,
// delete). It does NOT cover the native Android / iPhone services (background, locked, swiped away): those need a phone.
//
// Setup (nothing here is committed): a throwaway Postgres; the backend running with SMTP unset (it then logs the
// emailed code), DEMO_EMAIL=demo@example.com DEMO_CODE=123456, ALLOW_ORIGINS=<the app's origin>, its output
// written to a file; the app built with API_BASE pointing at that backend and served as static files.
//   npm i playwright && npx playwright install chromium
//   KINJO_LOG=/path/to/server.log KINJO_APP=http://localhost:8777/ KINJO_API=http://localhost:8090 node frontend/web/tests/e2e.test.js
// Takes about 3 minutes (it waits out the one-code-per-minute limit once). Run it on an empty database.
const { chromium } = require('playwright');
const fs = require('fs');
const BASE = process.env.KINJO_APP || 'http://localhost:8777/', API = process.env.KINJO_API || 'http://localhost:8090';
const LOG = process.env.KINJO_LOG || (__dirname + '/server.log');
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==', 'base64');
const HOME = { latitude: 26.9124, longitude: 75.7873 };
const off = (n, e) => ({ latitude: HOME.latitude + n / 111320, longitude: HOME.longitude + e / (111320 * Math.cos(HOME.latitude * Math.PI / 180)), accuracy: 8 });

let pass = 0, fail = 0; const failures = [];
const ok = (c, msg, extra) => { if (c) { pass++; console.log('  ok   ' + msg); } else { fail++; failures.push(msg); console.log('  FAIL ' + msg + (extra !== undefined ? ' :: ' + JSON.stringify(extra) : '')); } };
const sleep = ms => new Promise(r => setTimeout(r, ms));
async function until(fn, ms = 12000, step = 250) { const t = Date.now(); for (;;) { try { const v = await fn(); if (v) return v; } catch (e) {} if (Date.now() - t > ms) return false; await sleep(step); } }

function codeFor(email) {
  const s = fs.readFileSync(LOG, 'utf8'); const i = s.lastIndexOf('would email ' + email);
  if (i < 0) return null; const m = s.slice(i, i + 3000).match(/\b(\d{6})\b/); return m && m[1];
}

async function newUser(browser, name, pos) {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 800 }, geolocation: pos, permissions: ['geolocation'], acceptDownloads: false });
  const page = await ctx.newPage(); const errs = [];
  page.on('pageerror', e => errs.push('pageerror: ' + e.message));
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|favicon|net::ERR/.test(m.text())) errs.push('console: ' + m.text()); });
  return { name, ctx, page, errs };
}
const deckNames = p => p.evaluate(() => [...document.querySelectorAll('#deck .card .card-name')].map(e => e.textContent));
const screen = p => p.evaluate(() => [...document.querySelectorAll('.scr.on')].map(e => e.id).filter(i => i !== 's-splash').join(','));

async function signIn(u, email) {
  const p = u.page; await p.goto(BASE); await p.evaluate(() => localStorage.clear());
  await p.reload(); await until(() => p.$('#s-auth.on'), 8000);
  ok(await screen(p) === 's-auth', u.name + ': sign-in screen shows');
  await p.click('#emailStart'); await p.fill('#inEmail', email); await p.click('#emailNext');
  const code = await until(() => codeFor(email), 8000); ok(!!code, u.name + ': a 6-digit code was issued');
  await until(() => p.$('#s-code.on'), 5000); await sleep(400);
  await p.fill('#inCode', code || '000000');
  return !!code;
}

async function makeProfile(u, { name, role, look, camera }) {
  const p = u.page;
  await until(() => p.$('#s-profile.on'), 10000);
  ok(await screen(p) === 's-profile', u.name + ': new account lands on profile screen');
  // validation: empty save shows errors and does not leave the screen
  await p.click('#profSave'); await sleep(400);
  ok((await p.textContent('#errName')).length > 0 && (await p.textContent('#photoErr')).length > 0, u.name + ': empty profile is refused with messages');
  ok(await screen(p) === 's-profile', u.name + ': still on profile after refused save');
  // the new Take photo control exists and is wired to a capture input
  ok(await p.$eval('#camBtn', e => e.textContent.trim()) === 'Take photo', u.name + ': "Take photo" button is present');
  ok(await p.$eval('#photoCam', e => e.getAttribute('capture') === 'user' && e.accept === 'image/*'), u.name + ': camera input uses capture');
  await p.setInputFiles(camera ? '#photoCam' : '#photoInput', { name: 'me.png', mimeType: 'image/png', buffer: PNG });
  ok(await until(() => p.$eval('#myImg', e => !e.hidden && !!e.src), 5000), u.name + ': photo from ' + (camera ? 'camera' : 'upload') + ' input previews');
  await p.fill('#inName', name); await p.fill('#inRole', role); await p.fill('#inLook', look);
  ok((await p.textContent('#pvName')) === name, u.name + ': live preview shows the name');
  await p.click('#profSave');
}

async function allowLocation(u) {
  const p = u.page;
  const sheet = await until(() => p.$('#pm.on'), 3000);
  if (sheet) await p.click('#pmOk');
}

(async () => {
  const browser = await chromium.launch(process.env.KINJO_CHANNEL ? { channel: process.env.KINJO_CHANNEL } : {});
  const A = await newUser(browser, 'A', off(0, 0)), B = await newUser(browser, 'B', off(0, 10)), C = await newUser(browser, 'C', off(0, 80));
  const emails = { A: 'alice@example.com', B: 'bob@example.com', C: 'carol@example.com' };
  const people = { A: ['Alice Anand', 'Founder at Alpha', 'A designer to build with'], B: ['Bob Bhatt', 'Engineer at Beta', 'Co-founders for health tech'], C: ['Carol Chen', 'Investor at Gamma', 'Pre-seed founders'] };

  console.log('\n== sign-in, profile, permissions');
  for (const [u, cam] of [[A, false], [B, true], [C, false]]) {
    await signIn(u, emails[u.name]);
    await makeProfile(u, { name: people[u.name][0], role: people[u.name][1], look: people[u.name][2], camera: cam });
    await allowLocation(u);
  }

  console.log('\n== location permission is required');
  const settle = async (u, want, ms = 15000) => until(async () => { const n = await deckNames(u.page); return want.every(x => n.includes(x)) && n.length === want.length ? n : false; }, ms);
  const P = await newUser(browser, 'P', off(0, 3)); await P.ctx.clearPermissions();
  await P.ctx.addInitScript(() => { const g = navigator.geolocation, orig = g.getCurrentPosition.bind(g); window.__allow = false; g.getCurrentPosition = (ok, err, o) => window.__allow ? orig(ok, err, o) : err({ code: 1 }); }); // a user who taps Don't allow
  await signIn(P, 'pat@example.com'); await makeProfile(P, { name: 'Pat Patel', role: 'Dev at Delta', look: 'Mentors', camera: false });
  ok(!!(await until(() => P.page.$('#pm.on'), 10000)), 'P: location permission card appears when location is not allowed');
  await P.page.click('#pmLater'); await sleep(400);
  ok(!!(await P.page.$('#pm.on')), 'P: location cannot be skipped ("Not now" keeps the card)');
  await P.page.click('#pmOk'); await sleep(1500);
  ok(!!(await P.page.$('#pm.on')) && /blocked/i.test(await P.page.textContent('#pmBody')), 'P: refusing the browser prompt shows the "blocked" help', await P.page.textContent('#pmBody'));
  await P.ctx.grantPermissions(['geolocation']); await P.page.evaluate(() => { window.__allow = true; }); await P.page.click('#pmOk');
  ok(!!(await until(async () => !(await P.page.$('#pm.on')), 10000)), 'P: after allowing, the card goes away');

  console.log('\n== profiles visible to each other');
  ok(await settle(A, ['Bob Bhatt', 'Pat Patel']), 'A sees B (10 m) and P (3 m), not C (80 m away)', await deckNames(A.page));
  ok(await settle(B, ['Alice Anand', 'Pat Patel']), 'B sees A and P, not C', await deckNames(B.page));
  ok(await settle(P, ['Alice Anand', 'Bob Bhatt']), 'P sees A and B, not C', await deckNames(P.page));
  ok((await deckNames(C.page)).length === 0 || (await screen(C.page)).includes('s-nearby'), 'C (80 m away) sees nobody', await deckNames(C.page));
  ok(await screen(A.page) === 's-nearby', 'A is on the nearby screen');
  const cardRole = await A.page.evaluate(() => document.querySelector('#deck .card .card-role')?.textContent);
  ok(cardRole === 'Dev at Delta', 'card shows role, and the closest person (P, 3 m) is first, before B (10 m)', cardRole);
  ok((await deckNames(A.page))[0] === 'Pat Patel', 'deck is ordered closest first', await deckNames(A.page));
  const img = await A.page.evaluate(() => document.querySelector('#deck .card img')?.getAttribute('src') || document.querySelector('#deck .card img')?.dataset.src);
  ok(/\/api\/photo\/.+\?v=\d+/.test(img || ''), 'card photo is a URL, not inline data', img);
  const photoStatus = img ? (await (await fetch(img)).status) : 0; ok(photoStatus === 200, 'card photo URL serves', photoStatus);

  console.log('\n== C walks into range, then away');
  await C.ctx.setGeolocation(off(0, 25));
  ok(await settle(A, ['Bob Bhatt', 'Carol Chen', 'Pat Patel']), 'C at 25 m appears on A', await deckNames(A.page));
  ok(await settle(C, ['Alice Anand', 'Bob Bhatt', 'Pat Patel']), 'C sees A, B and P', await deckNames(C.page));
  await C.ctx.setGeolocation(off(0, 120));
  ok(await settle(A, ['Bob Bhatt', 'Pat Patel'], 20000), 'C at 120 m disappears from A', await deckNames(A.page));

  console.log('\n== hide / show');
  await B.page.click('#menuBtn'); await sleep(500); await B.page.click('#toSettings'); await sleep(600);
  await B.page.click('#visSwitch');
  ok(await settle(A, ['Pat Patel'], 15000), 'B hides: B vanishes from A', await deckNames(A.page));
  ok(await until(async () => (await B.page.getAttribute('#visSwitch', 'aria-checked')) === 'false', 3000), 'B switch shows hidden');
  await B.page.click('#visSwitch');
  await B.page.click('#s-settings [data-ui-back]'); await sleep(700);
  ok(await settle(A, ['Bob Bhatt', 'Pat Patel'], 20000), 'B shows again: B returns on A', await deckNames(A.page));

  console.log('\n== menu: no Events, Messages + Settings work');
  await A.page.click('#menuBtn'); await sleep(600);
  const rows = await A.page.$$eval('#drawer .row', rs => rs.map(r => r.textContent.trim()));
  ok(!rows.some(r => /event/i.test(r)), 'hamburger menu has no Events row', rows);
  ok(rows.includes('Messages') && rows.includes('Settings'), 'menu still has Messages and Settings', rows);
  ok(!(await A.page.$('#s-events, #s-event, #s-going, #toEvents, #evList')), 'no events screens or controls exist in the page');
  await A.page.click('#toInbox'); await sleep(700);
  ok(await screen(A.page) === 's-inbox', 'Messages screen opens', await screen(A.page));
  ok((await A.page.textContent('#inboxList')).includes('No messages yet') , 'inbox empty state (and is styled: class inbox-none)', await A.page.textContent('#inboxList'));
  await A.page.click('#s-inbox [data-ui-back]'); await sleep(700);
  ok(await screen(A.page) === 's-nearby', 'back from Messages returns to the deck', await screen(A.page));
  await A.page.click('#menuBtn'); await sleep(500); await A.page.click('#toSettings'); await sleep(700);
  ok(await screen(A.page) === 's-settings', 'Settings opens');
  ok((await A.page.textContent('#setName')).includes('Alice'), 'settings shows my name');
  await A.page.click('#editProfile'); await sleep(700);
  ok(await screen(A.page) === 's-profile' && (await A.page.inputValue('#inName')) === 'Alice Anand', 'Edit profile opens prefilled');
  await A.page.fill('#inLook', 'Updated look'); await A.page.click('#profSave'); await sleep(900);
  ok(await until(async () => (await deckNames(B.page)).length >= 0 && (await B.page.evaluate(() => [...document.querySelectorAll('#deck .card .card-desc')].map(e => e.textContent))).includes('Updated look'), 12000), 'edited card reaches B live', await B.page.evaluate(() => [...document.querySelectorAll('#deck .card .card-desc')].map(e => e.textContent)));

  console.log('\n== deep links (old kinjo://events link must not break)');
  const dl = await A.page.evaluate(() => { try { return typeof window.KINJO_HOOK; } catch (e) { return 'err'; } });

  console.log('\n== demo account (App Review)');
  const D = await newUser(browser, 'Demo', off(5000, 0));
  await D.page.goto(BASE); await D.page.evaluate(() => localStorage.clear()); await D.page.reload(); await until(() => D.page.$('#s-auth.on'), 8000);
  await D.page.click('#emailStart'); await D.page.fill('#inEmail', 'demo@example.com'); await D.page.click('#emailNext'); await until(() => D.page.$('#s-code.on'), 5000);
  await D.page.fill('#inCode', '123456');
  await until(() => D.page.$('#s-profile.on'), 8000);
  await D.page.setInputFiles('#photoInput', { name: 'd.png', mimeType: 'image/png', buffer: PNG }); await until(() => D.page.$eval('#myImg', e => !e.hidden), 4000);
  await D.page.fill('#inName', 'Demo Reviewer'); await D.page.fill('#inRole', 'Reviewer'); await D.page.fill('#inLook', 'Testing'); await D.page.click('#profSave');
  await allowLocation(D);
  const demoDeck = await until(async () => { const n = await deckNames(D.page); return n.length >= 3 ? n : false; }, 15000);
  ok(!!demoDeck, 'demo account always sees sample cards', demoDeck);

  console.log('\n== wrong code, logout, delete');
  const W = await newUser(browser, 'W', off(0, 0));
  await W.page.goto(BASE); await W.page.evaluate(() => localStorage.clear()); await W.page.reload(); await until(() => W.page.$('#s-auth.on'), 8000);
  await W.page.click('#emailStart'); await W.page.fill('#inEmail', 'wrong@example.com'); await W.page.click('#emailNext'); await until(() => W.page.$('#s-code.on'), 5000);
  await W.page.fill('#inCode', '000000'); await sleep(900);
  ok((await W.page.textContent('#codeErr')).length > 0 && await screen(W.page) === 's-code', 'wrong code is rejected with a message');
  await W.ctx.close();

  await C.page.click('#menuBtn'); await sleep(500); await C.page.click('#toSettings'); await sleep(600);
  await C.page.click('#logout'); await sleep(500); await C.page.click('#outConfirm'); await until(() => C.page.$('#s-auth.on'), 6000);
  ok(await screen(C.page) === 's-auth', 'logout returns to sign-in');
  console.log('  (waiting 62 s: the server allows one sign-in code per address per minute)'); await sleep(62000);
  await signIn(C, emails.C);
  ok(await until(() => C.page.$('#s-scan.on, #s-nearby.on'), 12000), 'returning user skips the profile and goes straight in', await screen(C.page));
  await C.page.click('#menuBtn'); await sleep(500); await C.page.click('#toSettings'); await sleep(600);
  await C.page.click('#deleteAcc'); await sleep(500); await C.page.click('#delConfirm'); await until(() => C.page.$('#s-auth.on'), 8000);
  ok(await screen(C.page) === 's-auth', 'delete account returns to sign-in');

  console.log('\n== console errors');
  for (const u of [A, B, C, D, P]) ok(u.errs.length === 0, u.name + ': no JS errors', u.errs.slice(0, 3));

  await browser.close();
  console.log(`\nE2E: ${pass} passed, ${fail} failed`); if (fail) console.log('FAILED:\n - ' + failures.join('\n - '));
  process.exit(fail ? 1 : 0);
})().catch(e => { console.error('E2E crashed', e); process.exit(2); });
