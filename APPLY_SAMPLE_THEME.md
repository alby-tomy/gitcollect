# gitcollect docs — Apply sample.html Theme to docs/index.html

> This prompt applies the exact visual theme from sample.html to
> docs/index.html. Content must not change — not a single word,
> command, flag description, code example, link, or anchor id.
> Only the visual presentation changes.
>
> sample.html is the reference design. docs/index.html is the target.
> Read both files completely before writing a single line.

---

## Step 0 — Read both files completely first

```bash
# Confirm both files exist and see their sizes
wc -l sample.html docs/index.html

# Read both files fully before proceeding
cat sample.html
cat docs/index.html
```

Do not start writing until you have read every line of both files.
Report what you found — structure, sections, class names currently
in docs/index.html, and how they map to the sample.html design system.

---

## What you are doing

Extracting the complete visual design system from sample.html and
applying it to docs/index.html. The result should look like
docs/index.html was built with the same design system as sample.html
from the beginning.

You are NOT changing in docs/index.html:
- Any text content — descriptions, flag names, command names, notes
- Any code examples or terminal output blocks
- Any links (href values)
- Any section anchor IDs (id attributes on sections and divs)
- Any structural ordering of sections
- Any navigation link labels or targets

You ARE changing:
- The entire `<style>` block (replace with the design system below)
- Google Fonts import (add Space Grotesk + JetBrains Mono)
- HTML class names on existing elements to match the design system
- Wrapping elements where needed to support the new patterns
- The `<nav>` structure to match sample.html's nav pattern
- The footer structure to match sample.html's footer pattern
- Adding the JavaScript from sample.html (tabs, nav active state)

---

## Design system to implement — extract exactly from sample.html

### CSS variables — copy these exactly

```css
:root {
  --ink:   #0A0E14;
  --ink2:  #111720;
  --ink3:  #1A2230;
  --wire:  #1E2A3A;
  --wire2: #2A3A50;
  --dim:   #4A5A70;
  --muted: #6A7A90;
  --body:  #B8C4D4;
  --head:  #E8EEF8;
  --lime:  #C8FF57;
  --lime2: #A8E040;
  --mono:  'JetBrains Mono', monospace;
  --sans:  'Space Grotesk', system-ui, sans-serif;
}
```

### Google Fonts — add to `<head>` before the `<style>` block

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@300;400;500;600;700&family=JetBrains+Mono:ital,wght@0,400;0,500;0,600;1,400&display=swap" rel="stylesheet">
```

### Base styles

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
html { scroll-behavior: smooth; }

body {
  background: var(--ink);
  color: var(--body);
  font-family: var(--sans);
  font-size: 16px;
  line-height: 1.6;
  -webkit-font-smoothing: antialiased;
}

.container { max-width: 1080px; margin: 0 auto; padding: 0 40px; }
```

### Nav — replace existing nav entirely

```html
<nav>
  <div class="nav-inner">
    <a href="#" class="nav-logo">git<span>collect</span></a>
    <ul class="nav-links">
      <!-- Keep all existing nav links exactly as they are -->
      <!-- Only change the HTML wrapper structure, not the link labels/hrefs -->
    </ul>
  </div>
</nav>
```

```css
nav {
  position: sticky;
  top: 0;
  z-index: 100;
  background: rgba(10,14,20,0.92);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--wire);
}

.nav-inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: 0 40px;
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.nav-logo {
  font-family: var(--mono);
  font-size: 14px;
  font-weight: 600;
  color: var(--lime);
  text-decoration: none;
  letter-spacing: -0.02em;
}
.nav-logo span { color: var(--muted); }

.nav-links {
  display: flex;
  align-items: center;
  gap: 32px;
  list-style: none;
}
.nav-links a {
  font-size: 13px;
  font-weight: 500;
  color: var(--muted);
  text-decoration: none;
  letter-spacing: 0.02em;
  transition: color 0.15s;
}
.nav-links a:hover { color: var(--head); }

/* CTA button in nav (GitHub link) */
.nav-cta {
  font-family: var(--mono);
  font-size: 12px;
  font-weight: 600;
  padding: 7px 16px;
  background: var(--lime);
  color: var(--ink);
  border-radius: 4px;
  text-decoration: none;
  transition: background 0.15s;
}
.nav-cta:hover { background: var(--lime2); }
```

