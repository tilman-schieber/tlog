// The desktop outliner. Every operation is a call into the Go core; this file
// draws the result and translates keystrokes, and decides nothing on its own.
//
// Wails exposes the bound methods as window.go.main.API, so no generated
// bindings and no build step are needed.

const api = () => window.go.main.API;

const $ = (id) => document.getElementById(id);

let page = null; // the current PageView; a stand-in while on the journals page
let focusOffset = null; // where to put the caret after a redraw
let focusCaret = null; // how far into that block, null meaning at its end
let focusRel = null; // which file that block is in, when more than one is shown
let completion = null;

// The journals page is every day, newest first, each still its own file. While
// it is open, feed holds the days and `page` is a stand-in named JOURNALS, so
// that history, the recent list and the title all have something to say.
let JOURNALS = "journals"; // asked of the core at startup
let feed = null; // { rels: every day, days: the DayViews opened so far }

// viewOf is the page a block belongs to, as it now stands. On the journals page
// that is the block's own day: every edit is addressed through it, with that
// day's hash, so a change can only ever reach the day it was made on.
function viewOf(rel) {
  if (feed) {
    const d = feed.days.find((d) => d.page.rel === rel);
    return d ? d.page : null;
  }
  return page && page.rel === rel ? page : null;
}

// --- plumbing ---------------------------------------------------------------

// What the window has said, newest last. A failed push used to appear for six
// seconds and then be gone with no way to get it back — if you were looking at
// the other screen when it happened, it never happened.
const notices = [];

// say puts a line in the corner. A notice fades, because it is the receipt for
// something you just did and watched happen. An error does not, because the
// thing it is telling you about is the thing you were not watching.
function say(text, kind) {
  const line = { text: String(text), kind: kind || "note", at: new Date() };
  notices.push(line);
  if (notices.length > 200) notices.shift();

  const box = $("error");
  box.className = kind === "error" ? "bad" : "";
  box.textContent = line.text;
  box.hidden = false;
  clearTimeout(say.timer);
  if (kind !== "error") say.timer = setTimeout(() => (box.hidden = true), 4000);
  renderLog();
}

function fail(err) {
  say(err && err.message ? err.message : err, "error");
}

// The log is everything said so far, reachable from the corner. Dismissing a
// message should not be the same as destroying it.
function renderLog() {
  const list = $("loglist");
  if (!list) return;
  list.innerHTML = "";
  for (const n of [...notices].reverse()) {
    const li = document.createElement("li");
    li.className = "logline " + n.kind;
    const t = String(n.at.getHours()).padStart(2, "0") + ":" +
      String(n.at.getMinutes()).padStart(2, "0");
    li.innerHTML = `<span class="when">${t}</span><span>${escapeHTML(n.text)}</span>`;
    list.appendChild(li);
  }
  if (notices.length === 0) {
    const li = document.createElement("li");
    li.className = "hint";
    li.textContent = "Nothing has gone wrong yet.";
    list.appendChild(li);
  }
}

async function call(fn) {
  try {
    return await fn();
  } catch (err) {
    fail(err);
    return null;
  }
}

// show draws a page. Arriving somewhere is also how the recent list learns
// where you are: every way of navigating ends up here, so no route can forget.
function show(p) {
  if (!p) return;
  const arriving = !page || page.rel !== p.rel;
  if (p.rel !== JOURNALS) feed = null;
  else if (p.feed) feed = p.feed;
  page = p;
  render();
  if (arriving) arrived(p.rel);
}

// An edit returns the page plus where the block ended up, because rewriting the
// file moves every offset after it.
//
// On the journals page only the day that was written is redrawn, so typing in
// another day is left alone. An edit to a page no longer on screen — a save on
// blur that finished after a click had already gone elsewhere — is not drawn
// at all: it would take you back to where you had just left.
function applied(edit, caret = null) {
  if (!edit) return;
  const rel = edit.page.rel;
  if (feed && feed.days.some((d) => d.page.rel === rel)) {
    feed.days.find((d) => d.page.rel === rel).page = edit.page;
    focusRel = rel;
    focusOffset = edit.offset;
    focusCaret = caret;
    renderDay(rel);
    takeFocus();
  } else if (!feed && (!page || page.rel === rel)) {
    focusRel = rel;
    focusOffset = edit.offset;
    focusCaret = caret;
    show(edit.page);
  }
  refreshIndex();
  checkSync();
}

// takeFocus puts the caret where the last edit asked for it.
function takeFocus() {
  if (focusOffset === null) return;
  const el = [...document.querySelectorAll("#outline .text[data-offset]")].find(
    (t) => Number(t.dataset.offset) === focusOffset && (!focusRel || t.dataset.rel === focusRel)
  );
  if (el) placeCaretAt(el, focusCaret);
  focusOffset = null;
  focusCaret = null;
  focusRel = null;
}

// A push happens in the background after a commit; if it failed, say so.
async function checkSync() {
  const st = await call(() => api().Sync());
  if (st && st.lastError) fail("Not pushed: " + st.lastError);
}

// --- rendering --------------------------------------------------------------

// Quotes are escaped as well as angle brackets, and that is not optional:
// decorateInline escapes the whole block once and then splices pieces of the
// result into quoted attributes — data-page, data-url, data-lang. A page name
// containing a quote would otherwise close the attribute and let the rest of
// the name become attributes of that span, which is an event handler away from
// running. Your own notes are the injection vector, and notes get pasted into.
const escapeHTML = (s) =>
  s.replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c])
  );

// Rendering a block for reading. The block being edited shows raw text instead:
// the caret has to land where the characters actually are, and what you type is
// what is stored. Nothing here changes how anything is saved — every construct
// below is ordinary markdown living inside a block's text.

const RE = {
  fence: /^\s*```/,
  heading: /^\s*(#{1,6})\s+(.*)$/,
  rule: /^\s*([-*_])\1{2,}\s*$/,
  quote: /^\s*>\s?(.*)$/,
  ordered: /^(\s*)(\d+)[.)]\s+(.*)$/,
  bullet: /^(\s*)[-*+]\s+(.*)$/,
  row: /^\s*\|.*\|\s*$/,
  sep: /^\s*\|[\s:|-]+\|\s*$/,
};

function decorate(text, fences, embeds = []) {
  // A missing field must not take the page down with it. Three separate
  // crashes have started as one absent property and ended as a window that
  // draws nothing at all, and a renderer is the wrong place to find out.
  if (typeof text !== "string") text = text == null ? "" : String(text);
  // References are numbered across the whole block but drawn a line at a time,
  // so the count is shared rather than restarted for each line.
  const cursor = { i: 0 };
  const out = [];
  const lines = text.split("\n");
  let i = 0;
  let fence = 0;
  let plain = [];

  const flush = () => {
    if (!plain.length) return;
    out.push(
      `<span class="ln">${plain.map((l) => decorateInline(l, embeds, cursor)).join("\n")}</span>`
    );
    plain = [];
  };

  // Collect a run of consecutive lines matching a pattern.
  const run = (test) => {
    const got = [];
    while (i < lines.length && test(lines[i])) got.push(lines[i++]);
    return got;
  };

  while (i < lines.length) {
    const line = lines[i];

    // A fenced block is verbatim: a # comment inside it is not a tag, and an
    // asterisk in a glob is not emphasis.
    if (RE.fence.test(line)) {
      flush();
      const lang = line.replace(/^\s*```/, "").trim();
      const body = [];
      i++;
      while (i < lines.length && !RE.fence.test(lines[i])) body.push(lines[i++]);
      if (i < lines.length) i++; // the closing fence
      // The core scans fences with the same rule, so the nth here is the nth
      // there: already highlighted, or already parsed into rows.
      out.push(fenceHTML(lang, body.join("\n"), fences && fences[fence++]));
      continue;
    }

    // A table needs a separator row under its header to be a table at all.
    if (RE.row.test(line) && i + 1 < lines.length && RE.sep.test(lines[i + 1])) {
      flush();
      out.push(tableHTML(run((l) => RE.row.test(l))));
      continue;
    }

    if (RE.quote.test(line)) {
      flush();
      const body = run((l) => RE.quote.test(l)).map((l) =>
        decorateInline(l.match(RE.quote)[1])
      );
      out.push(`<blockquote>${body.join("\n")}</blockquote>`);
      continue;
    }

    if (RE.ordered.test(line)) {
      flush();
      const rows = run((l) => RE.ordered.test(l));
      const from = Number(rows[0].match(RE.ordered)[2]);
      const items = rows.map((l) => `<li>${decorateInline(l.match(RE.ordered)[3])}</li>`);
      out.push(`<ol${from === 1 ? "" : ` start="${from}"`}>${items.join("")}</ol>`);
      continue;
    }

    if (RE.bullet.test(line)) {
      flush();
      const items = run((l) => RE.bullet.test(l)).map(
        (l) => `<li>${decorateInline(l.match(RE.bullet)[2])}</li>`
      );
      out.push(`<ul>${items.join("")}</ul>`);
      continue;
    }

    if (RE.heading.test(line)) {
      const m = line.match(RE.heading);
      flush();
      out.push(`<b class="h h${m[1].length}">${decorateInline(m[2])}</b>`);
      i++;
      continue;
    }

    if (RE.rule.test(line)) {
      flush();
      out.push("<hr>");
      i++;
      continue;
    }

    plain.push(line);
    i++;
  }

  flush();
  return out.join("");
}

