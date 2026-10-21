import re
html=open('app.fb.html').read(); js=open('app.fb.js').read(); w=open('../preview/wrapper.html').read(); mock=open('../preview/mock.js').read()
def sec(name): return re.search(r'<!--%s-->(.*?)<!--/%s-->'%(name,name),w,re.S).group(1)
# strip firebase imports
body_js=re.sub(r'^import[\s\S]*?;\s*\n','',js,flags=re.M)
assert 'import ' not in body_js.split('\n')[0]
page=html
page=page.replace('</style>','</style>\n'+sec('CSS'),1)
assert '<div class="shell">' in page
page=page.replace('<div class="shell">',sec('PANEL')+'<div class="shell">',1)
a=page.index('  </main>\n</div>')
page=page[:a]+sec('OVERLAYS')+'  </main>\n</div>\n'+sec('TAIL')+page[a+len('  </main>\n</div>\n'):]
page=page.replace('<script type="module">\n/*APP_JS*/\n</script>', sec('BOOT')+'\n<script type="module">\n'+mock+'\n'+body_js+'\n</script>')
page=page.replace('<title>Kinjo</title>','<title>Kinjo iPhone Preview</title>')
page='<meta charset="utf-8">\n'+page
open('../preview/index.html','w').write(page)
print(len(page), page.count('<script'))