### Section pattern

Every `<section>` gets this treatment:

```css
.section { padding: 80px 0; }

.section-label {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--lime);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
}
.section-label::after {
  content: '';
  flex: 1;
  max-width: 40px;
  height: 1px;
  background: var(--lime);
  opacity: 0.4;
}

.section-title {
  font-size: clamp(24px, 3vw, 36px);
  font-weight: 700;
  color: var(--head);
  letter-spacing: -0.025em;
  line-height: 1.2;
  margin-bottom: 16px;
}

.section-sub {
  font-size: 15px;
  color: var(--muted);
  max-width: 480px;
  margin-bottom: 48px;
  line-height: 1.7;
}
```

For each section heading in docs/index.html:
- The `<h2>` section title → add class `section-title`
- The lead paragraph → add class `section-sub`
- Add a `<div class="section-label">Label text</div>` before the h2
  using a short label that matches the section (e.g. "Commands",
  "Installation", "Walkthrough", "Philosophy")
- Wrap existing `<section>` elements in `class="section"` if not present

### Code blocks

```css
.code-block {
  background: var(--ink2);
  border: 1px solid var(--wire);
  border-radius: 6px;
  padding: 14px 18px;
  font-family: var(--mono);
  font-size: 12.5px;
  line-height: 1.7;
  overflow-x: auto;
}

/* Inline spans inside code blocks */
.code-block .comment { color: var(--dim); font-style: italic; }
.code-block .prompt  { color: var(--lime); user-select: none; }
.code-block .out     { color: var(--muted); }
.code-block .ok      { color: var(--lime); }
.code-block .flag    { color: #C792EA; }
.code-block .str     { color: #7DBBE6; }

.code-tag {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
  color: var(--dim);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  margin-bottom: 6px;
}
```

Apply `class="code-block"` to every `<pre>`, `<code>` block, or
terminal output block in docs/index.html. Where docs/index.html
currently uses `<pre><code>`, wrap in a `<div class="code-block">`.

### Inline code

```css
:not(.code-block) > code,
:not(pre) > code {
  font-family: var(--mono);
  font-size: 0.875em;
  color: var(--lime);
  background: rgba(200,255,87,0.08);
  border: 1px solid rgba(200,255,87,0.12);
  border-radius: 3px;
  padding: 1px 5px;
}
```

### Command signature lines

Where docs/index.html has command signatures (`.sig` or `<p class="sig">`):

```css
.sig {
  font-family: var(--mono);
  font-size: 15px;
  font-weight: 500;
  margin-bottom: 8px;
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.prog {
  color: var(--lime);
  font-weight: 600;
}
```

### Flag tables

```css
.flags {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
  border: 1px solid var(--wire);
  border-radius: 6px;
  overflow: hidden;
  margin-top: 12px;
}
.flags tr { border-bottom: 1px solid var(--wire2); }
.flags tr:last-child { border-bottom: none; }
.flags td { padding: 8px 14px; vertical-align: top; line-height: 1.5; }
.flagname {
  font-family: var(--mono);
  font-size: 12.5px;
  color: #C792EA;
  white-space: nowrap;
  width: 180px;
  background: rgba(199,146,234,0.05);
}
.flagdesc { color: var(--muted); font-size: 0.875rem; }
.flagdesc code { color: var(--lime); font-size: 0.85em; }
```

### Command blocks (cmd-block)

```css
.cmd-block {
  padding: 20px 0;
  border-bottom: 1px solid var(--wire2);
}
.cmd-block:last-child { border-bottom: none; }

.desc {
  font-size: 0.9rem;
  color: var(--muted);
  line-height: 1.7;
  margin-bottom: 12px;
  max-width: 680px;
}
.desc code  { color: var(--lime); }
.desc strong { color: var(--head); font-weight: 500; }
```

