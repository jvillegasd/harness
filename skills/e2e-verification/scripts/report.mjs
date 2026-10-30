// Builds a self-contained report page from a kit.mjs run.
//   node report.mjs <shots-dir> [--title "Release 1.2.3 E2E"] [--findings findings.json] [--out report.html]
// findings.json: [{ "severity": "bug|ux|note", "title": "...", "body": "...", "tests": ["X1"] }]
// Screenshots are referenced relative to the report, so publish the shots folder next to it.
import { readFileSync, writeFileSync } from 'node:fs';
import { join, dirname, relative, resolve } from 'node:path';

const args = process.argv.slice(2);
const opt = (name, dflt) => { const i = args.indexOf(name); return i >= 0 ? args[i + 1] : dflt; };
const shots = resolve(args[0] || 'shots');
const out = resolve(opt('--out', join(dirname(shots), 'report.html')));
const title = opt('--title', 'E2E report');
const findingsFile = opt('--findings', null);
const results = JSON.parse(readFileSync(join(shots, 'results.json'), 'utf8'));
const findings = findingsFile ? JSON.parse(readFileSync(findingsFile, 'utf8')) : [];
const rel = relative(dirname(out), shots) || '.';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const passed = results.filter((r) => r.ok).length;
const areas = [...new Set(results.map((r) => r.area))];
const shotCount = results.reduce((n, r) => n + r.shots.length, 0);

const card = (r) => `<article class="card ${r.ok ? 'ok' : 'fail'}" id="${esc(r.id)}">
<header><span class="badge">${r.ok ? 'PASS' : 'FAIL'}</span><b>${esc(r.id)}</b> ${esc(r.title)}</header>
${r.error ? `<p class="err">${esc(r.error)}</p>` : ''}
<ul class="checks">${r.checks.map((c) => `<li class="${c.ok ? 'y' : 'n'}">${esc(c.msg)}</li>`).join('')}</ul>
<div class="shots">${r.shots.map((s) => `<figure><a href="${esc(rel + '/' + s.file)}" data-lb><img loading="lazy" src="${esc(rel + '/' + s.file)}" alt="${esc(s.label)}"></a><figcaption>${esc(s.label)}</figcaption></figure>`).join('')}</div>
</article>`;

const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>${esc(title)}</title><style>
:root{--bg:#f7f7f5;--fg:#1d1d1b;--mute:#6b6b66;--card:#fff;--line:#e2e1dc;--ok:#1f7a4d;--fail:#b3261e;--warn:#9a6700}
@media (prefers-color-scheme:dark){:root{--bg:#161615;--fg:#ecebe6;--mute:#a3a29b;--card:#1f1f1d;--line:#34332f;--ok:#4cc38a;--fail:#ff8a80;--warn:#e3b341}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.5 system-ui,sans-serif}
main{max-width:1100px;margin:0 auto;padding:24px 16px 64px}h1{margin:0 0 4px}h2{margin:36px 0 12px}.mute{color:var(--mute)}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:12px;margin:20px 0}
.tile{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px}.tile b{display:block;font-size:26px}
.card,.finding{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:14px 16px;margin:12px 0}
.card.fail{border-color:var(--fail)}.badge{font:600 11px system-ui;padding:2px 7px;border-radius:5px;margin-right:8px;color:#fff;background:var(--ok)}
.fail .badge{background:var(--fail)}.err{color:var(--fail);font-family:ui-monospace,monospace;font-size:13px}
.checks{margin:8px 0;padding-left:20px;font-size:14px}.checks .n{color:var(--fail)}
.shots{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:10px}figure{margin:0}
img{width:100%;border:1px solid var(--line);border-radius:6px;display:block}figcaption{font-size:12px;color:var(--mute);padding-top:4px}
.sev{font:600 11px system-ui;text-transform:uppercase;color:var(--warn)}.sev.bug{color:var(--fail)}
#lb{position:fixed;inset:0;background:rgba(0,0,0,.85);display:none;align-items:center;justify-content:center;padding:16px}#lb img{max-height:100%;width:auto;max-width:100%}
</style></head><body><main>
<h1>${esc(title)}</h1><p class="mute">${new Date().toISOString().slice(0, 16).replace('T', ' ')} UTC</p>
<div class="tiles"><div class="tile"><b>${passed}/${results.length}</b>tests passed</div><div class="tile"><b>${areas.length}</b>areas</div>
<div class="tile"><b>${shotCount}</b>screenshots</div><div class="tile"><b>${findings.length}</b>findings</div></div>
${findings.length ? `<h2>Findings</h2>${findings.map((f) => `<div class="finding"><span class="sev ${esc(f.severity)}">${esc(f.severity)}</span> <b>${esc(f.title)}</b>
<p>${esc(f.body)}</p>${(f.tests || []).map((t) => `<a href="#${esc(t)}">${esc(t)}</a>`).join(' ')}</div>`).join('')}` : ''}
${areas.map((a) => `<h2>${esc(a)}</h2>${results.filter((r) => r.area === a).map(card).join('')}`).join('')}
</main><div id="lb"><img alt=""></div><script>
const lb=document.getElementById('lb');document.addEventListener('click',(e)=>{const a=e.target.closest('[data-lb]');
if(a){e.preventDefault();lb.firstChild.src=a.href;lb.style.display='flex';}else if(e.target.closest('#lb')){lb.style.display='none';}});
</script></body></html>`;

writeFileSync(out, html);
console.log(`${out}  (${passed}/${results.length} passed, ${shotCount} screenshots)`);