// A ```csv fence is drawn as a table. Pipe tables are miserable to type and
// worse to edit; rows of values are not, and the file stays ordinary markdown.
function fenceHTML(lang, code, prepared) {
  if (prepared && prepared.rows) return gridHTML(prepared.rows, lang);

  const body =
    prepared && prepared.tokens
      ? prepared.tokens
          .map((t) =>
            t.kind
              ? `<span class="t-${t.kind}">${escapeHTML(t.text)}</span>`
              : escapeHTML(t.text)
          )
          .join("")
      : escapeHTML(code);

  return (
    `<pre class="code"${lang ? ` data-lang="${escapeHTML(lang)}"` : ""}>` +
    `<code>${body}</code></pre>`
  );
}

// gridHTML draws parsed rows; the first is the header.
function gridHTML(rows, lang) {
  const head = rows[0] || [];
  const th = head.map((c) => `<th>${decorateInline(c)}</th>`).join("");
  const tr = rows
    .slice(1)
    .map((r) => `<tr>${r.map((c) => `<td>${decorateInline(c)}</td>`).join("")}</tr>`)
    .join("");
  return (
    `<table class="grid" data-lang="${escapeHTML(lang)}">` +
    `<thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table>`
  );
}

function tableHTML(rows) {
  const cells = (l) =>
    l.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
  const head = cells(rows[0]);
  const th = head.map((c) => `<th>${decorateInline(c)}</th>`).join("");
  const tr = rows
    .slice(2)
    .map((r) => `<tr>${cells(r).map((c) => `<td>${decorateInline(c)}</td>`).join("")}</tr>`)
    .join("");
  return `<table><thead><tr>${th}</tr></thead><tbody>${tr}</tbody></table>`;
}