### Note and warning boxes

```css
.note {
  font-size: 0.875rem;
  color: var(--muted);
  border-left: 2px solid var(--lime);
  padding: 10px 14px;
  margin: 14px 0;
  background: rgba(200,255,87,0.04);
  border-radius: 0 4px 4px 0;
  line-height: 1.6;
}
.note code { color: var(--lime); }

.warn {
  font-size: 0.875rem;
  color: var(--muted);
  border-left: 2px solid #FEBC2E;
  padding: 10px 14px;
  margin: 14px 0;
  background: rgba(254,188,46,0.05);
  border-radius: 0 4px 4px 0;
  line-height: 1.6;
}
```

### Concept / philosophy table

```css
.concept-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
  border: 1px solid var(--wire);
  border-radius: 6px;
  overflow: hidden;
  margin-top: 16px;
}
.concept-table td {
  padding: 12px 16px;
  border-bottom: 1px solid var(--wire2);
  vertical-align: top;
  line-height: 1.6;
}
.concept-table td:first-child {
  font-family: var(--mono);
  font-size: 12.5px;
  font-weight: 600;
  color: var(--lime);
  white-space: nowrap;
  width: 160px;
  background: rgba(200,255,87,0.04);
}
.concept-table td:last-child { color: var(--muted); }
.concept-table tr:last-child td { border-bottom: none; }
.concept-table code { color: var(--lime); font-size: 0.85em; }
```

### Walkthrough steps

```css
.setup-steps {
  display: flex;
  flex-direction: column;
  gap: 0;
  position: relative;
}
.setup-steps::before {
  content: '';
  position: absolute;
  left: 19px; top: 38px; bottom: 38px;
  width: 1px;
  background: var(--wire);
}
.setup-step {
  display: flex;
  gap: 24px;
  position: relative;
}
.step-num {
  width: 38px; height: 38px;
  border-radius: 50%;
  background: var(--ink2);
  border: 1px solid var(--wire2);
  display: flex; align-items: center; justify-content: center;
  font-family: var(--mono);
  font-size: 13px;
  font-weight: 600;
  color: var(--lime);
  flex-shrink: 0;
  position: relative;
  z-index: 1;
}
.step-body {
  padding-top: 8px;
  padding-bottom: 36px;
  flex: 1;
}
.step-body h3 {
  font-size: 15px;
  font-weight: 600;
  color: var(--head);
  margin-bottom: 6px;
}
.step-body p {
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 14px;
  line-height: 1.6;
}
```

Apply this to the walkthrough / getting-started section in docs/index.html.
Each numbered step gets wrapped in `<div class="setup-step">` with a
`<div class="step-num">N</div>` and a `<div class="step-body">`.
The connecting line between steps is drawn by `.setup-steps::before`.

### Install method cards

```css
.install-method {
  background: var(--ink2);
  border: 1px solid var(--wire);
  border-radius: 6px;
  padding: 20px 24px;
  margin-bottom: 16px;
}
.install-method h3 {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  color: var(--lime);
  letter-spacing: 0.1em;
  text-transform: uppercase;
  margin-bottom: 10px;
}
```

Wrap each install method block in `<div class="install-method">`.

### Tabbed install panels (for macOS / Linux / Windows tabs)

```css
.install-tabs {
  display: flex;
  gap: 2px;
  background: var(--wire);
  border: 1px solid var(--wire);
  border-bottom: none;
  border-radius: 6px 6px 0 0;
  padding: 8px 8px 0;
}
.tab-btn {
  font-family: var(--mono);
  font-size: 11px;
  font-weight: 600;
  padding: 7px 16px;
  border-radius: 4px 4px 0 0;
  background: transparent;
  border: none;
  color: var(--muted);
  cursor: pointer;
  transition: color 0.15s;
  letter-spacing: 0.04em;
}
.tab-btn:hover { color: var(--body); }
.tab-btn.active { background: var(--ink2); color: var(--lime); }
.tab-panel {
  display: none;
  background: var(--ink2);
  border: 1px solid var(--wire);
  border-top: none;
  border-radius: 0 0 6px 6px;
  padding: 18px 20px;
}
.tab-panel.active { display: block; }
```

