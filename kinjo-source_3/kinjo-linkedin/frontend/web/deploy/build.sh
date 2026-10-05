#!/bin/bash
# Build the static web app for hosting (Utho / nginx / any static host).
# No Firebase. Injects the Go backend URL into the built index.html and bumps
# the service-worker cache so clients pick up the new build.
#
#   API_BASE=https://your-backend-host bash build.sh
set -e
cd "$(dirname "$0")"

API_BASE=${API_BASE:?set API_BASE to the Go backend URL, e.g. https://api.kinjo.example}

# 1. assemble public/index.html from web/src (app.fb.html + app.fb.js)
( cd ../src && python3 build_live.py )

# 2. point the app at the backend
python3 - "$API_BASE" <<'PY'
import sys
api = sys.argv[1].rstrip('/')
p = 'public/index.html'; s = open(p).read()
old = '/*API_BASE*/ "http://localhost:8080"'
assert old in s, "API_BASE placeholder not found (did build_live run?)"
open(p, 'w').write(s.replace(old, '/*API_BASE*/ "%s"' % api))
print('API base set to', api)
PY

# 3. bump the service-worker cache name (kinjo-vN -> kinjo-v[N+1])
python3 - <<'PY'
import re
p = 'public/sw.js'; s = open(p).read()
m = re.search(r'kinjo-v(\d+)', s)
if m:
    n = int(m.group(1)) + 1
    open(p, 'w').write(re.sub(r'kinjo-v\d+', 'kinjo-v%d' % n, s))
    print('sw cache -> kinjo-v%d' % n)
PY

echo "built public/ — upload its contents to your static host, or serve with nginx."
echo "the phone app loads this site via server.url in native/capacitor.config.json."
