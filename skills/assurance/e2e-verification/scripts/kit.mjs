// Helpers for a screenshot-backed Playwright E2E suite. Copy next to the runner and install:
//   npm i playwright-core pdfjs-dist   (pdfjs-dist only when the product renders PDFs)
// Usage:
//   import { createRun } from './kit.mjs';
//   const run = await createRun({ out: 'shots', chrome: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' });
//   const ctx = await run.newContext();
//   await run.test('E1', 'Editor', 'Editor loads', async () => { ...; run.ok(cond, 'msg'); await run.shot(page, 'label'); });
//   await run.finish();   // writes <out>/results.json, closes the browser, sets the exit code
// Filter tests with a regex argument: node e2e.mjs '^(E1|X1)$'
import { chromium } from 'playwright-core';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { createRequire } from 'node:module';

const VIEW = 'http://pdfview.test'; // fake origin routed in-process, so pdf.js never needs a server

export async function createRun({ out = 'shots', chrome, only = process.argv[2], viewport = { width: 1440, height: 1000 }, locale } = {}) {
  mkdirSync(out, { recursive: true });
  const filter = only ? new RegExp(only) : null;
  const browser = await chromium.launch({ executablePath: chrome, headless: true });
  const results = [];
  const pdfs = new Map();
  let cur = null;

  function ok(cond, msg) {
    cur.checks.push({ ok: !!cond, msg });
    if (!cond) { throw new Error('assert: ' + msg); }
  }

  async function shot(page, label, opts = {}) {
    const file = `${cur.id}-${String(cur.shots.length + 1).padStart(2, '0')}.png`;
    await page.screenshot({ path: join(out, file), fullPage: !!opts.full, ...(opts.clip ? { clip: opts.clip } : {}) });
    cur.shots.push({ file, label });
  }

  async function test(id, area, title, fn) {
    if (filter && !filter.test(id)) { return; }
    cur = { id, area, title, checks: [], shots: [], ok: true, error: null };
    results.push(cur);
    try { await fn(); } catch (e) { cur.ok = false; cur.error = String(e.message || e).slice(0, 600); }
    if (!cur.shots.length) { cur.ok = false; cur.error ||= 'no screenshot captured'; }
    console.log(`${cur.ok ? 'PASS' : 'FAIL'} ${id} ${title}${cur.error ? '  -> ' + cur.error : ''}`);
  }

  async function newContext(extra = {}) {
    const ctx = await browser.newContext({ viewport, deviceScaleFactor: 1, ...(locale ? { locale } : {}), ...extra });
    await ctx.route(`${VIEW}/**`, (route) => servePdfViewer(route, pdfs));
    return ctx;
  }

  // Captured terminal or HTTP output, rendered as a page so it can be screenshotted.
  async function evidence(ctx, title, text) {
    const p = await ctx.newPage();
    await p.setViewportSize({ width: 1200, height: 400 });
    await p.setContent(`<!doctype html><meta charset="utf-8"><style>body{margin:0;background:#1e1e1e;color:#d4d4d4;font:13px/1.45 Menlo,monospace}
h1{font:600 14px system-ui;background:#333;color:#fff;margin:0;padding:10px 16px}pre{margin:0;padding:14px 16px;white-space:pre-wrap}</style>
<h1>${esc(title)}</h1><pre>${esc(text)}</pre>`);
    await shot(p, title, { full: true });
    await p.close();
  }

  // Rasterizes PDF bytes with pdf.js and screenshots every page. Returns {pages, text[], sizes[] in mm} for assertions.
  async function viewPdf(ctx, bytes, title, opts = {}) {
    const key = `${cur.id}-${cur.shots.length + 1}.pdf`;
    pdfs.set(key, bytes);
    writeFileSync(join(out, key), bytes);
    const p = await ctx.newPage();
    await p.setViewportSize({ width: opts.width || 1440, height: 900 });
    await p.goto(`${VIEW}/viewer.html?f=${key}&scale=${opts.scale || 1}&title=${encodeURIComponent(title)}&pages=${(opts.pages || []).join(',')}`);
    await p.waitForFunction(() => window.__pdf, null, { timeout: 30000 });
    const info = await p.evaluate(() => window.__pdf);
    await shot(p, title, { full: true });
    await p.close();
    return info;
  }

  // Fetches with the page's cookies (session-bound endpoints, downloads).
  async function fetchBody(page, url, init) {
    const r = await page.request.fetch(url, init);
    return { status: r.status(), type: r.headers()['content-type'] || '', body: await r.body() };
  }

  async function finish() {
    writeFileSync(join(out, 'results.json'), JSON.stringify(results, null, 1));
    const failed = results.filter((r) => !r.ok).length;
    console.log(`\n${results.length - failed}/${results.length} passed`);
    await browser.close();
    process.exitCode = failed ? 1 : 0;
  }

  return { browser, results, current: () => cur, ok, shot, test, newContext, evidence, viewPdf, fetchBody, finish };
}