// Inline syntax within one line. Links are lifted out before anything else
// runs: a #tag inside brackets describes the page being linked to, and an
// underscore inside a page name is part of the name.
// embeds are the block's resolved references and cursor counts how many have
// been drawn. Neither is defaulted in the signature: the test harness lifts
// these functions by matching braces from the first one, and a brace in the
// parameter list would end the function before it began.
function decorateInline(text, embeds, cursor) {
  embeds = embeds || [];
  cursor = cursor || { i: 0 };
  const held = [];
  const hold = (html) => {
    held.push(html);
    return "\u0000" + (held.length - 1) + "\u0000";
  };

  let out = escapeHTML(text);

  // [[Page]], [[Page#^anchor]], [[Page #tag]] — show the name and what it is.
  //
  // A block reference shows the block. It used to show the page name, which
  // told the reader nothing: you point at one block out of a hundred and are
  // shown the word "Timetable". The resolved text comes from the core, in the
  // order the references appear, so they are consumed as they are met.
  out = out.replace(/\[\[([^\[\]]+)\]\]/g, (_, inner) => {
    const tags = [];
    let name = inner.replace(/\s+#([A-Za-z][\w/-]*)/g, (m, t) => {
      tags.push(t);
      return "";
    });
    const anchor = name.includes("#^") ? name.split("#^")[1].trim() : "";
    name = name.split("#^")[0].trim();

    if (anchor) {
      const e = embeds[cursor.i++];
      if (e && !e.missing) {
        // The referenced text is decorated on its own, with no references of
        // its own to follow: one hop, so a block that points back cannot loop.
        return hold(
          `<span class="ref" data-page="${name}" data-anchor="${anchor}">` +
            decorateInline(e.text) +
            `</span>`
        );
      }
      if (e) {
        // Resolved, and there is nothing behind it. Say so rather than drawing
        // a page name that is a lie — a dangling pointer the reader cannot see
        // is the worse one.
        return hold(
          `<span class="ref missing" data-page="${name}">[[${name}#^${anchor}]]</span>`
        );
      }
      // Nobody resolved anything: a backlink row or an agenda line, which are
      // drawn from a view that does not carry the text behind a reference.
      // The page name is what is honestly known there.
    }

    let html = `<span class="link" data-page="${name}">${name}</span>`;
    for (const t of tags) html += `<span class="tag" data-tag="${t}">#${t}</span>`;
    return hold(html);
  });

  // An image before a link, so ![alt](src) is not read as a link.
  out = out.replace(/!\[([^\[\]]*)\]\(([^()\s]+)\)/g, (_, alt, src) =>
    hold(`<img src="${src}" alt="${alt}" loading="lazy">`)
  );
  out = out.replace(/\[([^\[\]]+)\]\(([^()\s]+)\)/g, (_, label, url) =>
    hold(`<span class="link" data-url="${url}">${label}</span>`)
  );

  // A bare URL is a link too, including the file:// ones `att` writes.
  out = out.replace(/(^|\s)((?:https?|file|mailto):[^\s<>]+)/g, (_, pre, url) =>
    pre + hold(`<span class="link" data-url="${url}">${url.replace(/^file:\/\//, "")}</span>`)
  );

  out = out.replace(/`([^`]+)`/g, (_, code) => hold(`<code>${code}</code>`));
  out = out.replace(/\*\*([^*]+)\*\*/g, (_, b) => hold(`<strong>${b}</strong>`));
  out = out.replace(/(^|\W)\*([^*\s][^*]*)\*/g, (_, pre, em) => pre + hold(`<em>${em}</em>`));
  out = out.replace(/(^|\W)_([^_\s][^_]*)_/g, (_, pre, em) => pre + hold(`<em>${em}</em>`));
  out = out.replace(/~~([^~]+)~~/g, (_, d) => hold(`<del>${d}</del>`));

  // A bare #tag in prose is a mention of that tag.
  out = out.replace(
    /(^|\s)(#[A-Za-z][\w/-]*)/g,
    (m, pre, tag) => `${pre}<span class="tag" data-tag="${tag.slice(1)}">${tag}</span>`
  );

  return out.replace(/\u0000(\d+)\u0000/g, (_, i) => held[Number(i)]);
}

function render() {
  $("title").textContent = page.title;
  $("daynav").style.visibility = page.isJournal || feed ? "visible" : "hidden";

  const tags = $("pagetags");
  tags.innerHTML = "";
  (page.tags || []).forEach((t) => {
    const el = document.createElement("span");
    el.className = "pill";
    el.textContent = "#" + t;
    el.onclick = () => open_(t);
    tags.appendChild(el);
  });
  // The other names this page answers to, so a [[link]] that worked is not a
  // mystery when you arrive.
  if ((page.aliases || []).length) {
    const el = document.createElement("span");
    el.className = "alsoknown";
    el.textContent = "auch: " + page.aliases.join(", ");
    tags.appendChild(el);
  }

  // Drawing a page is also how the agenda is left: every way of navigating
  // ends up here, so putting it in one place means no route can forget.
  showOutline();

  renderOutline();
  renderTagged();
  renderRefs();
  markActive();
  updateNav();
  drawRecent();
  takeFocus();
}

// Which blocks are folded, per page, by the text of the block rather than by
// where it sits. The outliner learned this the hard way: a position renumbers
// the moment anything moves, and the fold ends up on whatever took that slot.
// Text survives a move, an indent and a delete, which is every operation that
// renumbers. Editing the text loses that block's own fold and no other.
//
// Folding is view state and the core is not told about it — an anchor written
// to mark a folded row would be a file changed by looking at it.
const folded = new Map(); // rel -> Set of block texts

function foldsFor(rel) {
  if (!folded.has(rel)) folded.set(rel, new Set());
  return folded.get(rel);
}

// A fold is kept per file, so a day folded on the journals page is folded on
// its own page too, and the other way round.
function isFolded(rel, b) {
  return foldsFor(rel).has(b.text);
}

function toggleFold(rel, b) {
  const set = foldsFor(rel);
  if (set.has(b.text)) set.delete(b.text);
  else set.add(b.text);
  if (feed) renderDay(rel);
  else renderOutline();
}

// hiddenUnder walks the flat block list and reports which offsets are inside a
// folded subtree. The list is flat with a depth on each block, so a subtree is
// the run that follows a block at a greater depth than it.
function hiddenUnder(rel, blocks) {
  const hide = new Set();
  for (let i = 0; i < blocks.length; i++) {
    if (!blocks[i].hasChildren || !isFolded(rel, blocks[i])) continue;
    for (let j = i + 1; j < blocks.length && blocks[j].depth > blocks[i].depth; j++) {
      hide.add(blocks[j].offset);
    }
  }
  return hide;
}

function renderOutline() {
  const out = $("outline");
  out.innerHTML = "";
  if (feed) return renderFeed(out);
  renderBlocks(out, page);
}

// renderBlocks draws one file's blocks: the page, or one day of the journals
// page. Every handler asks for the file as it stands when it fires, not as it
// was drawn, because another day may have been redrawn in between.
function renderBlocks(out, pv) {
  const rel = pv.rel;
  const cur = () => viewOf(rel) || pv;

  if (!pv.blocks || pv.blocks.length === 0) {
    const hint = document.createElement("div");
    hint.className = "refline";
    hint.textContent = "Empty — click here to start writing";
    hint.onclick = async () =>
      applied(await call(() => api().AppendBlock(rel, cur().hash, "")));
    out.appendChild(hint);
    return;
  }

  const hidden = hiddenUnder(rel, pv.blocks);

  pv.blocks.forEach((b) => {
    if (hidden.has(b.offset)) return;
    const row = document.createElement("div");
    row.className = "block" + (b.task === "done" ? " done" : "");
    row.style.marginLeft = b.depth * 22 + "px";

    if (b.task) {
      const check = document.createElement("span");
      check.className = "check";
      check.textContent = b.task === "done" ? "☑" : "☐";
      check.title = b.task === "done" ? "Not done after all" : "Done";
      check.onclick = async () =>
        applied(await call(() => api().ToggleTask(rel, b.offset, cur().hash)));
      row.appendChild(check);
    } else {
      // The bullet folds what is under it, and does nothing else. It used to
      // turn the block into a task, which meant a stray click on the wrong
      // pixel silently rewrote a note into a checkbox. A block becomes a task
      // because you said /todo or gave it a /deadline — never because you
      // clicked near it.
      const kids = b.hasChildren;
      const shut = kids && isFolded(rel, b);
      const bullet = document.createElement("span");
      bullet.className = "bullet" + (kids ? " haskids" : "") + (shut ? " folded" : "");
      bullet.textContent = "●";
      if (kids) {
        bullet.title = shut ? "Unfold" : "Fold";
        bullet.onclick = () => toggleFold(rel, b);
      }
      row.appendChild(bullet);
    }

    if (b.due) {
      const chip = document.createElement("span");
      chip.className = "due " + (b.dueState ? "t-" + b.dueState : "done");
      chip.textContent = "⏰ " + b.due.slice(8) + "." + b.due.slice(5, 7) + ".";
      chip.title = b.dueLabel || b.due;
      row.appendChild(chip);
    }

    const text = document.createElement("div");
    text.className = "text";
    text.contentEditable = "plaintext-only";
    text.spellcheck = false;
    text.dataset.offset = b.offset;
    text.dataset.rel = rel;
    text.dataset.raw = b.text;
    // The checkbox is drawn beside the block, so the marker is not repeated in
    // the text. What is stored still has it, and focusing shows that.
    text.innerHTML = decorate(b.task ? b.body : b.text, b.fences, b.embeds) || "<br>";
    wireBlock(text, b, pv);
    row.appendChild(text);

    out.appendChild(row);

    // A block that is nothing but a reference brings the subtree with it —
    // which is the whole reason to embed rather than to refer. The rows are
    // the other page's, so they are shown and not edited: editing them here
    // would be editing a file this page does not have open.
    (isFolded(rel, b) ? [] : b.embedKids || []).forEach((kid) => {
      const line = document.createElement("div");
      line.className = "block embedded";
      line.style.marginLeft = (b.depth + kid.depth) * 22 + "px";

      const bullet = document.createElement("span");
      bullet.className = "bullet";
      bullet.textContent = "●";
      line.appendChild(bullet);

      const kt = document.createElement("div");
      kt.className = "text";
      kt.innerHTML = decorate(kid.text) || "<br>";
      kt.onclick = () => openBlock(kid);
      line.appendChild(kt);
      out.appendChild(line);
    });

    (b.props || []).filter((p) => p.key.toLowerCase() !== "deadline").forEach((p) => {
      const pr = document.createElement("div");
      pr.className = "props";
      pr.style.marginLeft = b.depth * 22 + 20 + "px";
      pr.textContent = `${p.key}:: ${p.value}`;
      out.appendChild(pr);
    });
  });
}

// --- the journals page ------------------------------------------------------
//
// Every day under its own date, newest first, today at the top whether it has
// a file or not. Each day is drawn by renderBlocks from its own PageView, so
// it is edited exactly as on its own page; the core rewrites one file per
// operation, so indenting, moving or joining across a date is refused as it is
// at the edge of any page. Days are opened a fortnight at a time as you scroll
// down to them.

const FEED_CHUNK = 14;

// The stand-in drawn as "the page" while the journals page is open. It carries
// the days, and show takes them from it, so the journals page and its days
// arrive together or not at all.
function feedPage(f) {
  return { rel: JOURNALS, title: "Journals", hash: "", isJournal: false,
           tags: [], blocks: [], tagged: [], refs: [], feed: f };
}

// loadFeed opens the journals page with at least n days.
async function loadFeed(n) {
  const rels = await call(() => api().JournalDays());
  if (!rels) return null;
  const want = Math.min(rels.length, Math.max(n || 0, FEED_CHUNK));
  const days = await call(() => api().OpenDays(rels.slice(0, want)));
  if (!days) return null;
  return { rels, days };
}

function renderFeed(out) {
  feed.days.forEach((d) => out.appendChild(daySection(d)));
  moreDaysButton(out);
}

// daySection draws one day: its date, which opens that day on its own, then
// its blocks.
function daySection(d) {
  const sec = document.createElement("section");
  sec.className = "day";
  sec.dataset.rel = d.page.rel;

  const head = document.createElement("h2");
  head.className = "dayhead";
  const date = document.createElement("span");
  date.className = "date";
  date.textContent = d.date;
  date.title = "Open this day on its own";
  date.onclick = () => openRel(d.page.rel);
  head.appendChild(date);
  const note = document.createElement("span");
  note.className = "note";
  note.textContent = [d.weekday, d.today ? "today" : d.ahead ? "ahead" : ""]
    .filter(Boolean).join(" · ");
  head.appendChild(note);
  sec.appendChild(head);

  const body = document.createElement("div");
  body.className = "dayblocks";
  renderBlocks(body, d.page);
  sec.appendChild(body);
  return sec;
}

function moreDaysButton(out) {
  if (!feed || feed.days.length >= feed.rels.length) return;
  const more = document.createElement("button");
  more.className = "moredays";
  more.textContent = "Older days";
  more.onclick = () => loadMoreDays();
  out.appendChild(more);
}

// renderDay redraws one day and leaves every other day — and anything being
// typed into one — exactly as it is.
function renderDay(rel) {
  const d = feed && feed.days.find((d) => d.page.rel === rel);
  const sec = [...document.querySelectorAll("#outline section.day")].find(
    (s) => s.dataset.rel === rel
  );
  if (!d || !sec) return renderOutline();
  sec.replaceWith(daySection(d));
}

// loadMoreDays opens the next fortnight below the last one. They are appended
// rather than the page redrawn, so the block being typed in keeps its caret.
let loadingDays = false;
async function loadMoreDays() {
  const f = feed;
  if (!f || loadingDays || f.days.length >= f.rels.length) return;
  loadingDays = true;
  const next = f.rels.slice(f.days.length, f.days.length + FEED_CHUNK);
  const more = await call(() => api().OpenDays(next));
  loadingDays = false;
  if (!more || feed !== f) return;
  f.days.push(...more);
  const out = $("outline");
  const btn = out.querySelector(".moredays");
  if (btn) btn.remove();
  more.forEach((d) => out.appendChild(daySection(d)));
  moreDaysButton(out);
}

// refreshFeed re-reads the journals page — a day written elsewhere, or links
// into a day that moved — keeping as many days open as there were.
async function refreshFeed() {
  const f = feed;
  if (!f) return;
  const fresh = await loadFeed(f.days.length);
  if (!fresh || feed !== f) return;
  feed = fresh;
  if (page && page.rel === JOURNALS) page.feed = fresh; // what show restores
  renderOutline();
}

// jumpDay scrolls to the next older (delta < 0) or newer day, which is what
// the previous and next day buttons mean on the journals page.
async function jumpDay(delta) {
  const secs = () => [...document.querySelectorAll("#outline section.day")];
  const top = $("main").getBoundingClientRect().top;
  let at = secs().findIndex((s) => s.getBoundingClientRect().bottom > top + 1);
  if (at < 0) at = 0;
  const to = at + (delta < 0 ? 1 : -1);
  if (to < 0) return;
  if (to >= secs().length) await loadMoreDays();
  const target = secs()[to];
  if (target) target.scrollIntoView({ block: "start" });
}

$("main").addEventListener("scroll", () => {
  if (!feed) return;
  const m = $("main");
  if (m.scrollTop + m.clientHeight > m.scrollHeight - 600) loadMoreDays();
});

// --- the recent list --------------------------------------------------------
//
// The header names the places you were last — four pages, the last journal day
// and the journals page, newest first — the same list from the same file as
// the outliner, so moving between the two keeps it. The core decides what is
// on it; this draws it and steps through it.

let recent = []; // [{rel, title}]
let cycling = false;
let cycleAt = 0;
let recentFailed = false;

// arrived tells the core where you are now. While cycling the order holds
// still, so the list catches up only once you stop (settleRecent).
async function arrived(rel) {
  if (cycling) return drawRecent();
  const r = await call(() => api().Visited(rel));
  if (r) {
    recent = r.places || [];
    // Said once: a state directory that cannot be written stays that way, and
    // a message on every page change would bury everything else.
    if (r.error && !recentFailed) {
      recentFailed = true;
      fail("Could not remember the recent pages: " + r.error);
    }
  }
  drawRecent();
}

function drawRecent() {
  const bar = $("recent");
  if (!bar) return;
  bar.innerHTML = "";
  const cur = page ? page.rel : null;
  const list = recent.length ? recent : page ? [{ rel: page.rel, title: page.title }] : [];
  list.forEach((p, i) => {
    if (i) {
      const sep = document.createElement("span");
      sep.className = "sep";
      sep.textContent = "·";
      bar.appendChild(sep);
    }
    const el = document.createElement("span");
    el.className = "place" + (p.rel === cur ? " current" : "");
    el.textContent = p.title;
    el.title = p.rel === JOURNALS ? "Every day, newest first" : p.rel;
    el.onclick = () => openRel(p.rel);
    bar.appendChild(el);
  });
}

// cycleRecent steps to the next older place, wrapping, like alt-tab: the order
// holds still while you keep stepping, or every step would put the page you
// landed on first and the next could only go back. One step from a settled
// list is therefore the place you were on before this one.
async function cycleRecent() {
  if (recent.length < 2) return say("No other recent page yet");
  if (!cycling) {
    cycling = true;
    cycleAt = Math.max(0, recent.findIndex((p) => page && p.rel === page.rel));
  }
  cycleAt = (cycleAt + 1) % recent.length;
  await openRel(recent[cycleAt].rel);
}

// settleRecent ends a run of cycling: wherever it stopped is now most recent.
function settleRecent() {
  if (!cycling) return;
  cycling = false;
  if (page) arrived(page.rel);
}

function renderTagged() {
  const sec = $("tagged");
  const list = $("taggedlist");
  list.innerHTML = "";
  const items = page.tagged || [];
  sec.hidden = items.length === 0;
  if (sec.hidden) return;
  $("taggedhead").textContent = `${items.length} pages tagged #${page.title}`;
  items.forEach((name) => {
    const li = document.createElement("li");
    li.textContent = name;
    li.onclick = () => open_(name);
    list.appendChild(li);
  });
}

