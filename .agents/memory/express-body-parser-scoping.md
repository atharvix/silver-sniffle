---
name: Express body-parser limit scoping
description: Why adding a bigger express.json({limit}) on one route doesn't actually raise the limit for that route in this app.
---

Express's `express.json()` / `express.urlencoded()` middleware set an internal flag once they've consumed a request body. A later `express.json()` call in the middleware chain sees that flag and skips re-parsing — it does not re-check its own `limit` against the body.

**Why:** This app has an app-wide `app.use(express.json({limit: "256kb"}))` in `app.ts`, mounted before `app.use("/api", router)`. A route inside that router (e.g. `POST /profiles`, which needs a much larger limit for base64 photo uploads) tried adding its own `express.json({limit: "12mb"})` directly on the route — but the app-wide parser had already run and rejected (or truncated) the body first, so the route-level override never took effect. Silent-looking 413s resulted.

**How to apply:** To give one path prefix a different body-size limit than the app-wide default, mount a path-scoped `express.json({limit})` for that exact prefix (e.g. `app.use("/api/profiles", express.json({limit: "12mb"}))`) *before* the general `app.use(express.json({limit: "256kb"}))` in `app.ts`. Mounting order — not where the middleware is attached (app-level vs. router-level) — determines which limit actually wins.
