# gitcollect docs — HTML design system reference

> Paste this into any agent session that needs to add or update
> content in docs/index.html. The agent must follow every rule here
> exactly — do not introduce new CSS classes, new color variables,
> new layout patterns, or new fonts. The goal is that a new section
> is visually indistinguishable from an existing one.
>
> Before writing any HTML, read docs/index.html lines 1–170 to see
> the full CSS. Every rule below is derived from that CSS — this
> document is a summary, not a replacement for reading the source.

---

## Core design values

The page has one job: make a developer trust and understand the tool
in under 60 seconds. Every design decision serves that goal.

- Dark theme, always. No light mode toggle.
- Monospace for everything code-related. Sans-serif for prose only.
- Dense but not cramped. Information is the decoration.
- One accent color used sparingly. Not decorative — functional only.
- Consistent rhythm. Every section feels like it belongs to the same page.

---

## Color system

Read the `:root` block in docs/index.html to get the exact hex values.
Never hardcode colors — always use the CSS variables below.

```css
var(--bg)        /* page background — near black */
var(--surface)   /* card / section background — slightly lighter than bg */
var(--border)    /* subtle border color */
var(--text)      /* primary text — off-white, high contrast on dark bg */
var(--muted)     /* secondary text — lower contrast, used for descriptions */
var(--accent)    /* single accent color — teal/cyan — used for: active nav
                    items, command name highlights, key labels, links */
var(--yellow)    /* warning color — used only for [experimental] badge */
var(--green)     /* success/checkmark color — used in terminal output */
var(--red)       /* error color — used in terminal output only */
var(--code-bg)   /* code block background — darker than surface */
```

If you need a color not in this list, you do not need it.
Do not introduce new CSS variables. Do not use raw hex values.

---

## Typography

```css
/* Body prose */
font-family: system-ui, -apple-system, sans-serif;
font-size: 1rem;
line-height: 1.7;
color: var(--text);

/* All code, commands, filenames, flags, CLI output */
font-family: 'JetBrains Mono', 'Fira Code', monospace;

/* Section headings (h2) */
font-size: 1.4rem;
font-weight: 600;
color: var(--text);

/* Command signature headings (h3 equivalent) */
font-size: 1rem;
font-weight: 600;
font-family: monospace;
color: var(--accent);

/* Descriptions, secondary text */
color: var(--muted);
font-size: 0.95rem;
```

Never use font-weight above 600.
Never use font-size below 0.8rem for visible text.
Never use a serif font anywhere on the page.

---

## Layout

The page is a single column, max-width constrained, centered.

```css
.container {
  max-width: 860px;
  margin: 0 auto;
  padding: 0 24px;
}
```

Sections stack vertically with consistent spacing between them.
There are no multi-column layouts in the main content area.
The nav is the only element that spans full width.

---

## Nav

Sticky top nav with links to all major sections.
Background: var(--bg) with slight transparency + backdrop-filter blur.
Nav links: var(--muted) by default, var(--accent) when active or on hover.
No borders, no shadows, no background change on scroll.

To add a new section to the nav:
1. Add an `<a href="#your-section-id">Label</a>` to the nav list
2. Add `id="your-section-id"` to the corresponding `<section>` element
3. Keep the label short — 1-3 words max

---

## Section structure

Every major section follows this exact pattern:

```html
<section id="section-id">
  <div class="container">
    <h2>Section title</h2>
    <p class="lead">One sentence explaining what this section covers.</p>

    <!-- section content here -->

  </div>
</section>
```

The `<h2>` is the only heading at section level.
The `.lead` paragraph is optional but used on most sections.
Do not add decorative dividers, icons, or badges to section headings.

---

## Command blocks

Each command in the reference uses this exact structure:

```html
<div class="cmd-block">
  <p class="sig">
    <span class="prog">gitcollect</span> commandname &lt;required&gt; [optional]
  </p>
  <p class="desc">
    Plain prose description of what the command does.
    Use <code>inline code</code> for filenames, flags, and command names.
    Use <strong>bold</strong> only for genuinely critical warnings.
  </p>
  <table class="flags">
    <tr>
      <td class="flagname">--flag-name</td>
      <td class="flagdesc">what this flag does</td>
    </tr>
  </table>
</div>
```

Rules for command blocks:
- `<span class="prog">gitcollect</span>` wraps only the binary name,
  not the subcommand. This applies the accent color to the binary name.
- The signature (`<p class="sig">`) is monospace, slightly larger.
- The description (`<p class="desc">`) is sans-serif prose.
- The flags table (`<table class="flags">`) has exactly two columns:
  flag name (left, monospace, fixed width) and description (right, prose).
- Commands with no flags omit the table entirely.
- Do not add a heading (`<h3>`, `<h4>`) before each command block.
  The signature line IS the heading.

---

## Code blocks (terminal output and examples)

For multi-line terminal output or shell commands:

```html
<pre><code>gitcollect clone cybersecurity
✓ Access verified (alice · groups: red-team)
[1/3] Cloning pen-test-tools...   ✓ done  (1.2s)
</code></pre>
```

For inline terminal output with syntax coloring:

```html
<pre><code><span class="c"># this is a comment</span>
gitcollect init cybersecurity
<span class="out">✓ Created collection: cybersecurity (private)</span>
</code></pre>
```

Available inline spans inside `<pre><code>`:
- `<span class="c">` — comment (muted color, italic)
- `<span class="out">` — terminal output (slightly muted, not a command)
- `<span class="err">` — error output (var(--red))
- `<span class="ok">` — success output (var(--green))

Do not add line numbers. Do not add copy buttons. Do not add
a filename header above code blocks. Keep it minimal.