function renderRefs() {
  const sec = $("refs");
  const list = $("reflist");
  list.innerHTML = "";
  const refs = page.refs || [];
  sec.hidden = refs.length === 0;
  if (sec.hidden) return;

  const heads = refs.filter((r) => r.head).length;
  $("refshead").textContent = `${heads} linked reference${heads === 1 ? "" : "s"}`;

  let group = null;
  let lastPage = null;
  refs.forEach((r) => {
    if (r.head && r.page !== lastPage) {
      lastPage = r.page;
      group = document.createElement("div");
      group.className = "refgroup";
      const from = document.createElement("div");
      from.className = "from";
      from.textContent = r.page;
      from.onclick = () => openRel(r.rel);
      group.appendChild(from);
      list.appendChild(group);
    }
    if (!group) return;
    const line = document.createElement("div");
    line.className = "refline" + (r.task === "done" ? " done" : "");
    line.style.marginLeft = r.depth * 16 + "px";
    // A mention that is a task shows as one. The marker is replaced by the box
    // rather than printed beside it, the same as in the outline.
    const box = r.task ? (r.task === "done" ? "☑ " : "☐ ") : "";
    line.innerHTML = box + (decorate(r.task ? r.body : r.text) || "&nbsp;");
    line.onclick = (e) => {
      if (e.target.dataset.page) return; // an inline link wins
      openRel(r.rel);
    };
    group.appendChild(line);
  });
}

// --- editing ----------------------------------------------------------------

// childrenOf is everything nested under a block. The block list is flat with a
// depth on each one, so a subtree is the run that follows it while the depth
// stays greater.
function childrenOf(pv, b) {
  const all = (pv && pv.blocks) || [];
  const i = all.findIndex((x) => x.offset === b.offset);
  if (i < 0) return [];
  const out = [];
  for (let j = i + 1; j < all.length && all[j].depth > b.depth; j++) out.push(all[j]);
  return out;
}

function raw(el) {
  return el.innerText.replace(/ /g, " ").replace(/\n$/, "");
}

// caretIndex is how far into the block's raw text the caret sits. It has to
// normalise exactly as raw does, or a split lands one character out wherever
// the browser has put a non-breaking space.
function caretIndex(el) {
  return textBeforeCaret(el).replace(/\u00a0/g, " ").length;
}

// placeCaretAt puts the caret n characters into a block, or at its end when n
// is null. Splitting needs the start of the new block, merging needs the seam
// between the two texts; everything else wants the end.
//
// Focusing first is deliberate: the focus handler swaps the rendered markup for
// the raw text, which would invalidate a range built before it.
function placeCaretAt(el, n) {
  el.focus();
  const range = document.createRange();
  range.selectNodeContents(el);
  if (n !== null && el.firstChild) {
    range.setStart(el.firstChild, Math.min(n, el.firstChild.textContent.length));
    range.collapse(true);
  } else {
    range.collapse(false);
  }
  const sel = window.getSelection();
  sel.removeAllRanges();
  sel.addRange(range);
}

function placeCaretAtEnd(el) {
  placeCaretAt(el, null);
}

// seamAbove is where the caret belongs after a merge: the end of the text the
// block above had before this one was appended to it.
function seamAbove(pv, b) {
  const i = pv.blocks.findIndex((x) => x.offset === b.offset);
  return i > 0 ? pv.blocks[i - 1].text.length : 0;
}

function caretAtStart(el) {
  const sel = window.getSelection();
  if (!sel.rangeCount) return false;
  const r = sel.getRangeAt(0).cloneRange();
  r.selectNodeContents(el);
  r.setEnd(sel.getRangeAt(0).endContainer, sel.getRangeAt(0).endOffset);
  return r.toString().length === 0;
}

// wireBlock makes a block editable. pv is the file it was drawn from; every
// call goes through that file as it now stands, which on the journals page is
// the block's own day. Once the file is no longer on screen — a blur after a
// click went elsewhere — the hash it was drawn with is the one sent, so a
// change made meanwhile is refused rather than overwritten.
function wireBlock(el, b, pv) {
  const rel = pv.rel;
  const cur = () => viewOf(rel) || pv;
  // While a block has focus it shows raw text: what you type is what is stored.
  el.addEventListener("focus", () => {
    el.textContent = el.dataset.raw;
  });

  el.addEventListener("blur", async () => {
    hideCompletion();
    const text = raw(el);
    if (text === el.dataset.raw) {
      el.innerHTML = decorate(text) || "<br>";
      return;
    }
    el.dataset.raw = text;
    applied(await call(() => api().SetText(rel, b.offset, cur().hash, text)));
  });

  el.addEventListener("input", () => updateCompletion(el));

  el.addEventListener("keydown", async (e) => {
    if (completion && handleCompletionKey(e, el)) return;

    // Enter ends this block at the caret and starts the next one with the
    // rest; alt or shift makes a newline inside this one instead.
    //
    // One call, not SetText followed by NewBlock: that was two writes, two
    // commits, and a window in which the file could move between them and
    // leave half the split behind.
    if (e.key === "Enter" && !e.altKey && !e.shiftKey && !e.metaKey) {
      e.preventDefault();
      const text = raw(el);
      const at = caretIndex(el);
      // The whole text, so that a stray blur on the way out is a no-op; the
      // redraw replaces this element anyway.
      el.dataset.raw = text;
      applied(
        await call(() =>
          api().SplitBlock(
            rel, b.offset, cur().hash,
            text.slice(0, at), text.slice(at),
            // Nothing is collapsed in the window, so a block with children has
            // visible ones and the new block belongs under it.
            b.hasChildren
          )
        ),
        0
      );
      return;
    }

    if (e.key === "Tab") {
      e.preventDefault();
      const text = raw(el);
      el.dataset.raw = text;
      const saved = await call(() => api().SetText(rel, b.offset, cur().hash, text));
      if (!saved) return;
      const fn = e.shiftKey ? api().Outdent : api().Indent;
      applied(await call(() => fn.call(api(), saved.page.rel, saved.offset, saved.page.hash)));
      return;
    }

    if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      const delta = e.key === "ArrowUp" ? -1 : 1;
      applied(await call(() => api().Move(rel, b.offset, cur().hash, delta)));
      return;
    }

    // Delete the block and everything under it. The window could only remove
    // an empty first block, so there was no way to throw away a paragraph
    // short of selecting the text and leaving an empty bullet behind.
    if (e.key === "Backspace" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      const kids = childrenOf(cur(), b).length;
      const gone = await call(() => api().DeleteBlock(rel, b.offset, cur().hash));
      if (gone) {
        say(kids
          ? `Block and ${kids === 1 ? "its child" : `its ${kids} children`} deleted — git has the previous version`
          : "Block deleted — git has the previous version");
      }
      applied(gone);
      return;
    }

    // Backspace at the start of a block joins it onto the one above, the way
    // an outliner behaves. On the very first block there is nothing above, so
    // an empty one is simply removed.
    if (e.key === "Backspace" && caretAtStart(el)) {
      const first = cur().blocks.length > 0 && cur().blocks[0].offset === b.offset;
      if (first) {
        if (raw(el) === "") {
          e.preventDefault();
          const gone = await call(() => api().DeleteBlock(rel, b.offset, cur().hash));
          if (gone) say("Block deleted — git has the previous version");
          applied(gone);
        }
        return;
      }
      e.preventDefault();
      const text = raw(el);
      el.dataset.raw = text;
      const seam = seamAbove(cur(), b);
      const saved = await call(() => api().SetText(rel, b.offset, cur().hash, text));
      if (!saved) return;
      applied(
        await call(() =>
          api().MergeIntoPrevious(saved.page.rel, saved.offset, saved.page.hash)
        ),
        seam
      );
      return;
    }

    // A new block above this one. Enter makes the one below; this is the other
    // direction, and the outliner has had it as O from the start.
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && e.shiftKey) {
      e.preventDefault();
      const text = raw(el);
      el.dataset.raw = text;
      const saved = await call(() => api().SetText(rel, b.offset, cur().hash, text));
      if (!saved) return;
      applied(
        await call(() => api().InsertBefore(saved.page.rel, saved.offset, saved.page.hash, "")),
        0
      );
      return;
    }

    if (e.key === "Escape") {
      hideCompletion();
      el.blur();
    }
  });
}

