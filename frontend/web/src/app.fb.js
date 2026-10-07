/* ---------------- Kinjo backend (Go) ----------------
   Firebase is gone. Auth is "Continue with LinkedIn/Google" handled by our backend,
   the profile lives in Postgres behind a small REST API, and "who's within 30 m"
   is pushed live over a WebSocket. Point API at the Utho backend at build time. */
const API = (/*API_BASE*/ "http://localhost:8080").replace(/\/+$/, "");
const WS_API = API.replace(/^http/, "ws");

const $ = (id) => document.getElementById(id);
const root = document.documentElement;
/* Running inside the Kinjo phone app (Capacitor)? Then the native presence service reports location. */
const CAP = window.Capacitor;
const NATIVE = !!(CAP && CAP.isNativePlatform && CAP.isNativePlatform());
const plugin = n => (NATIVE && CAP.Plugins ? CAP.Plugins[n] : null);
if (NATIVE) root.classList.add("native");
const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
const BEAT_MS = 25e3;         // heartbeat while the app is open (browser position, socket liveness)
const MOVE_M = 4, MIN_WRITE_MS = 2500;  // write position after moving 4 m, at most every 2.5 s

/* ---------------- device storage ---------------- */
const store = {
  get(k, d) { try { const v = localStorage.getItem("kinjo:" + k); return v == null ? d : JSON.parse(v); } catch (e) { return d; } },
  set(k, v) { try { localStorage.setItem("kinjo:" + k, JSON.stringify(v)); } catch (e) {} },
  del(k) { try { localStorage.removeItem("kinjo:" + k); } catch (e) {} }
};

/* ---------------- session token + API client ----------------
   The backend hands us a session token in the redirect fragment after
   sign-in. We keep it in localStorage and send it as a Bearer header (and as the
   WebSocket ?token=). */
const tokenStore = {
  get() { try { return localStorage.getItem("kinjo:token") || ""; } catch (e) { return ""; } },
  set(t) { try { localStorage.setItem("kinjo:token", t); } catch (e) {} },
  del() { try { localStorage.removeItem("kinjo:token"); } catch (e) {} }
};
/* Thin fetch wrapper: attaches the token, sets JSON headers, and turns a lost
   session (401) into a clean sign-out signal. */
async function api(path, opts = {}) {
  const t = tokenStore.get();
  const headers = Object.assign({}, opts.headers);
  if (t) headers["Authorization"] = "Bearer " + t;
  if (opts.body != null && !headers["Content-Type"]) headers["Content-Type"] = "application/json";
  const res = await fetch(API + path, Object.assign({}, opts, { headers }));
  if (res.status === 401) { tokenStore.del(); const e = new Error("unauthorized"); e.code = "auth/unauthorized"; throw e; }
  return res;
}

/* ---------------- theme ---------------- */
let themePref = store.get("theme", "system");
const effective = () => themePref !== "system" ? themePref : (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
function applyTheme() {
  if (themePref === "system") root.removeAttribute("data-theme"); else root.setAttribute("data-theme", themePref);
  document.querySelectorAll("#themeSeg button").forEach(b => b.setAttribute("aria-checked", b.dataset.themeSet === themePref));
  const m = document.querySelector('meta[name="theme-color"]'); if (m) m.setAttribute("content", effective() === "light" ? "#ffffff" : "#0a0a0a");
}
$("themeSeg").addEventListener("click", e => { const b = e.target.closest("button"); if (!b) return; themePref = b.dataset.themeSet; store.set("theme", themePref); applyTheme(); });
try { matchMedia("(prefers-color-scheme: light)").addEventListener("change", applyTheme); } catch (e) {}
applyTheme();

/* ---------------- toast ---------------- */
let tt;
function toast(msg) { const t = $("toast"); t.textContent = msg; t.classList.add("on"); clearTimeout(tt); tt = setTimeout(() => t.classList.remove("on"), 3000); }

/* ---------------- state ---------------- */
let user = null;          // { uid, email } from the backend, or null when signed out
let profile = null;       // profiles/{uid}
const complete = p => !!(p && p.name && p.role && p.look && p.photo);
const locKey = () => "loc:" + (user ? user.uid : "");
function me() { return Object.assign({ email: user ? user.email : "" }, profile || {}); }

/* ---------------- history: phone back gesture ---------------- */
let depth = 0, overlayEntry = false, ignorePop = 0, current = null, profileMode = "create";
const hpush = () => { try { history.pushState({ kinjo: Date.now() }, ""); depth++; return true; } catch (e) { return false; } };
const hreplace = () => { try { history.replaceState({ kinjo: Date.now() }, ""); } catch (e) {} };
function hreset() { if (depth > 0) { const n = depth; depth = 0; ignorePop++; try { history.go(-n); } catch (e) { ignorePop--; } } overlayEntry = false; }
function backTarget() {
  if (current === "settings") return "nearby";
  if (current === "profile" && profileMode === "edit") return "settings";
  return null;
}
addEventListener("popstate", () => {
  if (ignorePop > 0) { ignorePop--; return; }
  depth = Math.max(0, depth - 1);
  if (anyOverlay()) { overlayEntry = false; closeAll(); return; }
  const t = backTarget(); if (t) go(t, { back: true });
});
function uiBack() { if (depth > 0 && !overlayEntry) history.back(); else { const t = backTarget(); if (t) go(t, { back: true }); } }
function uiClose() { if (overlayEntry && depth > 0) history.back(); else closeAll(); }
document.querySelectorAll("[data-ui-back]").forEach(b => b.addEventListener("click", uiBack));
document.querySelectorAll("[data-ui-close]").forEach(b => b.addEventListener("click", uiClose));

/* ---------------- navigation ---------------- */
const ORDER = ["splash", "auth", "profile", "location", "scan", "nearby", "settings"];
const ENTER = {};
function go(name, opts = {}) {
  const next = $("s-" + name); if (!next) return;
  const prev = current ? $("s-" + current) : null;
  if (prev === next) { if (ENTER[name]) ENTER[name](); return; }
  if (opts.reset) hreset();
  else if (opts.push) { if (overlayEntry) { hreplace(); overlayEntry = false; } else hpush(); }
  closeAll();
  if (prev) {
    if (prev.id === "s-splash") { prev.classList.add("lift"); setTimeout(() => prev.classList.remove("on", "lift", "run"), 1000); }
    else prev.classList.remove("on");
  }
  /* screens cross-fade in place: nothing slides sideways, so the layout never jumps */
  next.scrollTop = 0; next.scrollLeft = 0;
  next.classList.add("on");
  current = name;
  if (ENTER[name]) ENTER[name]();
}

/* Load the account + profile from the backend. Returns false if the session is
   gone (signed out), so callers can route to the auth screen. */
async function loadMe() {
  if (!tokenStore.get()) { user = null; profile = null; return false; }
  try {
    const res = await api("/api/me");
    if (!res.ok) { profile = profile || null; toast("Couldn’t load your profile. Check your connection."); return true; }
    const data = await res.json();
    user = { uid: data.uid, email: data.email || "", verified: !!data.emailVerified };
    profile = data.profile || null;
    rememberAccount();
    return true;
  } catch (e) {
    if (e.code === "auth/unauthorized") { user = null; profile = null; return false; }
    profile = profile || null; toast("Couldn’t load your profile. Check your connection.");
    return true;
  }
}
async function route(opts = {}) {
  if (!user) return go("auth", opts);
  if (profile === undefined) await loadMe();
  if (!complete(profile)) { profileMode = "create"; return go("profile", opts); }
  if (!store.get(locKey(), false)) return go("location", opts);
  startPresence();
  return go(opts.scan ? "scan" : "nearby", opts);
}

/* ---------------- overlays ---------------- */
const OVER = ["drawer", "delSheet", "outSheet", "cardViewer"];
const anyOverlay = () => OVER.some(id => $(id).classList.contains("on"));
function openOverlay(el) { $("scrim").classList.add("on"); el.classList.add("on"); el.setAttribute("aria-hidden", "false"); if (hpush()) overlayEntry = true; }
function closeAll() {
  OVER.forEach(id => { const el = $(id); el.classList.remove("on"); el.setAttribute("aria-hidden", "true"); });
  $("scrim").classList.remove("on"); $("menuBtn").setAttribute("aria-expanded", "false");
}
$("scrim").addEventListener("click", uiClose);
addEventListener("keydown", e => { if (e.key === "Escape" && anyOverlay()) uiClose(); });

function busy(btn, on) { if (on) btn.setAttribute("aria-busy", "true"); else btn.removeAttribute("aria-busy"); }

/* ---------------- 01 app open ---------------- */
let splashDone = false, authReady = false;
function runSplash() {
  const s = $("s-splash");
  s.classList.remove("run", "lift"); void s.offsetWidth;
  s.classList.add("on", "run"); current = "splash"; splashDone = false;
  setTimeout(() => { splashDone = true; maybeLeaveSplash(); }, reduce ? 200 : 2300);
}
function maybeLeaveSplash() { if (splashDone && authReady && current === "splash") route({ still: true, scan: true }); }

/* ---------------- 02 sign in ----------------
   Each button hands off to the backend's OAuth flow for its provider (LinkedIn, Google). The backend
   redirects back with a session token in the URL fragment (#token=...).
   - Web: the redirect lands on the page and boot() reads location.hash.
   - Phone app (bundled, no web origin): we open the provider in the system browser
     and the backend returns to the app via the kinjo://auth deep link, caught
     by the appUrlOpen listener below. */
const authBtns = () => document.querySelectorAll("[data-via]");
authBtns().forEach(b => b.addEventListener("click", function () { startLogin(this); }));
async function startLogin(btn) {
  if (document.querySelector("#s-auth [aria-busy]")) return;   /* one sign-in at a time */
  busy(btn, true);
  try { store.set("authPending", Date.now()); store.set("via", btn.dataset.via); } catch (e) {}
  let ch = ""; try { ch = await pkce(); } catch (e) {}
  const url = API + "/auth/" + btn.dataset.via + (ch ? "?challenge=" + ch : "");
  const Browser = plugin("Browser");   /* system browser (Custom Tab): reuses an existing sign-in; Google refuses embedded WebViews */
  if (NATIVE && Browser) {
    try { await Browser.open({ url, presentationStyle: "popover" }); }
    catch (e) { busy(btn, false); toast(authMessage()); }
    return;   /* the result arrives via the deep link */
  }
  location.href = url;
}
/* RFC 7636 PKCE between this app and our backend: only the instance that started
   the sign-in can redeem the one-time code that comes back in the deep link. */
const b64url = b => btoa(String.fromCharCode(...b)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
async function pkce() {
  const v = b64url(crypto.getRandomValues(new Uint8Array(32)));
  store.set("pkce", v);
  return b64url(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(v))));
}
/* the backend returns #code=… (PKCE), legacy #token=…, or #auth_error=… */
function authFrom(s) {
  const get = k => { const m = (s || "").match(new RegExp("[#?&]" + k + "=([^&]+)")); return m ? decodeURIComponent(m[1]) : ""; };
  return { token: get("token"), code: get("code"), err: get("auth_error"), verified: get("verified"), verifyErr: get("verify_error") };
}
/* back from the email-verification page (its "Open Kinjo" button) */
async function verifyResult(a) {
  toast(a.verified ? "Email verified ✓" : "That link expired. Send a new one from Settings.");
  if (a.verified && user) { await loadMe(); syncMe(); }
}
async function sessionFrom(a) {
  if (a.token || !a.code) return a.token;
  const verifier = store.get("pkce", ""); store.del("pkce");
  try { const r = await api("/auth/exchange", { method: "POST", body: JSON.stringify({ code: a.code, verifier }) }); if (r.ok) return (await r.json()).token; } catch (e) {}
  return "";
}
/* store a freshly-issued token, load the account, and route into the app.
   Shared by the web (hash) and native (deep link) sign-in paths. */