function esc(s) { return String(s).replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c])); }

let pdfjsRoot = null;
function servePdfViewer(route, pdfs) {
  const u = new URL(route.request().url());
  if (u.pathname.startsWith('/pdf/')) {
    return route.fulfill({ status: 200, contentType: 'application/pdf', body: pdfs.get(u.pathname.slice(5)) });
  }
  if (u.pathname === '/viewer.html') { return route.fulfill({ status: 200, contentType: 'text/html', body: VIEWER_HTML }); }
  if (!pdfjsRoot) {
    const require = createRequire(join(process.cwd(), 'package.json'));
    pdfjsRoot = dirname(require.resolve('pdfjs-dist/package.json'));
  }
  const f = join(pdfjsRoot, u.pathname.startsWith('/standard_fonts/') ? u.pathname : '/build' + u.pathname);
  if (!existsSync(f)) { return route.fulfill({ status: 404, body: 'not found' }); }
  return route.fulfill({ status: 200, contentType: f.endsWith('.mjs') ? 'text/javascript' : 'application/octet-stream', body: readFileSync(f) });
}

const VIEWER_HTML = `<!doctype html><meta charset="utf-8"><title>PDF (pdf.js)</title>
<style>body{margin:0;background:#525659;font:13px system-ui;color:#eee}header{padding:8px 14px;background:#323639}
#pages{display:flex;flex-wrap:wrap;gap:18px;padding:18px;align-items:flex-start}figure{margin:0}figcaption{text-align:center;padding:4px}
canvas{background:#fff;box-shadow:0 2px 8px rgba(0,0,0,.5);display:block}</style>
<header id="h"></header><div id="pages"></div>
<script type="module">
import * as pdfjs from '/pdf.min.mjs';
pdfjs.GlobalWorkerOptions.workerSrc = '/pdf.worker.min.mjs';
const q = new URLSearchParams(location.search);
const scale = Number(q.get('scale') || 1);
const doc = await pdfjs.getDocument({ url: '/pdf/' + q.get('f'), standardFontDataUrl: '/standard_fonts/' }).promise;
const text = [], sizes = [];
document.getElementById('h').textContent = q.get('title') + '  ·  ' + doc.numPages + ' page(s)';
const want = (q.get('pages') || '').split(',').filter(Boolean).map(Number);
for (let i = 1; i <= doc.numPages; i++) {
  const page = await doc.getPage(i);
  const tc = await page.getTextContent();
  text.push(tc.items.map((it) => it.str).join(' ').replace(/\\s+/g, ' '));
  const vp1 = page.getViewport({ scale: 1 });
  sizes.push([Math.round(vp1.width * 25.4 / 72), Math.round(vp1.height * 25.4 / 72)]);
  if (want.length && !want.includes(i)) continue;
  const vp = page.getViewport({ scale });
  const c = document.createElement('canvas'); c.width = vp.width; c.height = vp.height;
  await page.render({ canvasContext: c.getContext('2d'), viewport: vp }).promise;
  const fig = document.createElement('figure'); const cap = document.createElement('figcaption');
  cap.textContent = 'Page ' + i + ' of ' + doc.numPages; fig.append(c, cap);
  document.getElementById('pages').append(fig);
}
window.__pdf = { pages: doc.numPages, text, sizes };
</script>`;