// --- completion -------------------------------------------------------------

function textBeforeCaret(el) {
  const sel = window.getSelection();
  if (!sel.rangeCount) return "";
  const r = sel.getRangeAt(0).cloneRange();
  r.selectNodeContents(el);
  r.setEnd(sel.getRangeAt(0).endContainer, sel.getRangeAt(0).endOffset);
  return r.toString();
}

// A slash starts a command when it begins a word, so a URL or a path never
// opens the menu. Everything after the first space is the command's argument.
function slashAt(before) {
  const i = before.lastIndexOf("/");
  if (i < 0) return null;
  if (i > 0 && before[i - 1] !== " " && before[i - 1] !== "\t") return null;
  const rest = before.slice(i + 1);
  if (/[\n/\\]/.test(rest)) return null;
  const sp = rest.indexOf(" ");
  return {
    start: i,
    name: sp < 0 ? rest : rest.slice(0, sp),
    arg: sp < 0 ? "" : rest.slice(sp + 1),
  };
}

// The trigger is an unclosed [[ to the left of the caret. A # after the page
// name switches to tags: [[Andreas #person]] says what Andreas is.
function linkPrefix(before) {
  const open = before.lastIndexOf("[[");
  if (open < 0) return null;
  const rest = before.slice(open + 2);
  if (rest.includes("]]") || rest.includes("\n")) return null;
  const sp = Math.max(rest.lastIndexOf(" "), rest.lastIndexOf("\t"));
  if (sp >= 0 && rest[sp + 1] === "#") {
    return { kind: "tag", prefix: rest.slice(sp + 2) };
  }
  return { kind: "page", prefix: rest };
}

// The trigger for a block reference is an unclosed (( to the left of the caret.
// (( is free in the dialect and is what Logseq uses, so the habit carries; a
// closed pair is ordinary prose, so f((x)) opens nothing.
function refPrefix(before) {
  const open = before.lastIndexOf("((");
  if (open < 0) return null;
  const rest = before.slice(open + 2);
  if (rest.includes("))") || rest.includes("\n")) return null;
  return { kind: "ref", prefix: rest };
}

async function updateCompletion(el) {
  const before = textBeforeCaret(el);

  const slash = slashAt(before);
  if (slash) {
    const cmds = await call(() => api().Commands(slash.name));
    if (cmds && cmds.length) {
      const sel = completion && completion.cmds ? Math.min(completion.sel, cmds.length - 1) : 0;
      // A file command shows the shelf itself: what you choose between is the
      // attachments, not a menu entry called "attachment".
      if (cmds.length === 1 && cmds[0].arg === "attachment") {
        const files = await call(() => api().Attachments(slash.arg));
        if (files && files.length) {
          const s = completion && completion.files ? Math.min(completion.sel, files.length - 1) : 0;
          completion = { kind: "cmd", cmds, files, sel: s, el, ...slash };
          return drawCompletion();
        }
      }

      let date = null;
      if (cmds[sel] && cmds[sel].arg === "date") {
        date = await call(() => api().DatePreview(slash.arg));
        if (date && !date.iso) date = null;
      }
      completion = { kind: "cmd", cmds, sel, el, ...slash, date };
      if (date) completion.cal = await call(() => api().Month(date.iso));
      return drawCompletion();
    }
    return hideCompletion(); // not a command: ordinary text with a slash in it
  }

  const ref = refPrefix(before);
  if (ref) {
    const refs = (await call(() => api().Blocks(ref.prefix))) || [];
    completion = {
      ...ref,
      refs,
      items: refs.map((r) => r.text),
      sel: 0,
      el,
    };
    return drawCompletion();
  }

  const trigger = linkPrefix(before);
  if (!trigger) return hideCompletion();

  const items =
    trigger.kind === "tag"
      ? await call(() => api().CompleteTags(trigger.prefix))
      : await call(() => api().CompletePages(trigger.prefix));
  if (items === null) return hideCompletion();

  // An empty list still opens the popup with a hint: on a fresh notes directory
  // silence is indistinguishable from the feature not existing.
  completion = { ...trigger, items: items || [], sel: 0, el };
  drawCompletion();
}

function drawCompletion() {
  const box = $("complete");
  box.innerHTML = "";

  if (completion.kind === "cmd" && completion.files) {
    const list = document.createElement("div");
    list.className = "cmds files";
    completion.files.forEach((f, i) => {
      const item = document.createElement("div");
      item.className = "item" + (i === completion.sel ? " sel" : "");
      item.innerHTML =
        `${escapeHTML(f.name)}<span class="hint"> ${escapeHTML(f.size)} · ${escapeHTML(f.when)}</span>`;
      item.onmousedown = (e) => {
        e.preventDefault();
        completion.sel = i;
        runCommand();
      };
      list.appendChild(item);
    });
    box.appendChild(list);
    placeCompletion(box);
    box.hidden = false;
    return;
  }

  if (completion.kind === "cmd") {
    const list = document.createElement("div");
    list.className = "cmds";
    completion.cmds.forEach((c, i) => {
      const item = document.createElement("div");
      item.className = "item" + (i === completion.sel ? " sel" : "");
      item.innerHTML =
        `<b>/${escapeHTML(c.name)}</b> <span class="hint">${escapeHTML(c.hint)}</span>`;
      item.onmousedown = (e) => {
        e.preventDefault();
        completion.sel = i;
        runCommand();
      };
      list.appendChild(item);
    });
    box.appendChild(list);

    if (completion.date) {
      const d = document.createElement("div");
      d.className = "datepick";
      d.innerHTML =
        `<div class="resolved t-${completion.date.state}">${escapeHTML(completion.date.short)}` +
        `<span class="hint"> ${escapeHTML(completion.date.label)}</span></div>` +
        calendarHTML(completion.cal);
      box.appendChild(d);
    }
    placeCompletion(box);
    box.hidden = false;
    return;
  }
  if (completion.items.length === 0) {
    const hint = document.createElement("div");
    hint.className = "hint";
    hint.textContent =
      completion.kind === "tag"
        ? "no tags yet — type one to create it"
        : completion.kind === "ref"
          ? "no block says that — type a word from the one you mean"
          : "no page by that name yet — it is created when you go there";
    box.appendChild(hint);
  } else {
    completion.items.forEach((name, i) => {
      const item = document.createElement("div");
      item.className = "item" + (i === completion.sel ? " sel" : "");
      if (completion.kind === "ref") {
        // Which page a block is on is most of what identifies it.
        item.innerHTML =
          `${escapeHTML(name.replace(/\n/g, " ").slice(0, 60))}` +
          `<span class="hint"> ${escapeHTML(completion.refs[i].page)}</span>`;
      } else {
        item.textContent = name;
      }
      item.onmousedown = (e) => {
        e.preventDefault();
        completion.sel = i;
        acceptCompletion();
      };
      box.appendChild(item);
    });
  }

  placeCompletion(box);
  box.hidden = false;
}