async function applyToken(token) {
  tokenStore.set(token); store.del("authPending");
  const ok = await loadMe(); syncMe();
  if (ok && user) {
    toast(complete(profile) ? "Welcome back, " + profile.name.split(" ")[0] : "Signed in" + (user.email ? " as " + user.email : ""));
    initPush();
    loadAds();
    route({ reset: true, scan: true });
    return true;
  }
  return false;
}
/* Google-style "Continue as …": remember who last signed in on this device. It's our
   own profile data, and we never touch the provider's session. "via" is the button they used. */
function rememberAccount() {
  if (!profile || !profile.name) return;
  const save = photo => store.set("account", { name: profile.name, photo, via: store.get("via", "linkedin") });
  if (!profile.photo) return save("");
  const im = new Image();
  im.onload = () => { try { const c = document.createElement("canvas"), s = Math.min(im.width, im.height); c.width = c.height = 96; c.getContext("2d").drawImage(im, (im.width - s) / 2, (im.height - s) / 2, s, s, 0, 0, 96, 96); save(c.toDataURL("image/jpeg", .85)); } catch (e) { save(""); } };
  im.onerror = () => save("");
  im.src = profile.photo;
}
function renderAccount() {
  const a = store.get("account", null);
  $("acct").hidden = $("acctForget").hidden = !a;
  authBtns().forEach(b => { b.lastChild.textContent = a && (a.via || "linkedin") === b.dataset.via ? "Continue as " + a.name.split(" ")[0] : b.id === "googleBtn" ? "Continue with Google" : "Continue with LinkedIn"; });
  if (a) { avatarFill($("acctAvatar"), a); $("acctName").textContent = a.name; }
  if (a && a.via === "email" && !$("inEmail").value) $("inEmail").value = store.get("lastEmail", "");
}
ENTER.auth = renderAccount;

/* email: a 6-digit code goes to any inbox and is typed here (no password, no link to
   find its way back into the app) */
let emailAddr = "";
function emailNote(msg, err) { const n = $("emailNote"); n.textContent = msg || ""; n.classList.toggle("err", !!err); }
function codeStep(on) {
  $("fEmail").hidden = on; $("fCode").hidden = $("codeLinks").hidden = !on;
  $("emailBtn").lastChild.textContent = on ? "Sign in" : "Continue with email";
  if (on) { $("inCode").value = ""; setTimeout(() => $("inCode").focus(), 60); }
}
async function sendCode() {
  const r = await api("/auth/email/start", { method: "POST", body: JSON.stringify({ email: emailAddr }) });
  if (r.ok) { codeStep(true); emailNote("We sent a code to " + emailAddr + ". It can take a minute, so check spam too."); return; }
  const e = (await r.json().catch(() => ({}))).error;
  emailNote(e === "too_soon" ? "We just sent you a code. Wait a minute before asking for another." : e === "invalid_email" ? "That doesn’t look like an email address." : "Couldn’t send the code. Try again.", true);
}
$("emailForm").addEventListener("submit", async e => {
  e.preventDefault();
  const b = $("emailBtn"); if (document.querySelector("#s-auth [aria-busy]")) return;
  busy(b, true);
  try {
    if ($("fCode").hidden) {
      emailAddr = $("inEmail").value.trim().toLowerCase();
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailAddr)) { $("fEmail").classList.add("invalid"); emailNote("Enter your email address.", true); return; }
      store.set("lastEmail", emailAddr); store.set("via", "email");
      await sendCode();
    } else {
      const code = $("inCode").value;
      if (code.length !== 6) { $("fCode").classList.add("invalid"); emailNote("Enter the 6-digit code from the email.", true); return; }
      const r = await api("/auth/email/verify", { method: "POST", body: JSON.stringify({ email: emailAddr, code }) });
      const d = await r.json().catch(() => ({}));
      if (r.ok && d.token) { codeStep(false); emailNote(""); await applyToken(d.token); return; }
      emailNote(d.error === "wrong" ? "That code isn’t right. Check the email and try again." : d.error === "expired" || d.error === "too_many" ? "That code has expired. Send a new one." : "Couldn’t sign you in. Try again.", true);
    }
  } catch (err) { emailNote("No connection. Check your internet and try again.", true); }
  finally { busy(b, false); }
});
$("codeResend").addEventListener("click", () => sendCode().catch(() => emailNote("No connection. Check your internet and try again.", true)));
$("codeBack").addEventListener("click", () => { codeStep(false); emailNote(""); $("inEmail").focus(); });
$("inEmail").addEventListener("input", () => { $("fEmail").classList.remove("invalid"); emailNote(""); });
$("inCode").addEventListener("input", function () {
  this.value = this.value.replace(/\D/g, "").slice(0, 6); $("fCode").classList.remove("invalid"); emailNote("");
  if (this.value.length === 6) $("emailForm").requestSubmit();   /* pasted or autofilled: no extra tap */
});
$("acctForget").addEventListener("click", () => { store.del("account"); renderAccount(); });
/* Clear the sign-in button's busy state. The native Browser.open resolves
   immediately, so the button would stay "busy" (and look unclickable) if the
   user returns without finishing — e.g. taps back, or sign-in errors. We reset
   it whenever the app/page regains focus or the deep link comes back. */
