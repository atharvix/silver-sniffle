# Builds deploy/public/index.html from app.fb.html + app.fb.js (config injected later by deploy.sh)
old=open('../deploy/public/index.html').read()
head=old[:old.index('<title>Kinjo</title>')]
src=open('app.fb.html').read(); js=open('app.fb.js').read()
body=src.replace('<script type="module">\n/*APP_JS*/\n</script>','<script type="module">\n'+js+'\n</script>')
assert '/*APP_JS*/' not in body
body=body.replace('<title>Kinjo</title>\n','<title>Kinjo</title>\n<style>:root{padding:env(safe-area-inset-top,0px) 0 env(safe-area-inset-bottom,0px)}</style>\n',1)
i=body.index('</style>\n', body.index('/* ================= Kinjo tokens'))+len('</style>\n')
body=body[:i]+'</head>\n<body>\n'+body[i:]
# not inside the phone app: it already has every file, and a stale cache there only gets in the way
sw="<script>if('serviceWorker' in navigator&&!(window.Capacitor&&Capacitor.isNativePlatform&&Capacitor.isNativePlatform())){addEventListener('load',function(){navigator.serviceWorker.register('/sw.js').catch(function(){})})}</script>\n</body>\n</html>\n"
open('../deploy/public/index.html','w').write(head+body.rstrip()+'\n\n'+sw)
print('ok')