function placeCompletion(box) {
  const sel = window.getSelection();
  if (!sel.rangeCount) return;
  const r = sel.getRangeAt(0).getBoundingClientRect();
  const anchor = r.width || r.height ? r : completion.el.getBoundingClientRect();
  box.style.left = Math.round(Math.min(anchor.left, window.innerWidth - 340)) + "px";
  box.style.top = Math.round(anchor.bottom + 4) + "px";
}

// The calendar is confirmation, not input: it shows what the shorthand meant so
// a mistyped date is caught before it is written.
function calendarHTML(cal) {
  if (!cal) return "";
  const head = ["Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"]
    .map((d) => `<th>${d}</th>`)
    .join("");
  const cells = cal.days
    .map((d) => {
      if (!d.day) return "<td></td>";
      const cls = [d.today ? "today" : "", d.sel ? "sel" : ""].filter(Boolean).join(" ");
      return `<td class="${cls}">${d.day}</td>`;
    })
    .join("");
  const rows = [];
  const all = cells.match(/<td[^>]*>.*?<\/td>/g) || [];
  for (let i = 0; i < all.length; i += 7) rows.push(`<tr>${all.slice(i, i + 7).join("")}</tr>`);
  return (
    `<div class="calhead">${escapeHTML(cal.title)}</div>` +
    `<table class="cal"><thead><tr>${head}</tr></thead><tbody>${rows.join("")}</tbody></table>`
  );
}

function hideCompletion() {
  completion = null;
  $("complete").hidden = true;
}

function handleCompletionKey(e, el) {
  // ctrl+n and ctrl+p move through an open list, as they do in the outliner;
  // with no list open, ctrl+n steps through the recent pages instead.
  const emacs = e.ctrlKey && !e.metaKey && !e.altKey;
  const key = emacs && e.key === "n" ? "ArrowDown" : emacs && e.key === "p" ? "ArrowUp" : e.key;
  if (key === "Escape") {
    e.preventDefault();
    hideCompletion();
    return true;
  }
  if (completion.kind === "cmd") {
    if (key === "ArrowDown" || key === "ArrowUp") {
      e.preventDefault();
      const n = (completion.files || completion.cmds).length;
      const d = key === "ArrowDown" ? 1 : -1;
      completion.sel = Math.max(0, Math.min(completion.sel + d, n - 1));
      if (completion.files) drawCompletion();
      else updateCompletion(el);
      return true;
    }
    if (key === "Enter" || key === "Tab") {
      e.preventDefault();
      runCommand();
      return true;
    }
    return false;
  }
  if (completion.items.length === 0) {
    // A hint claims nothing, except the keys that would otherwise leave the page.
    if (key !== e.key) e.preventDefault();
    return key !== e.key;
  }
  if (key === "ArrowDown") {
    e.preventDefault();
    completion.sel = Math.min(completion.sel + 1, completion.items.length - 1);
    drawCompletion();
    return true;
  }
  if (key === "ArrowUp") {
    e.preventDefault();
    completion.sel = Math.max(completion.sel - 1, 0);
    drawCompletion();
    return true;
  }
  if (key === "Enter" || key === "Tab") {
    e.preventDefault();
    acceptCompletion();
    return true;
  }
  return false;
}

// replaceBefore swaps the n characters before the caret for text, as one edit
// the browser can undo. Completions that append can just insert; a block
// reference replaces what was typed, because (( is not part of what is stored.
function replaceBefore(n, text) {
  const sel = window.getSelection();
  if (!sel.rangeCount) return;
  for (let i = 0; i < n; i++) sel.modify("extend", "backward", "character");
  document.execCommand("insertText", false, text);
}

// A page name closes the link; a tag does not, because you may want another.
// A block reference is different in kind: accepting one gives that block a
// durable name, which is a write to another file, and only then is there
// anything to type here.
async function acceptCompletion() {
  if (completion.kind === "ref") {
    const r = completion.refs[completion.sel];
    const n = completion.prefix.length + 2;
    hideCompletion();
    if (!r) return;
    const link = await call(() => api().RefTo(r.rel, r.offset, r.hash));
    if (link) replaceBefore(n, link);
    return;
  }
  const name = completion.items[completion.sel];
  if (!name) return hideCompletion();
  const closing = completion.kind === "tag" ? "" : "]]";
  document.execCommand("insertText", false, name.slice(completion.prefix.length) + closing);
  hideCompletion();
}

// runCommand applies the selected command. The core cuts the "/name arg" out of
// the text, so both adapters cut identically.
async function runCommand() {
  const c = completion;
  if (!c || !c.cmds || !c.cmds.length) return hideCompletion();
  // When the menu was the shelf, the selection names the file.
  const cmd = c.files ? c.cmds[0] : c.cmds[c.sel];
  const arg = c.files ? (c.files[c.sel] || {}).name || c.arg : c.arg;
  const el = c.el;
  const text = raw(el);
  const from = c.start;
  const to = textBeforeCaret(el).length;
  const offset = Number(el.dataset.offset);
  const rel = el.dataset.rel;
  const pv = viewOf(rel);
  hideCompletion();
  if (!pv) return;

  const edit = await call(() =>
    api().RunCommand(rel, offset, pv.hash, cmd.name, arg, text, from, to)
  );
  if (!edit) return;
  applied(edit);
}

// --- navigation -------------------------------------------------------------

// Where you have been, so that following a link is not a one-way door. Only
// the path is kept: the page itself may have changed by the time you go back,
// and showing the version you remember rather than the one on disk is how an
// editor starts lying to you.
const history = [];
let forward = [];

// remember records where we are before going somewhere else. Going back and
// arriving at the same page again are different things, so the caller says
// which this is rather than the function guessing from the path.
function remember() {
  if (page && history[history.length - 1] !== page.rel) history.push(page.rel);
}

// fetchPlace reads a place by its path, the journals page included.
async function fetchPlace(rel) {
  if (rel === JOURNALS) {
    const f = await loadFeed(0);
    return f ? feedPage(f) : null;
  }
  return call(() => api().OpenRel(rel));
}

async function openRel(rel) {
  if (page && page.rel === rel) return; // already here
  hideCompletion();
  remember();
  forward = [];
  show(await fetchPlace(rel));
}

function openJournals() {
  return openRel(JOURNALS);
}

async function open_(name) {
  hideCompletion();
  remember();
  forward = [];
  show(await call(() => api().OpenPage(name)));
}

async function openToday() {
  hideCompletion();
  remember();
  forward = [];
  show(await call(() => api().TodayJournal()));
}

// openStartup is where the window opens: today's journal, or whatever was
// written last, as the startup setting says. Both methods have always been
// bound and the window called the wrong one, so `startup = last` did nothing
// here while the outliner honoured it.
async function openStartup() {
  show(await call(() => api().Today()));
}

// goBack returns to the last page, keeping where you were so that going back
// and forward again lands where it started.
async function goBack() {
  if (!history.length) return;
  const rel = history.pop();
  if (page) forward.push(page.rel);
  hideCompletion();
  show(await fetchPlace(rel));
  updateNav();
}

async function goForward() {
  if (!forward.length) return;
  const rel = forward.pop();
  if (page) history.push(page.rel);
  hideCompletion();
  show(await fetchPlace(rel));
  updateNav();
}

// updateNav dims the arrows when there is nowhere to go, so the window says
// whether back is available rather than leaving it to be discovered.
function updateNav() {
  $("back").disabled = history.length === 0;
  $("fwd").disabled = forward.length === 0;
}

async function shiftDay(delta) {
  if (!page) return;
  if (feed) return jumpDay(delta); // on the journals page, scroll to the day
  remember();
  forward = [];
  show(await call(() => api().Journal(page.rel, delta)));
}

// markActive is called from refreshIndex, which runs before the first page is
// open — so there may not be a page yet. Reading page.rel here threw on every
// cold start with anything in the sidebar, which aborted start() before it
// could open today's journal: the window came up with an empty pane, and
// clicking any page in the sidebar was what appeared to fix it.
function markActive() {
  const rel = page ? page.rel : null;
  document.querySelectorAll("#nav li").forEach((li) => {
    li.classList.toggle("active", rel !== null && li.dataset.rel === rel);
  });
}

async function refreshIndex() {
  const idx = await call(() => api().Index());
  if (!idx) return;

  // Journals and pages are opened by their path, which the index already knows;
  // resolving them by name again would be a second chance to get it wrong.
  const fill = (el, names, relOf) => {
    el.innerHTML = "";
    (names || []).forEach((name) => {
      const li = document.createElement("li");
      li.textContent = name;
      const rel = relOf(name);
      li.dataset.rel = rel;
      li.onclick = () => (rel ? openRel(rel) : open_(name));
      el.appendChild(li);
    });
  };

  fill($("journals"), idx.journals, (n) => `journals/${n}.md`);
  fill($("pages"), idx.pages, (n) => `pages/${n}.md`);
  fill($("tags"), idx.tags, () => "");
  markActive();
}

