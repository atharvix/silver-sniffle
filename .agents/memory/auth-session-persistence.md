---
name: auth session persistence decision
description: Why OTP/verification-token state must live in the database, not in-memory Maps, in this project's api-server.
---

This project's api-server (`artifacts/api-server/src/routes/auth.ts`) issues opaque bearer "verification tokens" after OTP verification, used by profile endpoints to derive caller identity. These tokens (and the underlying OTP codes) are persisted in Postgres (`lib/db/src/schema/verification.ts`: `verification_tokens`, `otp_codes`, `verified_emails` tables) rather than kept in in-memory `Map`s.

**Why:** An in-memory Map is wiped on every server restart/deploy. Since every signed-in user's session is just a token bound to that Map, a deploy would instantly 401 every active user and force the whole user base through re-verification simultaneously (plus a burst of OTP emails). This is a common trap for lightweight custom-auth setups on Replit, where deploys/restarts are frequent.

**How to apply:** Any new short-lived server-side auth/session state (magic links, OTPs, session tokens, CSRF-ish tokens tied to a user) should default to a persisted store (the existing Postgres DB here) instead of a plain in-memory Map, unless the state is provably fine to lose on restart (e.g. simple rate-limit counters, where losing them just resets a quota harmlessly).
