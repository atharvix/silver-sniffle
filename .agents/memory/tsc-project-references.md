---
name: tsc project references and stale dist
description: Why `tsc --noEmit` can report "no exported member" for exports that clearly exist in source, in this pnpm monorepo.
---

When a package's tsconfig has a `references` array (composite project references, e.g. `artifacts/api-server` referencing `lib/db` and `lib/api-zod`), running `tsc -p <pkg> --noEmit` resolves the referenced packages' types from their built `dist/*.d.ts` output — not their `src/*.ts` source — even though the package's `exports` field in `package.json` points at `./src/index.ts` for runtime resolution.

**Why:** Composite referenced projects (`"composite": true`, `"emitDeclarationOnly": true`) only produce type info via their own build step. If you edit source in a referenced package (e.g. add a new schema/export) but never rebuild it, `tsc` on the dependent package silently uses stale/missing declarations and reports "has no exported member" for things that plainly exist in source.

**How to apply:** After adding/renaming exports in a referenced package (e.g. `lib/db`, `lib/api-zod`), run `npx tsc -b <path-to-changed-package> --force` (or `tsc -b` at the repo root) before trusting a dependent package's `tsc --noEmit` output. Deleting `.tsbuildinfo` alone is not enough — the `dist/*.d.ts` files themselves must be regenerated.
