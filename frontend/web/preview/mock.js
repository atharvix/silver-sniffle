/* ===== Demo backend for the iPhone preview. Same function names as Firebase, all in this page. ===== */
const DEMO_STORE = "kinjo-preview";
const mem = window.__demo.mem;
const DEVICE_ACCOUNTS = window.__demo.accounts;
const SAMPLE_PROFILES = window.__demo.people;
function persist() { try { localStorage.setItem(DEMO_STORE, JSON.stringify({ user: mem.user, docs: mem.docs })); } catch (e) {} }
function setUser(u) { mem.user = u; persist(); mem.listeners.forEach(cb => { try { cb(u); } catch (e) {} }); }
window.__demo.setUser = setUser;
window.__demo.persist = persist;

function initializeApp() { return {}; }
function getAuth() { return {}; }
function getFirestore() { return {}; }
function onAuthStateChanged(a, cb) { mem.listeners.push(cb); setTimeout(() => cb(mem.user), 40); return () => {}; }
class GoogleAuthProvider { setCustomParameters() {} static credential(t) { return { t }; } }
function signInWithRedirect() {
  return new Promise(resolve => {
    window.__demo.chooser(acct => {
      document.getElementById("googleBtn").removeAttribute("aria-busy");
      if (acct) setUser({ uid: "g_" + acct.email.split("@")[0], email: acct.email });
      resolve();
    });
  });
}
function getRedirectResult() { return Promise.resolve(null); }
async function signInWithCredential() { return null; }

async function sendSignInLinkToEmail(a, email) {
  await new Promise(r => setTimeout(r, 500));
  setTimeout(() => {
    if (window.__kinjo && window.__kinjo.current === "check") { window.__demo.note("Demo: the sign-in link was opened"); setUser({ uid: "e_" + email.replace(/\W/g, "_"), email }); }
  }, 3200);
}
function isSignInWithEmailLink() { return false; }
async function signInWithEmailLink(a, email) { setUser({ uid: "e_" + email.replace(/\W/g, "_"), email }); }
async function signOut() { setUser(null); }
async function deleteUser(u) { setUser(null); }

function doc(db, col, id) { return { col, id, key: col + "/" + id }; }
async function getDoc(ref) {
  await new Promise(r => setTimeout(r, 60));
  let d = mem.docs[ref.key];
  if (!d && ref.col === "profiles") { const s = SAMPLE_PROFILES.find(p => p.uid === ref.id); if (s) d = { name: s.name, role: s.role, look: s.desc, photo: s.img }; }
  return { exists: () => !!d, data: () => d };
}
async function setDoc(ref, data) {
  const clean = {}; for (const k in data) clean[k] = data[k] === "__ts" ? Date.now() : data[k];
  mem.docs[ref.key] = clean; if (ref.col !== "presence") persist();
  if (ref.col === "presence") { mem.snaps.forEach(f => f()); }
}
async function deleteDoc(ref) { delete mem.docs[ref.key]; persist(); if (ref.col === "presence") mem.snaps.forEach(f => f()); }
function collection(db, name) { return { name }; }
function where(f, op, v) { return { f, op, v }; }
function query(c, w) { return { c, w }; }
function serverTimestamp() { return "__ts"; }
/* Live presence simulation: sample people have real positions around you and can walk in and out.
   The query honours the "cell in [...]" filter exactly like Firestore. */
const START = { lat: 26.9124, lng: 75.7873 };
const toLL = (n, e) => ({ lat: START.lat + n / 111320, lng: START.lng + e / (111320 * Math.cos(START.lat * Math.PI / 180)) });
const others = new Map();
function cellOfM(lat, lng) { return Math.floor(lat / 0.0005) + "_" + Math.floor(lng / 0.0005); }
SAMPLE_PROFILES.forEach((p, i) => { const a = i * 0.9; const ll = toLL(p.d * Math.cos(a), p.d * Math.sin(a)); others.set(p.uid, { ...ll, acc: 10, t: Date.now() }); });
const emitAll = () => mem.snaps.forEach(f => f());
window.__demo.place = (uid, n, e, acc) => { const ll = toLL(n, e); others.set(uid, { ...ll, acc: acc ?? 10, t: Date.now() }); emitAll(); };
window.__demo.remove = uid => { others.delete(uid); emitAll(); };
window.__demo.age = (uid, ms) => { const o = others.get(uid); if (o) { o.t = Date.now() - ms; emitAll(); } };
window.__demo.beatAll = () => { others.forEach(o => { o.t = Date.now(); }); emitAll(); };
window.__demo.others = others;
function onSnapshot(q, cb) {
  const cells = new Set(q.w && q.w.v || []);
  const emit = () => {
    const list = [];
    if (!mem.nobody) others.forEach((o, uid) => { if (cells.has(cellOfM(o.lat, o.lng))) list.push({ id: uid, data: { cell: cellOfM(o.lat, o.lng), lat: o.lat, lng: o.lng, acc: o.acc, t: { toMillis: () => o.t } } }); });
    const mine = mem.user && mem.docs["presence/" + mem.user.uid];
    if (mine && cells.has(mine.cell)) list.push({ id: mem.user.uid, data: Object.assign({}, mine, { t: { toMillis: () => mine.t } }) });
    cb({ forEach(fn) { list.forEach(d => fn({ id: d.id, data: () => d.data })); } });
  };
  mem.snaps.push(emit); setTimeout(emit, 30);
  return () => { mem.snaps = mem.snaps.filter(f => f !== emit); };
}