function resetAuthBtn() { authBtns().forEach(b => busy(b, false)); }
addEventListener("visibilitychange", () => { if (document.visibilityState === "visible") resetAuthBtn(); });
addEventListener("pageshow", resetAuthBtn);   /* web: restored from bfcache after redirect */

/* phone app: catch kinjo://auth#token=... (or #auth_error=...) from the browser */
(function () {
  const App = plugin("App");
  if (!NATIVE || !App) return;
  App.addListener("appUrlOpen", async data => {
    resetAuthBtn();
    const url = (data && data.url) || "";
    if (url.indexOf("auth") === -1) return;
    const a = authFrom(url);
    const Browser = plugin("Browser"); if (Browser) { try { await Browser.close(); } catch (e) {} }
    if (a.verified || a.verifyErr) { await verifyResult(a); return; }
    if (a.err) { toast(authMessage(a.err)); return; }
    const token = await sessionFrom(a);
    if (token) await applyToken(token); else if (a.code) toast(authMessage("session"));
  });
  /* returning to the app (even when no deep link fired) un-sticks the button */
  App.addListener("appStateChange", s => { if (s && s.isActive) resetAuthBtn(); });
  App.addListener("resume", () => { resetAuthBtn(); onResume(); });
})();
function authMessage(code) {
  const m = {
    bad_state: "Sign-in timed out. Please try again.",
    no_code: "Sign-in didn’t complete. Try again.",
    token_exchange: "Couldn’t reach the sign-in service. Check your connection and try again.",
    userinfo: "Couldn’t read your profile. Try again.",
    provider_off: "That sign-in option isn’t available yet. Use another one.",
    db_user: "Something went wrong creating your account. Try again.",
    session: "Something went wrong signing you in. Try again.",
    user_cancelled_login: "Sign-in was cancelled.",
    access_denied: "Sign-in was cancelled.",
    user_cancelled_authorize: "Sign-in was cancelled.",
    "auth/unauthorized": "Your session expired. Sign in again.",
    "auth/network": "No connection. Check your internet and try again."
  };
  return m[code] || "Sign-in didn’t work. Try again.";
}