Add the switchTab JavaScript from sample.html to make the tabs work.

### Experimental badge

```css
.badge-experimental {
  font-family: var(--mono);
  font-size: 10px;
  font-weight: 600;
  color: #FEBC2E;
  border: 1px solid #FEBC2E;
  border-radius: 3px;
  padding: 1px 6px;
  vertical-align: middle;
  margin-left: 6px;
  opacity: 0.85;
}
```

### Buttons

```css
.btn-primary {
  font-family: var(--mono);
  font-size: 13px;
  font-weight: 600;
  padding: 12px 24px;
  background: var(--lime);
  color: var(--ink);
  border-radius: 4px;
  text-decoration: none;
  transition: background 0.15s, transform 0.1s;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
.btn-primary:hover { background: var(--lime2); transform: translateY(-1px); }

.btn-ghost {
  font-family: var(--mono);
  font-size: 13px;
  font-weight: 500;
  padding: 12px 24px;
  border: 1px solid var(--wire2);
  color: var(--body);
  border-radius: 4px;
  text-decoration: none;
  transition: border-color 0.15s, color 0.15s;
}
.btn-ghost:hover { border-color: var(--muted); color: var(--head); }
```

### Footer

```css
footer {
  border-top: 1px solid var(--wire);
  padding: 40px 0;
  margin-top: 40px;
}
.footer-inner {
  max-width: 1080px;
  margin: 0 auto;
  padding: 0 40px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.footer-logo {
  font-family: var(--mono);
  font-size: 13px;
  color: var(--dim);
}
.footer-logo span { color: var(--lime); }
.footer-links {
  display: flex;
  gap: 24px;
  list-style: none;
}
.footer-links a {
  font-size: 12px;
  color: var(--dim);
  text-decoration: none;
  transition: color 0.15s;
}
.footer-links a:hover { color: var(--body); }
```

### Mobile responsive

```css
@media (max-width: 768px) {
  .container    { padding: 0 24px; }
  .nav-inner    { padding: 0 24px; }
  .nav-links li:not(:last-child) { display: none; }
  .footer-inner { flex-direction: column; gap: 20px; }
  .section      { padding: 48px 0; }
  .setup-steps::before { display: none; }
  .setup-step   { flex-direction: column; gap: 12px; }
}

@media (prefers-reduced-motion: reduce) {
  * { transition: none !important; }
}
```

### Nav active state + tab JavaScript

Add this JavaScript before `</body>`. Copy the tab switching and
nav active state code exactly from sample.html — the switchTab
function and the IntersectionObserver for nav link highlighting:

```javascript
/* ── TABS ── */
function switchTab(btn, id) {
  const wrap = btn.closest('div');
  const container = wrap.parentElement;
  wrap.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  container.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
  const panel = container.querySelector('#tab-' + id);
  if (panel) panel.classList.add('active');
}

/* ── NAV ACTIVE ── */
const sections = document.querySelectorAll('section[id]');
const navLinks = document.querySelectorAll('.nav-links a[href^="#"]');
const observer = new IntersectionObserver(entries => {
  entries.forEach(e => {
    if (e.isIntersecting) {
      const id = e.target.id;
      navLinks.forEach(a => {
        a.style.color = a.getAttribute('href') === '#' + id
          ? 'var(--head)' : '';
      });
    }
  });
}, { threshold: 0.3 });
sections.forEach(s => s.id && observer.observe(s));
```

---

## Mapping guide — docs/index.html structure to new classes

After reading docs/index.html, map each existing structural element
to the new class system. Use this as a guide — adapt based on what
you actually find in the file:

```
Existing element              →  New class / structure
──────────────────────────────────────────────────────
<nav>                         →  nav with .nav-inner, .nav-logo, .nav-links
<h2> section titles           →  add class="section-title"
Section intro paragraphs      →  add class="section-sub"
Before each h2                →  add <div class="section-label">
<section> wrappers            →  add class="section"
<pre><code> blocks            →  wrap in <div class="code-block">
Inline <code>                 →  styled by :not(pre) > code rule
.sig / command signatures     →  .sig with .prog span
.flags tables                 →  .flags with .flagname/.flagdesc cells
.cmd-block wrappers           →  .cmd-block
.desc paragraphs              →  .desc
.note boxes                   →  .note (lime left border)
.warn boxes                   →  .warn (yellow left border)
Philosophy / concepts table   →  .concept-table
Walkthrough steps             →  .setup-steps > .setup-step
Install method blocks         →  .install-method
Tabbed install panels         →  .install-tabs + .tab-btn + .tab-panel
Any existing .badge           →  .badge-experimental (yellow style)
CTA buttons                   →  .btn-primary or .btn-ghost
<footer>                      →  footer with .footer-inner, .footer-logo,
                                 .footer-links
```

---

## Rules — non-negotiable

**Content rules:**
- Every word of text in docs/index.html must appear in the output unchanged
- Every `id="..."` attribute must be preserved exactly — these are nav anchors
- Every `href="..."` must be preserved exactly — do not change any link
- Every command, flag name, flag description must be unchanged
- Every code example must be unchanged

**Style rules:**
- Use only CSS variables from the `:root` block — no raw hex values elsewhere
- Do not add color values not in the design system
- Do not add fonts beyond Space Grotesk and JetBrains Mono
- Lime (`--lime`) is used for: the logo, section labels, code highlights,
  step numbers, active states, and `.prog` command names only
- Lime is NEVER used for body text or prose descriptions
- `--muted` is for descriptions and secondary text
- `--head` is for primary headings and important labels
- `--body` is for normal prose

**What to do if content in docs/index.html has no clear mapping:**
Read the surrounding context and pick the closest matching class from
the design system. For elements with no good match, use minimal inline
styles derived from the CSS variables only. Do not introduce new CSS
classes — extend existing ones.

---

## Accuracy pass after writing

```bash
# 1. All section anchor IDs preserved
grep 'id="' docs/index.html | grep -v "tab-\|term-\|cursor"

# 2. All nav hrefs preserved
grep "nav-links" docs/index.html -A 30 | grep href

# 3. No raw hex values in CSS (only CSS variables should be used
#    except where absolutely necessary for rgba() transparency)
grep -n "#[0-9A-Fa-f]\{3,6\}" docs/index.html | grep -v "root\|C792EA\|7DBBE6\|FF5F57\|FEBC2E\|28C840\|FF6B6B\|0A0E14"

# 4. Google Fonts link present
grep "fonts.googleapis.com" docs/index.html

# 5. Space Grotesk and JetBrains Mono referenced in CSS
grep "Space Grotesk\|JetBrains Mono" docs/index.html

# 6. Key content still present
grep -c "declaration of intent" docs/index.html
grep -c "gitcollect clone" docs/index.html
grep -c "gitcollect member add" docs/index.html

# 7. No placeholder text
grep -in "yourusername\|TODO\|FIXME\|placeholder" docs/index.html
```

Report the output of every check. Fix any issue before finishing.

---

## Commit message when done

```
docs: apply lime-on-dark developer theme to index.html

- Space Grotesk (prose) + JetBrains Mono (code/UI) typography
- #0A0E14 dark background with lime #C8FF57 accent
- Sticky nav with blur backdrop and lime logo
- Section labels in mono uppercase with lime decorative line
- Code blocks with ink2 surface and wire border
- Inline code with lime color and subtle lime background tint
- Command signatures with lime .prog and purple flag names
- Setup steps with numbered circles and connecting vertical line
- Concept table with lime left-column mono labels
- Note boxes with lime left border, warn boxes with yellow
- Install method cards with ink2 surface
- Tab panels for platform-specific install blocks
- Footer with space-between logo and links layout
- Mobile responsive — single column below 768px
- Content unchanged — visual theme applied only
```
