// git guess playground: loads the WebAssembly build once and classifies the
// textarea on every change. Nothing is sent anywhere.
(function () {
  const $ = (s) => document.querySelector(s);
  const examples = {
    feat: `diff --git a/src/export/csv.ts b/src/export/csv.ts
new file mode 100644
--- /dev/null
+++ b/src/export/csv.ts
@@ -0,0 +1,18 @@
+import { Report } from "../report";
+
+export interface CsvOptions {
+  separator?: string;
+  header?: boolean;
+}
+
+export function toCsv(report: Report, options: CsvOptions = {}): string {
+  const sep = options.separator ?? ",";
+  const rows = report.rows.map((r) => r.map(escape).join(sep));
+  if (options.header !== false) {
+    rows.unshift(report.columns.join(sep));
+  }
+  return rows.join("\\n");
+}
+
+function escape(v: string) {
+  return /[",\\n]/.test(v) ? '"' + v.replace(/"/g, '""') + '"' : v;
+}
diff --git a/src/export/index.ts b/src/export/index.ts
--- a/src/export/index.ts
+++ b/src/export/index.ts
@@ -1,3 +1,4 @@
 export { toJson } from "./json";
 export { toHtml } from "./html";
+export { toCsv } from "./csv";
diff --git a/src/cli.ts b/src/cli.ts
--- a/src/cli.ts
+++ b/src/cli.ts
@@ -22,6 +22,9 @@ program
   .option("--json", "print the report as JSON")
+  .option("--csv", "print the report as CSV")
   .action((opts) => {
     if (opts.json) return console.log(toJson(report));
+    if (opts.csv) return console.log(toCsv(report));
     console.log(toHtml(report));
   });
`,
    fix: `diff --git a/src/cart/total.js b/src/cart/total.js
--- a/src/cart/total.js
+++ b/src/cart/total.js
@@ -12,7 +12,10 @@ export function total(items, coupon) {
   let sum = 0;
   for (const item of items) {
-    sum += item.price * item.qty;
+    if (item.qty > 0) {
+      sum += item.price * item.qty;
+    }
   }
-  return sum - coupon.amount;
+  const discount = coupon ? coupon.amount : 0;
+  return Math.max(0, sum - discount);
 }
`,
    docs: `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -20,6 +20,14 @@ npm install acme-cart
 
 ## Usage
 
+\`\`\`js
+import { total } from "acme-cart";
+
+total(items, coupon); // returns the amount to pay, never below zero
+\`\`\`
+
+Coupons are optional. See [the pricing guide](docs/pricing.md) for rounding rules.
+
 ## License
 
 MIT
`,
    test: `diff --git a/src/cart/total.test.js b/src/cart/total.test.js
new file mode 100644
--- /dev/null
+++ b/src/cart/total.test.js
@@ -0,0 +1,16 @@
+import { describe, it, expect } from "vitest";
+import { total } from "./total";
+
+describe("total", () => {
+  it("sums quantities", () => {
+    expect(total([{ price: 2, qty: 3 }])).toBe(6);
+  });
+
+  it("ignores negative quantities", () => {
+    expect(total([{ price: 2, qty: -1 }])).toBe(0);
+  });
+
+  it("never goes below zero with a coupon", () => {
+    expect(total([{ price: 2, qty: 1 }], { amount: 5 })).toBe(0);
+  });
+});
`,
    deps: `diff --git a/package.json b/package.json
--- a/package.json
+++ b/package.json
@@ -14,7 +14,7 @@
   "devDependencies": {
-    "vitest": "^1.6.0",
+    "vitest": "^2.1.3",
     "typescript": "^5.4.0"
   }
diff --git a/pnpm-lock.yaml b/pnpm-lock.yaml
--- a/pnpm-lock.yaml
+++ b/pnpm-lock.yaml
@@ -40,9 +40,9 @@ importers:
       vitest:
-        specifier: ^1.6.0
-        version: 1.6.0
+        specifier: ^2.1.3
+        version: 2.1.3
`,
  };

  // ---- terminal typewriter
  const lines = [
    ['<span class="t-dim">$ </span>git add .'],
    ['<span class="t-dim">$ </span>git guess'],
    ['<span class="t-ok">✔</span> <span class="t-b">feat(auth)</span>  <span class="t-dim">████████░░</span> <span class="t-ok">82%</span>', 0],
    ['<span class="t-dim">  also:</span> fix <span class="t-dim">11% ·</span> refactor <span class="t-dim">5%</span>', 0],
    [''],
    ['<span class="t-dim">$ </span>git commit -m "add magic links"'],
    ['<span class="t-acc">git guess:</span> feat(auth): add magic links', 0],
    ['<span class="t-dim">[main 3f2a1c0] feat(auth): add magic links</span>', 0],
  ];
  const demo = $('#demo');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;
  (async function type() {
    let html = '';
    for (const [line, typed = 1] of lines) {
      if (typed && !reduced) {
        const text = line.replace(/<[^>]+>/g, '');
        const prefix = line.startsWith('<span class="t-dim">$ </span>') ? '<span class="t-dim">$ </span>' : '';
        const body = text.replace(/^\$ /, '');
        for (let i = 1; i <= body.length; i++) {
          demo.innerHTML = html + prefix + '<span class="cursor">' + body.slice(0, i) + '</span>';
          await wait(28 + Math.random() * 40);
        }
        await wait(250);
      }
      html += line + '\n';
      demo.innerHTML = html + '<span class="cursor"></span>';
      await wait(typed ? 120 : 260);
    }
  })();
  function wait(ms) { return new Promise((r) => setTimeout(r, ms)); }

  // ---- playground
  const ta = $('#diff');
  const result = $('#result');
  let ready = false;
  ta.value = examples.feat;
  document.querySelectorAll('[data-example]').forEach((b) =>
    b.addEventListener('click', () => { ta.value = examples[b.dataset.example]; run(); ta.focus(); }));
  let timer;
  ta.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(run, 150); });

  const go = new Go();
  const url = 'git-guess.wasm';
  (WebAssembly.instantiateStreaming
    ? WebAssembly.instantiateStreaming(fetch(url), go.importObject)
    : fetch(url).then((r) => r.arrayBuffer()).then((b) => WebAssembly.instantiate(b, go.importObject)))
    .then((r) => { go.run(r.instance); ready = true; run(); })
    .catch((e) => { result.innerHTML = '<p class="r-err">The model could not be loaded (' + e + '). The command line works the same way.</p>'; });

  function run() {
    if (!ready) return;
    const text = ta.value;
    if (!text.trim()) { result.innerHTML = '<p class="empty">Paste the output of <code>git diff</code> or <code>git show</code>, or pick an example above.</p>'; return; }
    let r;
    try { r = JSON.parse(gitGuess(text)); } catch (e) { r = { error: String(e) }; }
    if (r.error) { result.innerHTML = '<p class="r-err">' + esc(r.error) + '</p>'; return; }
    const pct = Math.min(99, Math.round(r.confidence * 100));
    const low = r.confidence < 0.55;
    const alts = r.candidates.slice(1).filter((c) => c.p >= 0.01).map((c) => '<span><b>' + esc(c.type) + '</b> ' + Math.round(c.p * 100) + '%</span>').join('');
    result.innerHTML =
      '<div class="r-head"><span class="r-type">' + esc(r.header.replace(/:$/, '')) + '</span><span class="r-conf' + (low ? ' low' : '') + '">' + pct + '%</span>' + (low ? '<span class="r-conf low">would ask you</span>' : '') + '</div>' +
      '<div class="r-bar"><i style="width:' + pct + '%"></i></div>' +
      '<div class="r-alts"><span class="t-dim">also:</span>' + alts + '</div>';
  }
  function esc(s) { return String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c])); }

  // ---- tabs & copy
  document.querySelectorAll('.tabs button').forEach((b) => b.addEventListener('click', () => {
    document.querySelectorAll('.tabs button').forEach((x) => x.setAttribute('aria-selected', x === b));
    document.querySelectorAll('.panes pre').forEach((p) => p.classList.toggle('active', p.dataset.pane === b.dataset.tab));
  }));
  document.querySelectorAll('.copy').forEach((b) => b.addEventListener('click', async () => {
    const code = b.parentElement.querySelector('code').innerText;
    try { await navigator.clipboard.writeText(code); b.textContent = 'Copied'; setTimeout(() => (b.textContent = 'Copy'), 1500); } catch (_) {}
  }));
})();