// --- settings ---------------------------------------------------------------

// The same list the outliner edits and `tlog config` prints, so the three
// cannot drift apart or disagree about what a value means.
async function openSettings() {
  const items = await call(() => api().Settings());
  if (!items) return;
  const path = await call(() => api().ConfigPath());
  $("settingspath").textContent = path || "";
  drawSettings(items);
  $("settings").hidden = false;
}

function drawSettings(items, note) {
  const list = $("settingslist");
  list.innerHTML = "";
  items.forEach((s) => {
    const row = document.createElement("div");
    row.className = "setting";

    const label = document.createElement("div");
    label.className = "label";
    label.innerHTML =
      `<div class="key">${escapeHTML(s.key)}</div>` +
      `<div class="hint">${escapeHTML(s.hint)}</div>` +
      (s.note ? `<div class="note">${escapeHTML(s.note)}</div>` : "");
    row.appendChild(label);

    const field = document.createElement("div");
    field.className = "field";
    if (s.kind === "bool") {
      const box = document.createElement("input");
      box.type = "checkbox";
      box.checked = s.value === "true";
      box.onchange = () => saveSetting(s.key, String(box.checked));
      field.appendChild(box);
    } else {
      const input = document.createElement("input");
      input.type = "text";
      input.value = s.value;
      input.spellcheck = false;
      input.onchange = () => saveSetting(s.key, input.value);
      input.onkeydown = (e) => {
        if (e.key === "Enter") input.blur();
      };
      field.appendChild(input);
    }
    row.appendChild(field);
    list.appendChild(row);
  });

  const msg = document.createElement("div");
  msg.className = "savednote";
  msg.textContent = note || "";
  list.appendChild(msg);
}

async function saveSetting(key, value) {
  const res = await call(() => api().SetSetting(key, value));
  if (!res) return openSettings(); // a refused value: show what it really is
  drawSettings(res.settings, res.note ? "Gespeichert — " + res.note : "Gespeichert");
}

$("gear").onclick = openSettings;
$("closesettings").onclick = () => ($("settings").hidden = true);
$("settings").onclick = (e) => {
  if (e.target.id === "settings") $("settings").hidden = true;
};

// --- search -----------------------------------------------------------------

let searchTimer = null;

$("search").addEventListener("input", (e) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => runSearch(e.target.value), 120);
});

async function runSearch(query) {
  const box = $("results");
  if (!query.trim()) {
    box.hidden = true;
    $("nav").hidden = false;
    return;
  }
  const hits = await call(() => api().Search(query));
  box.innerHTML = "";
  (hits || []).slice(0, 60).forEach((h) => {
    const el = document.createElement("div");
    el.className = "hit";
    el.innerHTML = `<div class="what">${escapeHTML(h.text || "(empty block)")}</div><div class="where">${escapeHTML(h.page)}</div>`;
    el.onclick = () => openRel(h.rel);
    box.appendChild(el);
  });
  if (!hits || hits.length === 0) {
    box.innerHTML = `<div class="hit"><div class="where">no matches</div></div>`;
  }
  box.hidden = false;
  $("nav").hidden = true;
}

// --- wiring -----------------------------------------------------------------

$("today").onclick = openToday;
$("prev").onclick = () => shiftDay(-1);
$("next").onclick = () => shiftDay(1);
$("back").onclick = () => goBack();
$("fwd").onclick = () => goForward();
$("closehelp").onclick = () => ($("help").hidden = true);
$("closelog").onclick = () => ($("log").hidden = true);
$("palettequery").addEventListener("input", refreshPalette);
$("palettequery").addEventListener("keydown", (e) => {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    placeSel = Math.min(placeSel + 1, places.length - 1);
    drawPalette();
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    placeSel = Math.max(placeSel - 1, 0);
    drawPalette();
  } else if (e.key === "Enter") {
    e.preventDefault();
    choosePlace();
  }
});
// Dismissing a message and losing it are different things.
$("error").onclick = () => {
  $("error").hidden = true;
  renderLog();
  $("log").hidden = false;
};

// Navigation happens on mousedown, not click: by the time a click arrives the
// block has taken focus, swapped itself to raw text and destroyed the very
// span that was pressed. preventDefault stops that focus from happening.
// openBlock goes to a block on another page and puts the caret on it, which is
// what following a reference means.
async function openBlock(e) {
  if (!e || !e.rel) return;
  const p = await call(() => api().OpenRel(e.rel));
  if (!p) return;
  focusOffset = e.addr ? Number(String(e.addr).split(":").pop().split("@")[0]) : 0;
  show(p);
}

document.addEventListener("mousedown", (e) => {
  settleRecent();
  const d = e.target.dataset;
  if (!d) return;
  if (d.page) {
    e.preventDefault();
    return open_(d.page);
  }
  if (d.tag) {
    e.preventDefault();
    return open_(d.tag);
  }
  if (d.url) {
    e.preventDefault();
    return call(() => api().OpenURL(d.url));
  }
});

// The keys, in one table. The other adapter is a vim-style outliner with
// fifty-odd bindings; this had three, and reaching for the mouse to change
// page in a tool whose other face never needs one is a jarring difference.
//
// Every binding takes a modifier, so nothing here can swallow a keystroke
// meant for the block being edited.
const KEYS = [
  { key: "k", mod: true, label: "Search", run: focusSearch },
  { key: "f", mod: true, label: "Search", run: focusSearch },
  { key: "t", mod: true, label: "Today", run: () => openToday() },
  { key: "j", mod: true, label: "Journals", run: () => openJournals() },
  { key: "n", mod: true, label: "Next recent page", run: () => cycleRecent() },
  { key: "a", mod: true, shift: true, label: "Agenda", run: () => openAgenda() },
  { key: "[", mod: true, label: "Back", run: () => goBack() },
  { key: "]", mod: true, label: "Forward", run: () => goForward() },
  { key: "ArrowLeft", mod: true, alt: true, label: "Previous day", run: () => shiftDay(-1) },
  { key: "ArrowRight", mod: true, alt: true, label: "Next day", run: () => shiftDay(1) },
  { key: ",", mod: true, label: "Settings", run: openSettings },
  { key: "/", mod: true, label: "Keyboard shortcuts", run: toggleHelp },
  { key: "m", mod: true, shift: true, label: "Messages", run: openLog },
  { key: "p", mod: true, label: "Go to a page", run: () => openPalette() },
];

function focusSearch() {
  $("search").focus();
  $("search").select();
}

