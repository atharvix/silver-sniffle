const CACHE='kinjo-v39';
const SHELL=['/','/index.html','/manifest.webmanifest','/icons/icon-192.png?v=2','/icons/icon-512.png?v=2'];
self.addEventListener('install',e=>{e.waitUntil(caches.open(CACHE).then(c=>c.addAll(SHELL)).then(()=>self.skipWaiting()))});
self.addEventListener('activate',e=>{e.waitUntil(caches.keys().then(ks=>Promise.all(ks.filter(k=>k!==CACHE).map(k=>caches.delete(k)))).then(()=>self.clients.claim()))});
self.addEventListener('fetch',e=>{
  const r=e.request; if(r.method!=='GET') return;
  const u=new URL(r.url);
  const same=u.origin===location.origin&&!u.pathname.startsWith('/__/');
  const lib=(u.host==='www.gstatic.com'&&u.pathname.startsWith('/firebasejs/'))||u.host==='fonts.gstatic.com'||u.host==='fonts.googleapis.com';
  if(r.mode==='navigate'&&same){e.respondWith(fetch(r).then(res=>{const c=res.clone();caches.open(CACHE).then(x=>x.put('/index.html',c));return res}).catch(()=>caches.match('/index.html')));return}
  if(same||lib){e.respondWith(caches.match(r).then(hit=>{const net=fetch(r).then(res=>{if(res.ok||res.type==='opaque'){const c=res.clone();caches.open(CACHE).then(x=>x.put(r,c))}return res});return hit||net}))}
});