/* ---------------- 03 profile ---------------- */
let draftPhoto = null;
const ROLE_MAX = 7, LOOK_MAX = 20;
function capWords(el, max, msg) { if (words(el.value).length > max) { el.value = el.value.trim().split(/\s+/).slice(0, max).join(" "); toast(msg); } }
const words = t => t.trim().split(/\s+/).filter(Boolean);
function renderPreview() {
  const n = $("inName").value.trim(), r = $("inRole").value.trim(), l = $("inLook").value.trim();
  $("pvName").textContent = n || "Your name"; $("pvRole").textContent = r || "What you do"; $("pvDesc").textContent = l || "What you’re looking for";
  const c = words($("inLook").value).length, rc = words($("inRole").value).length;
  $("lookCount").textContent = c + " / " + LOOK_MAX + " words"; $("lookCount").classList.toggle("over", c >= LOOK_MAX);
  $("roleCount").textContent = rc + " / " + ROLE_MAX + " words"; $("roleCount").classList.toggle("over", rc >= ROLE_MAX);
  const img = $("myImg");
  if (draftPhoto) {
    img.src = draftPhoto; img.hidden = false; $("myEmpty").hidden = true; $("photoBtn").textContent = "Change photo";
    img.onload = () => photoHint(img); if (img.complete) photoHint(img);
  }
  else { img.hidden = true; img.removeAttribute("src"); $("myEmpty").hidden = false; $("photoBtn").textContent = "Upload photo"; }
}
/* an old 100 px LinkedIn photo (or any small upload): nudge for a sharper one rather than ship a blurry card */
function photoHint(img) { if (img.naturalWidth && img.naturalWidth < 400) $("photoErr").textContent = "This photo is low resolution. Upload a sharper one so people can recognise you."; }
function clearErr(f, e) { $(f).classList.remove("invalid"); $(e).textContent = ""; }
$("inLook").addEventListener("input", function () {
  capWords(this, LOOK_MAX, "Keep it to 20 words");
  clearErr("fLook", "errLook"); renderPreview();
});
$("inName").addEventListener("input", () => { clearErr("fName", "errName"); renderPreview(); });
$("inRole").addEventListener("input", function () { capWords(this, ROLE_MAX, "Keep it to 7 words"); clearErr("fRole", "errRole"); renderPreview(); });
$("myCard").addEventListener("click", () => $("photoInput").click());
$("photoBtn").addEventListener("click", () => $("photoInput").click());
$("photoInput").addEventListener("change", function () {
  const f = this.files && this.files[0]; this.value = ""; if (!f) return;
  if (!/^image\//.test(f.type)) { $("photoErr").textContent = "Choose an image file, like a JPG or PNG."; return; }
  const url = URL.createObjectURL(f), im = new Image();
  im.onload = () => {
    const W = 540, H = Math.round(540 / 0.68), cv = document.createElement("canvas"); cv.width = W; cv.height = H;
    const ctx = cv.getContext("2d"), r = Math.max(W / im.width, H / im.height), w = im.width * r, h = im.height * r;
    ctx.drawImage(im, (W - w) / 2, (H - h) / 2.6, w, h);
    draftPhoto = cv.toDataURL("image/jpeg", 0.8); URL.revokeObjectURL(url); $("photoErr").textContent = ""; renderPreview();
  };
  im.onerror = () => { $("photoErr").textContent = "That image couldn’t be opened. Try another photo."; URL.revokeObjectURL(url); };
  im.src = url;
});
ENTER.profile = () => {
  const edit = profileMode === "edit", u = me();
  $("profBrand").hidden = edit; $("profBack").hidden = !edit;
  $("profMeta").textContent = edit ? "Edit profile" : "Step 1 of 2";
  /* first sign-in: LinkedIn/Google filled the name, so greet them and ask for the
     rest: photo, role, what they're looking for. textContent: the
     name is user data, never HTML. */
  $("profTitle").innerHTML = '<span class="mask rv"><span></span></span>';
  $("profTitle").querySelector("span span").textContent = edit ? "Edit your profile." : u.name ? "Welcome, " + u.name.trim().split(/\s+/)[0] + "." : "Create your profile.";
  $("profSaveLabel").textContent = edit ? "Save changes" : "Continue";
  $("profScroll").scrollTop = 0;
  $("inName").value = u.name || ""; $("inRole").value = u.role || ""; $("inLook").value = u.look || ""; draftPhoto = u.photo || null;
  ["fName", "fRole", "fLook"].forEach(f => $(f).classList.remove("invalid")); ["errName", "errRole", "errLook", "photoErr"].forEach(e => $(e).textContent = "");
  renderPreview();
};
$("profSave").addEventListener("click", async function () {
  const n = $("inName").value.trim(), r = $("inRole").value.trim(), l = $("inLook").value.trim(); let bad = null;
  if (!draftPhoto) { $("photoErr").textContent = "Add a photo so people nearby can recognise you."; bad = bad || $("myCard"); }
  if (!n) { $("fName").classList.add("invalid"); $("errName").textContent = "Add your name."; bad = bad || $("fName"); }
  if (!r) { $("fRole").classList.add("invalid"); $("errRole").textContent = "Add what you do, like your role and company."; bad = bad || $("fRole"); }
  if (!l) { $("fLook").classList.add("invalid"); $("errLook").textContent = "Tell people what you’re looking for."; bad = bad || $("fLook"); }
  if (bad) { bad.scrollIntoView({ behavior: reduce ? "auto" : "smooth", block: "center" }); return; }
  const b = this; busy(b, true);
  try {
    const res = await api("/api/profile", { method: "PUT", body: JSON.stringify({ name: n, role: r, look: l, photo: draftPhoto }) });
    if (!res.ok) throw new Error("save failed");
    profile = await res.json(); syncMe(); rememberAccount();   /* a new account is remembered once it has a name */
    if (profileMode === "edit") { uiBack(); toast("Profile updated"); } else route();
  } catch (e) {
    if (e.code === "auth/unauthorized") { toast("Your session expired. Sign in again."); go("auth", { reset: true }); }
    else toast("Couldn’t save your card. Check your connection and try again.");
  }
  finally { busy(b, false); }
});

/* ---------------- 04 location ---------------- */
function locDefault() {
  $("locTitle").textContent = "See who’s around you.";
  $("locSub").textContent = "Kinjo needs your location to show people within 30 metres, what they do and what they’re looking for.";
}
ENTER.location = locDefault;
function denied(msg) {
  $("locTitle").textContent = "Location is off.";
  $("locSub").textContent = msg || "Without it Kinjo can’t show anyone nearby. You can allow it whenever you’re ready.";
}
$("allowLoc").addEventListener("click", function () {
  const b = this;
  if (NATIVE) { allowNative(b); return; }
  if (!navigator.geolocation) { denied("This browser can’t share location. Try Chrome or Safari on your phone."); return; }
  busy(b, true);
  navigator.geolocation.getCurrentPosition(p => {
    busy(b, false); store.set(locKey(), true); onPos(p); startPresence(); go("scan");
  }, err => {
    busy(b, false);
    if (err.code === 1) denied("Location is blocked for Kinjo. Allow it in your browser or phone settings, then tap Allow location again.");
    else toast("Couldn’t find your location. Check that location services are on.");
  }, { enableHighAccuracy: true, timeout: 15000, maximumAge: 30000 });
});
$("notNow").addEventListener("click", () => denied());
async function allowNative(b) {
  const K = presence(); if (!K) return;
  busy(b, true);
  let p = { location: "denied" };
  try { p = await K.requestPermissions({ permissions: ["location"] }); } catch (e) {}
  busy(b, false);
  if (p.location !== "granted") { denied("Location is blocked for Kinjo. Allow it in your phone settings, then tap Allow location again."); return; }
  store.set(locKey(), true); startPresence(); go("scan");
}

/* ---------------- presence + matching ----------------
   The server owns matching: it keeps everyone's latest position and pushes this phone
   the people within 30 m over a WebSocket. Who reports our position:
   - phone app: the native KinjoPresence service, every 5–30 s, whether the app is open,
     closed, or the phone just rebooted (Android shows "You're visible…" while it runs);
   - browser: watchPosition while the tab is open, sent over the socket.
   The server forgets a position 2 minutes after its last fix, so a phone that stops
   reporting leaves everyone's deck instead of lingering at an old spot. */
let watchId = null, pos = null, located = false, lastWrite = 0, lastWritePos = null, beat = null, writeTimer = 0;
let ws = null, wsRetry = 0, wsAt = 0, wsMsgAt = 0;
const GEO = { enableHighAccuracy: true, maximumAge: 0, timeout: 30000 };   /* never a cached fix: the server dates fixes by when they were taken */
const presence = () => plugin("KinjoPresence");

const visKey = () => "visible:" + (user ? user.uid : "");
const isVisible = () => store.get(visKey(), true) !== false;

/* straight-line distance in metres — used only to throttle position reports */
function dist(a, b) {
  const R = 6371000, t = Math.PI / 180, dLa = (b.lat - a.lat) * t, dLo = (b.lng - a.lng) * t;
  const x = Math.sin(dLa / 2) ** 2 + Math.cos(a.lat * t) * Math.cos(b.lat * t) * Math.sin(dLo / 2) ** 2;
  return 2 * R * Math.asin(Math.sqrt(x));
}

/* ---- live channel: send pos | hide | list | ping, receive nearby | notif | pong ---- */
function wsSend(obj) { if (ws && ws.readyState === 1) { try { ws.send(JSON.stringify(obj)); } catch (e) {} } }
/* fresh: drop the current socket and dial again. After screen-off the old one can be dead
   without knowing it; the server sends the nearby list as soon as a socket connects. */
function connectWS(fresh) {
  if (demoMode() || !user) return;
  const t = tokenStore.get(); if (!t) return;
  if (fresh && ws && Date.now() - wsAt > 2000) { const old = ws; ws = null; try { old.close(); } catch (e) {} }
  if (ws && (ws.readyState === 0 || ws.readyState === 1)) return;   /* connecting or open */
  let s;
  try { s = ws = new WebSocket(WS_API + "/ws", ["kinjo", t]); wsAt = Date.now(); }   /* token rides in a header, not the URL (logs) */
  catch (e) { ws = null; return; }
  s.onopen = () => { wsRetry = 0; wsMsgAt = Date.now(); if (pos) sendPos(true); };
  s.onmessage = e => {
    if (ws !== s) return;   /* a replaced socket's late messages */
    wsMsgAt = Date.now();
    try {
      const m = JSON.parse(e.data);
      if (m.type === "nearby") { organic = m.people || []; setPeople(withAds(organic)); }
      else if (m.type === "notif" && m.notification) toast(m.notification.title + (m.notification.body ? ": " + m.notification.body : ""));
    } catch (err) {}
  };
  s.onclose = () => {
    if (ws !== s) return;   /* replaced on purpose: the new socket owns the deck */
    ws = null;
    if (!demoMode()) { organic = []; setPeople([]); }   /* offline = no live list; don't keep showing the last one */
    /* reconnect while presence is on: exponential backoff (1 s → 30 s) with jitter, so a
       server restart isn't hit by every phone at the same instant */
    if (beat) setTimeout(connectWS, Math.min(30e3, 1e3 * 2 ** wsRetry++) * (0.5 + Math.random()));
  };
  s.onerror = () => { try { s.close(); } catch (e) {} };
}

function startPresence() {
  if (!user || !isVisible()) return;
  connectWS();
  if (!beat) beat = setInterval(tick, BEAT_MS);
  if (NATIVE) startNative();
  else if (navigator.geolocation && watchId == null) watchId = navigator.geolocation.watchPosition(onPos, onPosErr, GEO);
}
/* heartbeat while the app is open: keep a browser's position fresh, and replace a socket
   that died quietly (the server answers every ping; silence means it's gone) */
function tick() {
  /* browsers often report only on movement: someone standing still would go stale, so ask for a fresh fix */
  if (!NATIVE && pos && Date.now() - pos.at > 20e3 && navigator.geolocation) navigator.geolocation.getCurrentPosition(onPos, () => {}, GEO);
  sendPos(true);
  if (!ws || ws.readyState !== 1) return;
  if (Date.now() - wsMsgAt > 2 * BEAT_MS + 5e3) connectWS(true);
  else wsSend({ type: "ping" });
}

/* phone app: (re)start the native service; it keeps running after the app closes */
async function startNative() {
  const K = presence(); if (!K) return;
  try {
    if ((await K.checkPermissions()).location !== "granted") { onPosErr({ code: 1 }); return; }   /* turned off in settings */
    await K.start({ api: API, token: tokenStore.get() });
    if (!store.get("setupDone", false)) { store.set("setupDone", true); await setupAndroid(K); }
  } catch (e) {}
}
/* first run on a phone: the two settings that keep a card visible with the app closed */
async function setupAndroid(K) {
  if (!(await K.background()).granted && await ask("Stay visible when Kinjo is closed",
    "<p>Kinjo shows your card to people within 30 m <b>even when the app is closed or your screen is off</b>. To do that it uses your location in the background, only to find who is near you. Nobody ever sees your exact location, and you can hide your card any time in Settings.</p><p>On the next screen choose <b>Allow all the time</b>.</p>",
    "Continue")) await K.background({ ask: true });
  if (await ask("Don’t let your phone stop Kinjo",
    "<p>Some phones stop apps in the background to save battery. When that happens, people nearby stop seeing you.</p><ol><li>Tap <b>Open settings</b>.</li><li>Tap <b>Battery</b> (on some phones <b>App battery usage</b>).</li><li>Choose <b>Unrestricted</b> (or <b>Don’t optimise</b> / <b>No restrictions</b>).</li><li>Xiaomi, Redmi, POCO: also turn on <b>Autostart</b> on the same page.</li></ol><p>Then come back to Kinjo.</p>",
    "Open settings")) K.openSettings();
}
/* one modal for setup steps; resolves true on the main button, false on Not now / back */
function ask(title, html, ok) {
  const d = $("dlg");
  $("dlgTitle").textContent = title; $("dlgBody").innerHTML = html; $("dlgOk").textContent = ok;   /* html: our own constants only */
  return new Promise(res => {
    const done = v => { d.onclose = null; d.close(); res(v); };
    $("dlgOk").onclick = () => done(true);
    $("dlgLater").onclick = () => done(false);
    d.onclose = () => res(false);
    d.showModal();
  });
}
if (NATIVE && presence()) presence().addListener("fix", () => {
  located = true;
  if (current === "scan") $("scanState").textContent = "Location found";
});

/* browser: a new fix from watchPosition */
function onPos(p) {
  /* unknown accuracy stays 0: the server won't match on a fix it can't trust */
  pos = { lat: p.coords.latitude, lng: p.coords.longitude, acc: p.coords.accuracy || 0, at: p.timestamp || Date.now() };
  located = true;
  sendPos(false);
  if (current === "scan") $("scanState").textContent = "Location found";
}
function onPosErr(err) { if (err.code === 1) { store.set(locKey(), false); if (current === "nearby" || current === "scan") { stopPresence(false); go("location"); denied("Location was turned off. Allow it again to see people nearby."); } } }

/* browser: report position over the socket, throttled: skip tiny moves, and never send
   more than once per MIN_WRITE_MS (queue the latest spot when moving fast) */
function sendPos(force) {
  if (!user || !pos || !isVisible() || document.visibilityState === "hidden") return;
  const now = Date.now(), moved = lastWritePos ? dist(lastWritePos, pos) : 1e9;
  if (!force && moved < MOVE_M && now - lastWrite < BEAT_MS) return;
  if (now - lastWrite < MIN_WRITE_MS) {         /* moving fast: send the latest spot a moment later */
    clearTimeout(writeTimer); writeTimer = setTimeout(() => sendPos(true), MIN_WRITE_MS - (now - lastWrite) + 20); return;
  }
  if (!ws || ws.readyState !== 1) { connectWS(); return; }   /* sent on open */
  /* age lets the server date the fix itself: a heartbeat re-sending an old fix can't look fresh */
  wsSend({ type: "pos", lat: +pos.lat.toFixed(6), lng: +pos.lng.toFixed(6), acc: Math.round(Math.min(pos.acc, 999)), age: Math.max(0, now - pos.at) });
  lastWrite = now; lastWritePos = { lat: pos.lat, lng: pos.lng };
}

/* stop sharing: hidden, signed out, or account deleted (also stops the phone's service) */
async function stopPresence(removeDoc = true) {
  if (watchId != null) { navigator.geolocation.clearWatch(watchId); watchId = null; }
  if (presence()) { try { await presence().stop(); } catch (e) {} }
  clearInterval(beat); beat = null; clearTimeout(writeTimer);
  if (removeDoc && user) wsSend({ type: "hide" });   /* tell the server to drop us now */
  if (ws) { const s = ws; ws = null; try { s.close(); } catch (e) {} }
  pos = null; located = false; lastWrite = 0; lastWritePos = null;
}
/* back from screen-off/background */
function onResume() {
  if (drag) endDrag();   /* backgrounded mid-swipe: no pointerup ever comes, and the deck would hold every update */
  flushPeople();
  if (!user || !isVisible() || !store.get(locKey(), false)) return;
  connectWS(true); startPresence(); sendPos(true);
}
document.addEventListener("visibilitychange", () => { if (document.visibilityState === "visible") onResume(); });

/* ask the server for a fresh list (reopening the nearby screen, "look again") */
function refreshNearby() {
  if (demoMode()) { setPeople(withAds(DEMO.slice())); return; }
  if (!isVisible()) { setPeople([]); return; }
  if (ws && ws.readyState === 1) wsSend({ type: "list" }); else connectWS();   /* a new socket gets the list on connect */
}
const recompute = refreshNearby;   /* alias kept for the KINJO_HOOK test surface */

/* sample people, shown only with #demo in the address */
const DEMO = [
  { uid: "d1", name: "Elena Rostova", role: "ML Engineer @ Anthropic", desc: "Exploring real-time multimodal LLMs for mobile devices", img: "img/p3.jpg" },
  { uid: "d2", name: "Marco Reyes", role: "Founder @ Loop Health", desc: "Building preventive care for small clinics. Looking for a technical co-founder.", img: "img/p2.jpg" },
  { uid: "d3", name: "Noah Bennett", role: "Brand Designer, independent", desc: "Looking for early-stage teams who need an identity that feels like them.", img: "img/p4.jpg" },
  { uid: "d4", name: "Amara Okafor", role: "Associate @ Northfield Ventures", desc: "Meeting pre-seed founders in climate and fintech this month.", img: "img/p5.jpg" },
  { uid: "d5", name: "Daniel Ferreira", role: "Backend Engineer @ Kite", desc: "Open to joining an early climate team. Coffee’s on me.", img: "img/p6.jpg" },
  { uid: "d6", name: "Sophie Laurent", role: "Product Manager @ Fieldnote", desc: "Working on B2B pricing. Want to trade learnings with other PMs and founders.", img: "img/p7.jpg" },
  { uid: "d7", name: "James Carter", role: "Startup Lawyer @ Hale & Co", desc: "Helping founders get their seed paperwork right. Ask me anything.", img: "img/p8.jpg" }
];
const demoMode = () => location.hash === "#demo";
addEventListener("hashchange", () => { if (demoMode()) setPeople(withAds(DEMO.slice())); else { people = []; setPeople([]); recompute(); } });

/* scanning */
let scanT;
ENTER.scan = () => {
  clearTimeout(scanT); const t0 = performance.now(), el = $("scanCount");
  $("scanState").textContent = located ? "Location found" : "Finding your location";
  (function tick(n) { const p = Math.min(1, Math.max(0, (n - t0 - 300) / 1700)); el.textContent = Math.round(p * 30) + " m"; if (p < 1 && current === "scan") requestAnimationFrame(tick); })(t0);
  const t1 = Date.now();
  (function wait() {
    if (current !== "scan") return;
    const waited = Date.now() - t1;
    if ((located && waited > 2300) || waited > 12000) { go("nearby"); if (!located) toast("Still finding your location. Keep location on."); return; }
    scanT = setTimeout(wait, 300);
  })();
};

/* ---------------- 05 nearby: swipe deck ----------------
   A fixed photo stack, in order:
   drag left  -> the top card slides away and tucks behind; the next person is underneath.
   drag right -> the last swiped person comes back from the left pile; the dragged card tucks back in on the right.
   Swiped people pile up on the left (last two visible), upcoming people peek out on the right.
   First card: can't go right. Last card: can't go left (the card just stretches and springs back). */
const deck = $("deck");
let people = [], cards = [], idx = 0, deckBusy = false, drag = null, timers = [], raf = 0;
const POS = [
  { x: 0, y: 0, r: 0, s: 1, b: 1, o: 1 },
  { x: 12, y: 3, r: 4, s: .955, b: .5, o: 1 },
  { x: 22, y: 7, r: 8, s: .91, b: .32, o: 1 },
  { x: 28, y: 9, r: 10, s: .88, b: .25, o: 0 }
];
/* upcoming people peek out on the right; people already swiped pile up on the left (last two visible) */
const mirror = p => ({ x: -p.x, y: p.y, r: -p.r, s: p.s, b: p.b, o: p.o });
const P = i => (i < 0 ? mirror(POS[Math.min(-i, 3)]) : POS[Math.min(i, 3)]);
const EASE = "cubic-bezier(.22,1,.36,1)", EASE_OUT = "cubic-bezier(.2,.8,.25,1)", SPRING = "cubic-bezier(.25,1.35,.45,1)";
const W = () => deck.clientWidth || 250;
const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
const lerp = (a, b, t) => a + (b - a) * t;
const mix = (a, b, t) => ({ x: lerp(a.x, b.x, t), y: lerp(a.y, b.y, t), r: lerp(a.r, b.r, t), s: lerp(a.s, b.s, t), b: lerp(a.b, b.b, t), o: lerp(a.o, b.o, t) });
/* the dimming is a black layer inside the card (cheap to animate), not a CSS filter */
function setPos(el, p) {
  el.style.transform = `translate3d(${p.x.toFixed(2)}px,${p.y.toFixed(2)}px,0) rotate(${p.r.toFixed(3)}deg) scale(${p.s.toFixed(4)})`;
  el.style.opacity = p.o; el._shade.style.opacity = (1 - p.b).toFixed(3);
}
function trans(el, d, e) {
  const t = d ? `transform ${d}s ${e},opacity ${Math.min(d, .32)}s ease` : "none";
  el.style.transition = t; el._shade.style.transition = d ? `opacity ${d}s ${e}` : "none";
}
function place(el, p) { trans(el, 0); setPos(el, p); }
function anim(el, p, dur, ease) { trans(el, reduce ? 0 : dur, ease || EASE); setPos(el, p); }
const later = (fn, ms) => timers.push(setTimeout(fn, reduce ? 0 : ms));
const dragPose = (dx, dy) => ({ x: dx, y: dy * .1 + Math.abs(dx) * .025, r: clamp(dx / W() * 12, -12, 12), s: 1, b: 1, o: 1 });
const OUT_L = () => ({ x: -W() * .66, y: 12, r: -9, s: .96, b: .85, o: 1 });
const OUT_R = () => ({ x: W() * .62, y: 12, r: 10, s: .98, b: 1, o: 1 });
/* rubber band at either end of the stack */
const band = dx => { const m = W() * .22; return Math.sign(dx) * m * (1 - 1 / (Math.abs(dx) / m * .55 + 1)); };

/* ---------------- native advertising (sponsored cards) ----------------
   Sponsored brand cards are interleaved into the organic feed by the backend-
   driven placement engine: one after every `adGap` organic cards, never two in
   a row, organic always first. They share the profile-card visual language and
   are clearly labelled. Ad impressions/clicks are tracked separately. */
let ads = [], adGap = 12, organic = [];
const adImpressed = new Set();
function withAds(list) {
  if (!ads.length || !list || !list.length) return list || [];
  const out = []; let n = 0;
  for (let i = 0; i < list.length; i++) {
    out.push(list[i]);
    if ((i + 1) % adGap === 0 && i !== list.length - 1) {   /* not after the last card */
      const ad = ads[n % ads.length];
      out.push({ type: "sponsored", uid: "ad:" + ad.id + ":" + n, ad });
      n++;
    }
  }
  return out;
}
async function loadAds() {
  try {
    const r = await api("/api/ads"); if (!r.ok) return;
    const d = await r.json();
    ads = d.ads || [];
    const lo = d.frequencyMin || 10, hi = Math.max(d.frequencyMax || 15, d.frequencyMin || 10);
    adGap = lo + Math.floor(Math.random() * (hi - lo + 1));   /* fixed per session, stable placement */
    if (!demoMode() && current === "nearby") setPeople(withAds(organic));
  } catch (e) {}
}
function adEvent(id, ev) { try { api("/api/ad-event", { method: "POST", body: JSON.stringify({ adId: id, event: ev }) }).catch(() => {}); } catch (e) {} }
function openAd(id, url, ev) {
  adEvent(id, ev);
  if (!url) return;
  let target = String(url).trim();
  if (!target) return;
  if (!/^https?:\/\//i.test(target)) target = "https://" + target;
  const Browser = plugin("Browser");
  if (NATIVE && Browser) {
    try { Browser.open({ url: target }); }
    catch (e) { window.open(target, "_blank", "noopener,noreferrer"); }
  } else {
    try {
      const win = window.open(target, "_blank", "noopener,noreferrer");
      if (!win) window.location.href = target;
    } catch (e) {
      window.location.href = target;
    }
  }
}
/* fire one impression the first time an ad card reaches the front of the stack */
function trackFront() {
  const el = cards[idx]; if (!el) return;
  const adId = el.dataset.adId, uid = el.dataset.uid;
  if (adId && uid && !adImpressed.has(uid)) { adImpressed.add(uid); adEvent(+adId, "impression"); }
}
function makeAdCard(p) {
  const ad = p.ad;
  const c = document.createElement("article"); c.className = "card card--ad";
  c.dataset.uid = p.uid; c.dataset.adId = ad.id; c.dataset.destUrl = ad.destinationUrl || "";
  const img = document.createElement("img"); img.alt = ""; img.src = ad.image || ad.logo || ""; img.decoding = "async"; img.draggable = false;
  const shade = document.createElement("i"); shade.className = "shade";
  const badge = document.createElement("span"); badge.className = "ad-badge"; badge.textContent = "Sponsored";
  const tx = document.createElement("div"); tx.className = "card-text";
  tx.innerHTML = '<p class="card-name"></p><p class="card-role"></p><p class="card-desc"></p>';
  const meta = [ad.category, ad.location].filter(Boolean).join(" · ");
  tx.children[0].textContent = ad.brandName || "";
  tx.children[1].textContent = ad.description || meta;
  tx.children[2].textContent = ad.campaignMessage || "";
  const cta = document.createElement("button"); cta.type = "button"; cta.className = "ad-cta";
  cta.textContent = ad.ctaLabel || "Learn More";
  cta.addEventListener("click", ev => { ev.stopPropagation(); openAd(ad.id, ad.destinationUrl, "cta_click"); });
  tx.appendChild(cta);
  c.append(img, shade, badge, tx); c._shade = shade;
  c.setAttribute("aria-label", "Sponsored: " + (ad.brandName || "") + ". " + (ad.description || ""));
  return c;
}

function makeCard(p) {
  if (p.type === "sponsored") return makeAdCard(p);
  const c = document.createElement("article"); c.className = "card"; c.dataset.uid = p.uid;
  const img = document.createElement("img"); img.alt = ""; img.src = p.img; img.decoding = "async"; img.draggable = false;
  const shade = document.createElement("i"); shade.className = "shade";
  const tx = document.createElement("div"); tx.className = "card-text";
  tx.innerHTML = '<p class="card-name"></p><p class="card-role"></p><p class="card-desc"></p>';
  tx.children[0].textContent = p.name; tx.children[1].textContent = p.role; tx.children[2].textContent = p.desc;
  c.append(img, tx, shade); c._shade = shade; c.setAttribute("aria-label", `${p.name}, ${p.role}. ${p.desc}`);
  return c;
}
/* People come and go while you swipe. Changes are applied in place, never by rebuilding the deck:
   - someone who leaves the circle fades out of the stack (if it was the card in front, the next one steps up)
   - someone who arrives slides in at the end of the upcoming pile, so the order you're swiping never jumps
   - while a finger is on the deck or a swipe is animating, the change waits and applies right after. */
let pendingPeople = null;
function setPeople(list) {
  if (demoMode() && list !== DEMO) list = DEMO.slice();
  if (drag || deckBusy) { pendingPeople = list; return; }
  pendingPeople = null;
  const next = new Map(list.map(p => [p.uid, p]));
  if (!people.length || !cards.length) {                 /* first people found: build the stack */
    people = list.slice(); idx = 0;
    deck.innerHTML = ""; cards = people.map(makeCard);
    cards.slice().reverse().forEach(c => deck.appendChild(c));
    cards.forEach((el, i) => place(el, Object.assign({}, P(i), { o: 0, s: P(i).s * .94, y: P(i).y + 14 })));
    void deck.offsetWidth; layout(true, .6); updateCount(); return;
  }
  /* 1. who left */
  const gone = [];
  people.forEach((p, i) => { if (!next.has(p.uid)) gone.push(i); });
  /* 2. who stayed: refresh their details */
  people.forEach(p => { const n = next.get(p.uid); if (n) { if (n.img !== p.img || n.name !== p.name || n.role !== p.role || n.desc !== p.desc) refreshCard(p, n); } });
  /* 3. who arrived (closest first), added after everyone already in the stack */
  const known = new Set(people.map(p => p.uid));
  const arrived = list.filter(p => !known.has(p.uid));
  if (!gone.length && !arrived.length) { updateCount(); return; }
  if (gone.length) {
    const before = gone.filter(i => i < idx).length;
    const goneSet = new Set(gone);
    gone.forEach(i => leaveCard(cards[i]));
    people = people.filter((_, i) => !goneSet.has(i));
    cards = cards.filter((_, i) => !goneSet.has(i));
    idx = Math.max(0, Math.min(idx - before, people.length - 1));
  }
  arrived.forEach(p => {
    const el = makeCard(p); deck.insertBefore(el, deck.firstChild);
    people.push(p); cards.push(el);
    const rel = cards.length - 1 - idx;
    place(el, Object.assign({}, P(rel), { o: 0, x: P(rel).x + 26, s: P(rel).s * .96 }));
  });
  updateCount();
  if (people.length) { void deck.offsetWidth; layout(true, .55); }
}
function refreshCard(p, n) {
  if (p.type === "sponsored") return;   /* ad content is static for the card's lifetime */
  Object.assign(p, n);
  const el = cards[people.indexOf(p)]; if (!el) return;
  el.querySelector("img").src = n.img;
  const t = el.querySelector(".card-text").children; t[0].textContent = n.name; t[1].textContent = n.role; t[2].textContent = n.desc;
  el.setAttribute("aria-label", `${n.name}, ${n.role}. ${n.desc}`);
}
function leaveCard(el) {
  if (!el) return;
  el.style.zIndex = 5; el.style.pointerEvents = "none";
  const t = el.style.transform || "";
  trans(el, reduce ? 0 : .38, EASE);
  el.style.opacity = 0; el.style.transform = t + " translate3d(0,18px,0) scale(.9)";
  setTimeout(() => el.remove(), reduce ? 0 : 420);
}
function flushPeople() { if (pendingPeople && !drag && !deckBusy) setPeople(pendingPeople); }
function updateCount() { $("s-nearby").classList.toggle("nobody", people.length === 0); }
function stackZ() {
  cards.forEach((el, i) => { const rel = i - idx; el.style.zIndex = 20 - Math.min(Math.abs(rel), 18); el.tabIndex = rel === 0 ? 0 : -1; el.setAttribute("aria-hidden", rel !== 0); });
  trackFront();
}
function layout(animate, dur, ease) {
  stackZ();
  cards.forEach((el, i) => { const rel = i - idx; if (animate) anim(el, P(rel), dur || .5, ease); else place(el, P(rel)); });
}
/* finish any running swipe instantly, so a quick second swipe never waits */
function settle() { timers.forEach(clearTimeout); timers = []; if (deckBusy) { deckBusy = false; layout(false); } }
const hasNext = () => idx < people.length - 1, hasPrev = () => idx > 0;

deck.addEventListener("pointerdown", e => {
  if (e.button > 0 || drag) return;
  if (!e.target.closest(".card")) return;
  if (e.target.closest(".ad-cta")) return;   /* let the CTA's own click fire, don't start a drag */
  settle();
  const el = cards[idx]; if (!el) return;
  drag = { el, x0: e.clientX, y0: e.clientY, dx: 0, dy: 0, lx: e.clientX, lt: performance.now(), vx: 0, id: e.pointerId, side: 0, moved: false, px: e.clientX, py: e.clientY };
  try { deck.setPointerCapture(e.pointerId); } catch (err) {}
  el.classList.add("dragging"); el.style.zIndex = 30;
  cards.forEach(c => trans(c, 0));
});
function paint() {
  raf = 0; if (!drag) return;
  const w = W(), d = drag;
  let dx = d.px - d.x0; const dy = d.py - d.y0;
  const side = dx < 0 ? -1 : 1;
  const allowed = side < 0 ? hasNext() : hasPrev();
  if (!allowed) dx = band(dx);
  d.dx = dx; d.dy = dy; d.allowed = allowed;
  setPos(d.el, dragPose(dx, allowed ? dy : dy * .3));
  if (side !== d.side) {
    stackZ(); d.el.style.zIndex = 30;
    cards.forEach((el, i) => { if (i !== idx) setPos(el, P(i - idx)); });
    d.side = side;
  }
  if (!allowed) return;
  const k = clamp(Math.abs(dx) / (w * .62), 0, 1);
  if (side < 0) {
    /* everything shifts one step left: next card comes forward, the left pile moves back */
    for (let i = Math.max(0, idx - 3); i < cards.length && i <= idx + 4; i++) if (i !== idx) setPos(cards[i], mix(P(i - idx), P(i - idx - 1), k));
  } else {
    /* everything shifts one step right: the last swiped card comes forward from the left pile */
    const pc = cards[idx - 1]; pc.style.zIndex = 25;
    for (let i = Math.max(0, idx - 4); i < cards.length && i <= idx + 3; i++) if (i !== idx) setPos(cards[i], mix(P(i - idx), P(i - idx + 1), k));
  }
}
deck.addEventListener("pointermove", e => {
  const d = drag; if (!d || e.pointerId !== d.id) return;
  if (!d.moved && Math.abs(e.clientX - d.x0) < 3) return;
  d.moved = true;
  const now = performance.now(), dt = now - d.lt;
  if (dt > 0) d.vx = .7 * ((e.clientX - d.lx) / dt) + .3 * d.vx;
  d.lx = e.clientX; d.lt = now; d.px = e.clientX; d.py = e.clientY;
  if (!raf) raf = requestAnimationFrame(paint);
});
function endDrag(e) {
  if (!drag || (e && e.pointerId !== drag.id)) return;
  if (raf) { cancelAnimationFrame(raf); raf = 0; paint(); }
  const d = drag; drag = null; d.el.classList.remove("dragging");
  if (!d.moved) {
    stackZ();
    const adId = d.el.dataset.adId;           /* a tap on a sponsored card opens it */
    if (adId) openAd(+adId, d.el.dataset.destUrl, "click");
    flushPeople(); return;
  }
  if (performance.now() - d.lt > 90) d.vx = 0;          /* finger stopped before lifting: no flick */
  const w = W(), far = Math.abs(d.dx) > w * .3, flick = Math.abs(d.vx) > .3 && Math.abs(d.dx) > 14 && Math.sign(d.vx) === Math.sign(d.dx);
  if (d.allowed && (far || flick) && d.dx < 0) next({ dragged: true, dx: d.dx, v: Math.abs(d.vx) });
  else if (d.allowed && (far || flick) && d.dx > 0) prev({ dragged: true, v: Math.abs(d.vx) });
  else { layout(true, .6, d.allowed ? EASE : SPRING); setTimeout(flushPeople, 620); }
}
deck.addEventListener("pointerup", endDrag);
deck.addEventListener("pointercancel", endDrag);
deck.addEventListener("lostpointercapture", endDrag);
deck.addEventListener("dragstart", e => e.preventDefault());
function swiped() { $("swipeHint").classList.add("gone"); store.set("swiped", true); if (navigator.vibrate) { try { navigator.vibrate(6); } catch (err) {} } }
/* a small nudge when you try to go past either end with the keyboard or trackpad */
function nudge(dir) {
  settle(); const el = cards[idx]; if (!el) return;
  anim(el, { x: dir * 14, y: 0, r: dir * 1.5, s: 1, b: 1, o: 1 }, .16, EASE_OUT);
  later(() => anim(el, P(0), .5, SPRING), 150);
}

/* next: the top card slides off to the left, then tucks in behind the stack */
function next(o = {}) {
  if (!hasNext()) { if (o.dragged) layout(true, .6, SPRING); else nudge(-1); return; }
  settle(); deckBusy = true; swiped();
  const el = cards[idx], fast = clamp(o.v || 0, 0, 2.5);
  const d1 = clamp(.34 - fast * .06, .2, .34);
  idx++; stackZ();
  el.style.zIndex = o.dragged && Math.abs(o.dx || 0) >= W() * .3 ? 19 : 30;
  anim(el, OUT_L(), d1, EASE_OUT);
  cards.forEach((c, i) => { if (c !== el) anim(c, P(i - idx), .52); });
  later(() => {
    el.style.zIndex = 19; anim(el, P(-1), .5);      /* tucks in as the newest card on the left pile */
    later(() => { deckBusy = false; stackZ(); flushPeople(); }, 520);
  }, d1 * 1000 - 20);
}
/* previous: the card before shows up underneath, the current card tucks in behind it */
function prev(o = {}) {
  if (!hasPrev()) { if (o.dragged) layout(true, .6, SPRING); else nudge(1); return; }
  settle(); deckBusy = true; swiped();
  const el = cards[idx], pc = cards[idx - 1];
  idx--;
  const finish = () => {
    stackZ();
    anim(pc, P(0), .44);
    anim(el, P(1), .6);
    cards.forEach((c, i) => { if (c !== el && c !== pc) anim(c, P(i - idx), .52); });
    later(() => { deckBusy = false; flushPeople(); }, 620);
  };
  if (o.dragged) { finish(); return; }
  pc.style.zIndex = 25; anim(pc, mix(P(-1), P(0), .6), .26, EASE_OUT);
  el.style.zIndex = 30; anim(el, OUT_R(), .26, EASE_OUT);
  later(finish, 240);
}
addEventListener("keydown", e => {
  if (current !== "nearby" || anyOverlay() || !people.length || e.target.matches("input,textarea")) return;
  if (e.key === "ArrowLeft") { e.preventDefault(); next(); } else if (e.key === "ArrowRight") { e.preventDefault(); prev(); }
});
let wheelAcc = 0, wheelLock = 0;
deck.addEventListener("wheel", e => {
  if (Math.abs(e.deltaX) <= Math.abs(e.deltaY)) return;
  e.preventDefault();
  const now = performance.now(); if (now < wheelLock) return;
  wheelAcc += e.deltaX;
  if (Math.abs(wheelAcc) > 60) { if (wheelAcc > 0) next({ v: .6 }); else prev(); wheelAcc = 0; wheelLock = now + 520; }
}, { passive: false });
(function wrapWords(el) { el.innerHTML = el.textContent.split(" ").map((w, i) => `<span class="w" style="transition-delay:${.35 + i * .084}s">${w}</span>`).join(" "); })($("nobodyWords"));
/* show / hide my profile */
function syncVisibility() {
  const v = isVisible();
  $("visSwitch").setAttribute("aria-checked", String(v));
  $("visNote").textContent = v ? "People within 30 m can see your card." : "Hidden. Nobody sees you, and you don’t see anyone.";
  $("s-nearby").classList.toggle("hidden-mode", !v);
}
async function setVisible(v) {
  store.set(visKey(), v); syncVisibility();
  if (v) { startPresence(); toast("You’re visible to people nearby"); }
  else { await stopPresence(true); setPeople([]); toast("Your profile is hidden"); }
}
$("visSwitch").addEventListener("click", () => setVisible(!isVisible()));
$("visOn").addEventListener("click", () => setVisible(true));
ENTER.nearby = () => {
  syncVisibility();
  if (demoMode()) setPeople(withAds(DEMO.slice())); else { updateCount(); recompute(); }
  $("swipeHint").classList.toggle("gone", !!store.get("swiped", false));
  if (user && !pushRegistered) initPush();
};
$("lookAgain").addEventListener("click", () => { refreshNearby(); go("scan"); });

/* ---------------- 06 menu ---------------- */
function avatarFill(el, u) {
  el.innerHTML = "";
  if (u.photo) { const i = document.createElement("img"); i.alt = ""; i.src = u.photo; el.appendChild(i); }
  else el.textContent = ((u.name || u.email || "?").trim().charAt(0) || "?").toUpperCase();
}
function syncMe() {
  const u = me();
  avatarFill($("drAvatar"), u); avatarFill($("setAvatar"), u);
  $("drName").textContent = u.name || ""; $("drRole").textContent = u.role || "";
  $("setName").textContent = u.name || ""; $("setEmail").textContent = u.email || "";
  $("verifyEmail").hidden = !user || !user.email || user.verified;
}
$("menuBtn").addEventListener("click", function () { syncMe(); openOverlay($("drawer")); this.setAttribute("aria-expanded", "true"); setTimeout(() => { try { $("drawerClose").focus({ preventScroll: true }); } catch (e) {} }, 350); });
$("toSettings").addEventListener("click", () => go("settings", { push: true }));

/* view my card, bigger */
function openViewer() {
  const u = me();
  $("bigImg").src = u.photo || ""; $("bigName").textContent = u.name || ""; $("bigRole").textContent = u.role || ""; $("bigDesc").textContent = u.look || "";
  const v = $("cardViewer");
  if (anyOverlay()) { OVER.forEach(id => { if (id !== "cardViewer") { $(id).classList.remove("on"); $(id).setAttribute("aria-hidden", "true"); } }); $("scrim").classList.remove("on"); v.classList.add("on"); v.setAttribute("aria-hidden", "false"); }
  else openOverlay(v);
  $("scrim").classList.remove("on");
}
["viewCard", "setAvatar", "drAvatar"].forEach(id => $(id).addEventListener("click", openViewer));
$("viewerEdit").addEventListener("click", () => { profileMode = "edit"; go("profile", { push: true }); });

/* ---------------- 07 settings ---------------- */
ENTER.settings = () => { syncMe(); syncVisibility(); applyTheme(); $("setScroll").scrollTop = 0; };
$("editProfile").addEventListener("click", () => { profileMode = "edit"; go("profile", { push: true }); });
$("verifyEmail").addEventListener("click", async function () {
  if (this.getAttribute("aria-busy")) return;   /* one request at a time */
  busy(this, true);
  try {
    const r = await api("/api/verify/send", { method: "POST" });
    toast(r.ok ? "Check your inbox — we sent a link to " + user.email : r.status === 429 ? "You’ve asked for a few already. Try again in an hour." : "Couldn’t send the email. Try again later.");
  } catch (e) { toast("Couldn’t send the email. Check your connection."); }
  finally { busy(this, false); }
});
$("logout").addEventListener("click", () => openOverlay($("outSheet")));
$("deleteAcc").addEventListener("click", () => openOverlay($("delSheet")));
$("outConfirm").addEventListener("click", async function () {
  busy(this, true);
  await stopPresence(true);
  try { await api("/api/logout", { method: "POST" }); } catch (e) {}
  tokenStore.del(); user = null; profile = null; pushRegistered = false;
  busy(this, false);
  go("auth", { back: true, reset: true }); toast("Logged out");
});
$("delConfirm").addEventListener("click", async function () {
  if (!user) return;
  const b = this, u = user; busy(b, true);
  try {
    await stopPresence(true);
    const res = await api("/api/account", { method: "DELETE" });
    if (!res.ok) throw new Error("delete failed");
    tokenStore.del(); store.del("loc:" + u.uid); store.del("swiped"); store.del("account");
    user = null; profile = null; pushRegistered = false;
    go("auth", { back: true, reset: true }); toast("Your account has been deleted");
  } catch (e) {
    if (e.code === "auth/unauthorized") {
      tokenStore.del(); user = null; profile = null; pushRegistered = false;
      go("auth", { back: true, reset: true }); toast("Your session expired. Sign in again.");
    } else toast("Couldn’t delete your account. Check your connection and try again.");
  } finally { busy(b, false); }
});

/* ---------------- auth state / boot ----------------
   On load we read the sign-in result the backend put in the URL fragment
   (#token=... on success, #auth_error=... on failure), store the token, then
   ask the backend who we are. There's no persistent listener like Firebase had:
   the session token in localStorage is the source of truth. */
function parseAuthHash() {
  const a = authFrom(location.hash);
  if (a.token || a.code || a.err || a.verified || a.verifyErr) { try { history.replaceState(null, "", location.pathname + location.search); } catch (x) {} }
  return a;
}

let pushRegistered = false;
async function initPush() {
  const P = plugin("PushNotifications") || (window.Capacitor && window.Capacitor.Plugins && window.Capacitor.Plugins.PushNotifications);
  if (!P || !user) return;
  try {
    let perm = await P.checkPermissions();
    if (perm.receive !== "granted") {
      perm = await P.requestPermissions();
    }
    if (perm.receive === "granted") {
      if (pushRegistered) return;
      pushRegistered = true;
      try {
        await P.createChannel({
          id: "fcm_default_channel",
          name: "General Notifications",
          description: "Kinjo notifications",
          importance: 4,
          visibility: 1,
          vibration: true
        });
      } catch (e) {}

      try { await P.removeAllListeners(); } catch (e) {}
      await P.addListener("registration", async tok => {
        if (tok && tok.value) {
          try {
            await api("/api/device", {
              method: "POST",
              body: JSON.stringify({ pushToken: tok.value, platform: "android" })
            });
            console.log("Device token registered successfully");
          } catch (e) {}
        }
      });
      await P.addListener("registrationError", err => {
        console.error("Push registration error:", err);
      });
      await P.addListener("pushNotificationReceived", notif => {
        if (notif) toast((notif.title || "Kinjo") + (notif.body ? ": " + notif.body : ""));
      });
      await P.register();
    }
  } catch (e) {
    console.error("initPush error:", e);
  }
}

async function boot() {
  const a = parseAuthHash(), token = await sessionFrom(a), err = a.err || (a.code && !token ? "session" : "");
  if (token) tokenStore.set(token);
  store.del("authPending");
  await loadMe();            /* sets user + profile from the backend (or clears on 401) */
  authReady = true;
  syncMe();
  if (err) toast(authMessage(err));
  else if (token && user) toast(complete(profile) ? "Welcome back, " + profile.name.split(" ")[0] : "Signed in" + (user.email ? " as " + user.email : ""));
  if (a.verified || a.verifyErr) verifyResult(a);
  maybeLeaveSplash();
  if (user) { initPush(); loadAds(); }
}

if (window.KINJO_HOOK) window.KINJO_HOOK({ go, route, runSplash, openViewer, syncMe, closeAll, setMode: m => { profileMode = m; }, get current() { return current; }, startPresence, recompute, deckState: () => ({ names: people.map(p => p.name), idx, cards: cards.length, dom: deck.querySelectorAll(".card").length, busy: deckBusy, pending: !!pendingPeople, visible: isVisible(), hiddenMode: $("s-nearby").classList.contains("hidden-mode"), nobody: $("s-nearby").classList.contains("nobody") }) });
hreplace();
runSplash();
boot();
