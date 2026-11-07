import fs from "fs";
const R = process.argv.slice(2).map(f => JSON.parse(fs.readFileSync(f, "utf8")));
const names = Object.keys(R[0].scenarios);
const c = (o, k) => o == null ? "-" : o[k] == null ? "-" : o[k];
console.log("| scenario | var | open-viewer found% | p50/p95 s | any-viewer found% | p50/p95 s | FP/far | flicker | mAh/h all / bg |");
console.log("|---|---|---|---|---|---|---|---|---|");
for (const n of names) for (const r of R) { const s = r.scenarios[n];
  console.log(`| ${n} | ${r.variant} | ${c(s.open_view,"found_pct")} (${s.open_view.near}) | ${c(s.open_view,"p50_s")}/${c(s.open_view,"p95_s")} | ${c(s.any_view,"found_pct")} (${s.any_view.near}) | ${c(s.any_view,"p50_s")}/${c(s.any_view,"p95_s")} | ${s.false_pos}/${s.far_pairs} | ${s.flicker} | ${s.mAh_h_all}/${s.mAh_h_background} |`); }