// isCycleKey is the chord that steps through the recent pages. Anything else
// pressed — other than a modifier on its own, on the way to the next step —
// settles the list where the stepping stopped.
function isCycleKey(e) {
  return (e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === "n";
}
const MODIFIERS = new Set(["Meta", "Control", "Shift", "Alt"]);

document.addEventListener("keydown", (e) => {
  if (!isCycleKey(e) && !MODIFIERS.has(e.key)) settleRecent();
  // A block that took the key — a completion list moving with ctrl+n — has it.
  if (e.defaultPrevented) return;
  // Escape backs out of whatever is covering the page, innermost first.
  if (e.key === "Escape") {
    if (!$("palette").hidden) return void ($("palette").hidden = true);
    if (!$("log").hidden) return void ($("log").hidden = true);
    if (!$("help").hidden) return void ($("help").hidden = true);
    if (!$("settings").hidden) return void ($("settings").hidden = true);
    if (!$("agenda").hidden && page) return void show(page);
    return;
  }
  const mod = e.metaKey || e.ctrlKey;
  for (const k of KEYS) {
    if (e.key.toLowerCase() !== k.key.toLowerCase() && e.key !== k.key) continue;
    if (!!k.mod !== mod) continue;
    if (!!k.shift !== e.shiftKey) continue;
    if (!!k.alt !== e.altKey) continue;
    e.preventDefault();
    k.run();
    return;
  }
});

// --- going somewhere -------------------------------------------------------
//
// Search finds words inside blocks, which is the wrong tool for "take me to
// the page about Ada": a page whose name you remember but whose contents you
// do not never came up, and the sidebar is an unfiltered list. The outliner
// has had ctrl+p from the start.

let places = [];
let placeSel = 0;

async function openPalette() {
  $("palettequery").value = "";
  await refreshPalette();
  $("palette").hidden = false;
  $("palettequery").focus();
}

// The journals page is not a file, so the core does not list it; it is offered
// first whenever what is typed could be its name, as the outliner does.
async function refreshPalette() {
  const q = $("palettequery").value;
  places = (await call(() => api().Places(q))) || [];
  if ("journals".includes(q.trim().toLowerCase())) {
    places.unshift({ kind: "journals", name: "Journals", rel: JOURNALS });
  }
  placeSel = 0;
  drawPalette();
}

function drawPalette() {
  const list = $("palettelist");
  list.innerHTML = "";
  if (places.length === 0) {
    const li = document.createElement("li");
    li.className = "hint";
    li.textContent = "Nothing by that name";
    list.appendChild(li);
    return;
  }
  places.forEach((p, i) => {
    const li = document.createElement("li");
    li.className = "place" + (i === placeSel ? " sel" : "");
    li.innerHTML =
      `<span class="kind">${escapeHTML(p.kind)}</span>` +
      `<span>${escapeHTML(p.kind === "tag" ? "#" + p.name : p.name)}</span>`;
    li.onmousedown = (e) => {
      e.preventDefault();
      placeSel = i;
      choosePlace();
    };
    list.appendChild(li);
  });
}

// choosePlace opens what is selected. A tag may have no file of its own, so it
// is opened by name — which is what creates it, and only then.
async function choosePlace() {
  const p = places[placeSel];
  $("palette").hidden = true;
  if (!p) return;
  if (p.kind === "tag" || !p.rel) return open_(p.name);
  openRel(p.rel);
}

function openLog() {
  renderLog();
  $("log").hidden = false;
}

// toggleHelp shows the table above rather than a list written out by hand,
// which would be a second place to keep in step and the one that goes stale.
function toggleHelp() {
  const box = $("help");
  if (!box.hidden) {
    box.hidden = true;
    return;
  }
  const mac = navigator.platform.toLowerCase().includes("mac");
  const rows = KEYS.map((k) => {
    const parts = [];
    if (k.mod) parts.push(mac ? "⌘" : "Ctrl");
    if (k.alt) parts.push(mac ? "⌥" : "Alt");
    if (k.shift) parts.push("⇧");
    parts.push({ ArrowLeft: "←", ArrowRight: "→" }[k.key] || k.key.toUpperCase());
    return `<tr><td class="chord">${escapeHTML(parts.join(mac ? "" : "+"))}</td>` +
           `<td>${escapeHTML(k.label)}</td></tr>`;
  });
  $("helplist").innerHTML =
    "<table>" + rows.join("") +
    `<tr><td class="chord">Esc</td><td>Close what is open</td></tr>` +
    `<tr><td class="chord">Tab</td><td>Indent the block being edited</td></tr>` +
    `<tr><td class="chord">${mac ? "⌥" : "Alt"}↑↓</td><td>Move the block</td></tr>` +
    `<tr><td class="chord">${mac ? "⌘⇧⏎" : "Ctrl+Shift+Enter"}</td><td>New block above</td></tr>` +
    `<tr><td class="chord">${mac ? "⌘⌫" : "Ctrl+Backspace"}</td><td>Delete the block and what is under it</td></tr>` +
    "</table>";
  box.hidden = false;
}

// Wails reports a file drop here. The files go on the shared shelf and their
// links into the page currently open — on the journals page, into the day of
// the block being edited, or today.
window.runtime && window.runtime.OnFileDrop(async (x, y, paths) => {
  if (!paths || !paths.length || !page) return;
  document.body.classList.remove("dropping");
  const rel = feed ? editingRel() || (feed.days[0] && feed.days[0].page.rel) : page.rel;
  if (!rel) return;
  const edit = await call(() => api().AttachFiles(rel, paths));
  // Nothing was being typed at, so the caret is not put anywhere.
  if (edit) applied({ ...edit, offset: null });
}, true);

// editingRel is the file of the block that has the caret, if one does.
function editingRel() {
  const a = document.activeElement;
  return a && a.classList && a.classList.contains("text") ? a.dataset.rel || null : null;
}

if (window.runtime && window.runtime.OnFileDropOff) {
  window.addEventListener("dragover", () => document.body.classList.add("dropping"));
  window.addEventListener("dragleave", () => document.body.classList.remove("dropping"));
}

// --- the agenda -------------------------------------------------------------
//
// Everything with a deadline, soonest first — the same list `tlog due` prints,
// from the same core call. It is a view rather than a page: nothing is written
// by looking at it, and clicking a line goes to the block itself.

async function openAgenda() {
  const items = await call(() => api().Due(false));
  if (items === null) return;

  const list = $("agendalist");
  list.innerHTML = "";
  if (items.length === 0) {
    const li = document.createElement("li");
    li.className = "hint";
    li.textContent = "nothing is due — /deadline on a block puts it here";
    list.appendChild(li);
  }
  items.forEach((it) => {
    const li = document.createElement("li");
    li.className = "agendaitem t-" + (it.state || "done");
    li.innerHTML =
      `<span class="due">${escapeHTML(it.due)}</span>` +
      `<span class="when">${escapeHTML(it.label || "")}</span>` +
      `<span class="what">${decorate(it.text)}</span>` +
      `<span class="where">${escapeHTML(it.page)}</span>`;
    li.onclick = async () => {
      const p = await call(() => api().OpenRel(it.rel));
      if (!p) return;
      focusOffset = it.offset;
      show(p); // which puts the outline back
    };
    list.appendChild(li);
  });

  $("outline").hidden = true;
  $("tagged").hidden = true;
  $("refs").hidden = true;
  $("pagehead").hidden = true;
  $("agenda").hidden = false;
}

// showOutline puts the page back. render calls it, so every way of navigating
// away from the agenda closes it without having to remember.
function showOutline() {
  $("agenda").hidden = true;
  $("pagehead").hidden = false;
  $("outline").hidden = false;
}

// --- noticing an edit made somewhere else -----------------------------------

// shouldReload decides what a change on disk means for what is on screen. It is
// a plain function taking the three things that matter, so it can be tested
// without a window, a watcher or a file.
//
//   - a file we are not looking at: the sidebar and the backlinks may have
//     moved, so refresh the index but leave the page alone
//   - the page we are looking at, unchanged bytes: our own write, coming back
//   - the page we are looking at, while a block has focus: never. What is being
//     typed has not reached the file, and a redraw would take it away
//   - otherwise: reload
function shouldReload(ev, page, editing) {
  if (!ev || !page) return "none";
  if (ev.rel !== page.rel) return "index";
  if (ev.hash === page.hash) return "none";
  if (editing) return "stale";
  return "page";
}

// feedReload is shouldReload for the journals page, where every day is its own
// file. editingRel is the day of the block that has the caret, or null.
//
//   - a day on screen, unchanged bytes: our own write, coming back
//   - a day on screen, being typed in: never reload it — warn
//   - a day on screen otherwise: reload that day and no other
//   - a journal day not listed yet: written elsewhere, so the list of days
//     is read again — once nothing is being typed
//   - anything else: the index, and how references into the days resolve
function feedReload(ev, feed, editingRel, journals) {
  if (!ev || !feed) return "none";
  const day = feed.days.find((d) => d.page.rel === ev.rel);
  if (day) {
    if (ev.hash === day.page.hash) return "none";
    if (editingRel === ev.rel) return "stale";
    return "day";
  }
  const isDay = ev.rel.startsWith(journals + "/");
  if (isDay && !feed.rels.includes(ev.rel)) return editingRel ? "index" : "list";
  return "index";
}

// reloadDay re-reads one day of the journals page after it changed on disk.
async function reloadDay(rel) {
  const days = await call(() => api().OpenDays([rel]));
  if (!days || !days.length || !feed) return;
  const i = feed.days.findIndex((d) => d.page.rel === rel);
  if (i < 0) return;
  feed.days[i] = days[0];
  renderDay(rel);
}

async function changedInFeed(ev) {
  const editing = editingRel();
  switch (feedReload(ev, feed, editing, JOURNALS)) {
    case "day":
      await reloadDay(ev.rel);
      await refreshIndex();
      break;
    case "list":
      await refreshIndex();
      await refreshFeed();
      break;
    case "index":
      await refreshIndex();
      if (!editing) await refreshFeed();
      break;
    case "stale":
      fail("This day changed on disk. Click away to save what you typed.");
      break;
  }
}

if (window.runtime && window.runtime.EventsOn) {
  window.runtime.EventsOn("notes:changed", async (ev) => {
    if (feed) return changedInFeed(ev);
    const editing =
      document.activeElement && document.activeElement.classList.contains("text");
    switch (shouldReload(ev, page, editing)) {
      case "index":
        await refreshIndex();
        // The page's own backlinks come from the same graph, so they are stale
        // too — but redrawing is only safe when nothing has focus.
        if (!editing) show(await call(() => api().OpenRel(page.rel)));
        break;
      case "page":
        show(await call(() => api().OpenRel(page.rel)));
        await refreshIndex();
        break;
      case "stale":
        fail("This page changed on disk. Click away to save what you typed.");
        break;
    }
  });
}

$("agendabtn").onclick = () => openAgenda();
$("journalsbtn").onclick = () => openJournals();

(async function start() {
  const root = await call(() => api().Root());
  if (root) $("rootpath").textContent = root;
  const journals = await call(() => api().JournalsRel());
  if (journals) JOURNALS = journals;
  const r = await call(() => api().Recent());
  if (r) recent = r.places || [];
  await refreshIndex();
  await openStartup();
})();