---

## Tables (non-flag tables)

For concept/comparison tables like the Philosophy table:

```html
<table class="concept-table">
  <tr>
    <td><strong>Concept name</strong></td>
    <td>Definition prose here. Use <code>inline code</code> for
    paths and commands.</td>
  </tr>
</table>
```

Rules:
- No `<thead>` — the bold first column acts as the header
- No zebra striping — border-bottom on each `<tr>` provides rhythm
- Left column is fixed-width (~160px), right column is fluid
- No more than two columns in any table on this page

---

## Note / callout boxes

For important warnings or "coming soon" notices:

```html
<p class="note">
  A <code>gitcollect fetch</code> command for sharing collections
  by URL is planned for a future release.
</p>
```

Or for stronger warnings:

```html
<p class="warn">
  Content of the warning here.
</p>
```

Rules:
- Notes use a left border in var(--accent) color
- Warnings use a left border in var(--yellow)
- Never use a full box/card for notes — left border only
- Keep note text short — one or two sentences maximum
- Do not use note boxes for normal prose

---

## Experimental badge

Used only on the `activity` command. Inline in the signature line:

```html
<span style="font-size:0.75rem;font-weight:400;color:var(--yellow);
border:1px solid var(--yellow);border-radius:4px;
padding:1px 6px;vertical-align:middle;">experimental</span>
```

And a note line immediately below the signature:

```html
<p style="color:var(--yellow);font-size:0.85rem;margin:0 0 8px;
font-style:italic;">[experimental] — output format and flag names
may change in a future release.</p>
```

Do not add this badge to any other command.
If a new feature needs a similar label, use the exact same pattern.

---

## Install section specifics

The install section (id="install") contains multiple method blocks.
Each method uses this structure:

```html
<div class="install-method">
  <h3>Method name</h3>
  <p class="desc">One sentence explaining who this method is for.</p>
  <pre><code><!-- commands here --></code></pre>
  <p class="note"><!-- optional note --></p>
</div>
```

The `<h3>` is allowed inside the install section only — it is not
used in the command reference sections.

Platform-specific blocks (Linux, macOS, Windows) use:

```html
<div class="platform-block">
  <h4>Platform name</h4>
  <pre><code><!-- platform-specific commands --></code></pre>
  <p class="note"><!-- platform note if needed --></p>
</div>
```

`<h4>` is allowed inside platform blocks only.

---

## Things that do not exist on this page

Do not add any of these — they are not part of the design system
and will look out of place:

- Gradient backgrounds or gradient text
- Box shadows or drop shadows
- Hover animations beyond simple color transitions
- Icons or emoji in headings or command signatures
- Sidebar navigation
- Tabs or accordion components
- Progress bars or step indicators
- Avatar images or user photos
- Any image other than the favicon
- Toast/snackbar notifications
- Tooltip components
- Modal dialogs
- Any JavaScript beyond what already exists in the page

---

## Adding a new command to the reference

When adding a new command (e.g. `gitcollect transfer` or
`gitcollect scale`), place it in the correct group section.
Do not create a new section unless you are adding an entire new
group that doesn't fit any existing section.

```html
<!-- Add inside the correct <section> after the last existing cmd-block -->
<div class="cmd-block">
  <p class="sig">
    <span class="prog">gitcollect</span> transfer &lt;collection&gt; &lt;new-owner&gt;
  </p>
  <p class="desc">
    Transfer ownership of a collection to another member.
    The previous owner becomes a regular member and retains access.
    Requires typing the new owner's username to confirm — this
    action cannot be undone by the previous owner.
  </p>
  <!-- no flags table if the command has no flags -->
</div>
```

---

## Adding a new section to the page

When adding an entirely new section:

1. Add the `<a href="#new-id">Label</a>` link to the nav
2. Use the standard section structure shown above
3. Use only the CSS classes that already exist:
   `cmd-block`, `sig`, `prog`, `desc`, `flags`, `flagname`,
   `flagdesc`, `concept-table`, `note`, `warn`, `lead`,
   `install-method`, `platform-block`
4. Do not add new CSS classes. If you need a style that
   doesn't exist, use a minimal inline style only — keep
   inline styles to a single property when possible
5. Do not add new `<style>` blocks

---

## Accuracy rules (non-negotiable)

These apply to every edit to docs/index.html:

- Every command shown must exist in the actual binary
  (verify with `gitcollect --help` or `grep` in cmd/)
- Every flag shown must match the actual flag name and separator style
  (comma-separated vs space-separated — verify in cmd/*.go)
- Every path shown (~/.gitcollect/, etc.) must match the real paths
  from internal/config/config.go
- --groups and --users are COMMA-separated: `--groups red-team,sre`
  NEVER space-separated: `--groups "red-team sre"` ← wrong
- --pick is SPACE-separated (quoted): `--pick "r1 r2"`
  NEVER comma-separated: `--pick r1,r2` ← wrong
- Binary name is `gitcollect` (from project_name in .goreleaser.yaml)
- Module path is `github.com/alby-tomy/gitcollect` (from go.mod)
- Go version required is 1.26+ (from go.mod)

Before finishing any edit session, run:
```bash
grep -n "\[BINARY\]\|\[real\|\[TODO\|\[FIXME\|placeholder" docs/index.html
```
If this returns anything, fix it before committing.

---

## Commit message format for docs changes

```
docs: <brief description of what changed>
```

Examples:
```
docs: add transfer and scale commands to reference
docs: add group admin section
docs: fix --namespace flag in init table
docs: add full install section per OS
```

Never use "update docs" or "fix docs" — be specific about what changed.
