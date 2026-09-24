// The desktop outliner. Every operation is a call into the Go core; this file
// draws the result and translates keystrokes, and decides nothing on its own.
//
// Wails exposes the bound methods as window.go.main.API, so no generated
// bindings and no build step are needed.

const api = () => window.go.main.API;

const $ = (id) => document.getElementById(id);

let page = null; // the current PageView
let focusOffset = null; // where to put the caret after a redraw
let focusCaret = null; // how far into that block, null meaning at its end
let completion = null;

// --- plumbing ---------------------------------------------------------------

function fail(err) {
  const box = $("error");
  box.textContent = String(err && err.message ? err.message : err);
  box.hidden = false;
  clearTimeout(fail.timer);
  fail.timer = setTimeout(() => (box.hidden = true), 6000);
}

async function call(fn) {
  try {
    return await fn();
  } catch (err) {
    fail(err);
    return null;
  }
}

function show(p) {
  if (!p) return;
  page = p;
  render();
}

// An edit returns the page plus where the block ended up, because rewriting the
// file moves every offset after it.
function applied(edit, caret = null) {
  if (!edit) return;
  focusOffset = edit.offset;
  focusCaret = caret;
  show(edit.page);
  refreshIndex();
  checkSync();
}

// A push happens in the background after a commit; if it failed, say so.
async function checkSync() {
  const st = await call(() => api().Sync());
  if (st && st.lastError) fail("not pushed: " + st.lastError);
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

function decorate(text, fences) {
  const out = [];
  const lines = text.split("\n");
  let i = 0;
  let fence = 0;
  let plain = [];

  const flush = () => {
    if (!plain.length) return;
    out.push(`<span class="ln">${plain.map(decorateInline).join("\n")}</span>`);
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
function decorateInline(text) {
  const held = [];
  const hold = (html) => {
    held.push(html);
    return "\u0000" + (held.length - 1) + "\u0000";
  };

  let out = escapeHTML(text);

  // [[Page]], [[Page#^anchor]], [[Page #tag]] — show the name and what it is.
  out = out.replace(/\[\[([^\[\]]+)\]\]/g, (_, inner) => {
    const tags = [];
    let name = inner.replace(/\s+#([A-Za-z][\w/-]*)/g, (m, t) => {
      tags.push(t);
      return "";
    });
    name = name.split("#^")[0].trim();
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
  $("daynav").style.visibility = page.isJournal ? "visible" : "hidden";

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

  if (focusOffset !== null) {
    const el = document.querySelector(`.text[data-offset="${focusOffset}"]`);
    if (el) placeCaretAt(el, focusCaret);
    focusOffset = null;
    focusCaret = null;
  }
}

function renderOutline() {
  const out = $("outline");
  out.innerHTML = "";

  if (!page.blocks || page.blocks.length === 0) {
    const hint = document.createElement("div");
    hint.className = "refline";
    hint.textContent = "Empty — click here to start writing";
    hint.onclick = async () =>
      applied(await call(() => api().AppendBlock(page.rel, page.hash, "")));
    out.appendChild(hint);
    return;
  }

  page.blocks.forEach((b) => {
    const row = document.createElement("div");
    row.className = "block" + (b.task === "done" ? " done" : "");
    row.style.marginLeft = b.depth * 22 + "px";

    if (b.task) {
      const check = document.createElement("span");
      check.className = "check";
      check.textContent = b.task === "done" ? "☑" : "☐";
      check.onclick = async () =>
        applied(await call(() => api().ToggleTask(page.rel, b.offset, page.hash)));
      row.appendChild(check);
    } else {
      const bullet = document.createElement("span");
      bullet.className = "bullet" + (b.hasChildren ? " haskids" : "");
      bullet.textContent = "●";
      bullet.title = "Make this a task";
      bullet.onclick = async () =>
        applied(await call(() => api().ToggleTask(page.rel, b.offset, page.hash)));
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
    text.dataset.raw = b.text;
    text.innerHTML = decorate(b.text, b.fences) || "<br>";
    wireBlock(text, b);
    row.appendChild(text);

    out.appendChild(row);

    (b.props || []).filter((p) => p.key.toLowerCase() !== "deadline").forEach((p) => {
      const pr = document.createElement("div");
      pr.className = "props";
      pr.style.marginLeft = b.depth * 22 + 20 + "px";
      pr.textContent = `${p.key}:: ${p.value}`;
      out.appendChild(pr);
    });
  });
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
    line.className = "refline";
    line.style.marginLeft = r.depth * 16 + "px";
    line.innerHTML = decorate(r.text) || "&nbsp;";
    line.onclick = (e) => {
      if (e.target.dataset.page) return; // an inline link wins
      openRel(r.rel);
    };
    group.appendChild(line);
  });
}

// --- editing ----------------------------------------------------------------

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
function seamAbove(b) {
  const i = page.blocks.findIndex((x) => x.offset === b.offset);
  return i > 0 ? page.blocks[i - 1].text.length : 0;
}

function caretAtStart(el) {
  const sel = window.getSelection();
  if (!sel.rangeCount) return false;
  const r = sel.getRangeAt(0).cloneRange();
  r.selectNodeContents(el);
  r.setEnd(sel.getRangeAt(0).endContainer, sel.getRangeAt(0).endOffset);
  return r.toString().length === 0;
}

function wireBlock(el, b) {
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
    applied(await call(() => api().SetText(page.rel, b.offset, page.hash, text)));
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
            page.rel, b.offset, page.hash,
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
      const saved = await call(() => api().SetText(page.rel, b.offset, page.hash, text));
      if (!saved) return;
      const fn = e.shiftKey ? api().Outdent : api().Indent;
      applied(await call(() => fn.call(api(), saved.page.rel, saved.offset, saved.page.hash)));
      return;
    }

    if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      const delta = e.key === "ArrowUp" ? -1 : 1;
      applied(await call(() => api().Move(page.rel, b.offset, page.hash, delta)));
      return;
    }

    // Backspace at the start of a block joins it onto the one above, the way
    // an outliner behaves. On the very first block there is nothing above, so
    // an empty one is simply removed.
    if (e.key === "Backspace" && caretAtStart(el)) {
      const first = page.blocks.length > 0 && page.blocks[0].offset === b.offset;
      if (first) {
        if (raw(el) === "") {
          e.preventDefault();
          applied(await call(() => api().DeleteBlock(page.rel, b.offset, page.hash)));
        }
        return;
      }
      e.preventDefault();
      const text = raw(el);
      el.dataset.raw = text;
      const seam = seamAbove(b);
      const saved = await call(() => api().SetText(page.rel, b.offset, page.hash, text));
      if (!saved) return;
      applied(
        await call(() =>
          api().MergeIntoPrevious(saved.page.rel, saved.offset, saved.page.hash)
        ),
        seam
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
  const head = ["Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"]
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
  if (e.key === "Escape") {
    e.preventDefault();
    hideCompletion();
    return true;
  }
  if (completion.kind === "cmd") {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const n = (completion.files || completion.cmds).length;
      const d = e.key === "ArrowDown" ? 1 : -1;
      completion.sel = Math.max(0, Math.min(completion.sel + d, n - 1));
      if (completion.files) drawCompletion();
      else updateCompletion(el);
      return true;
    }
    if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      runCommand();
      return true;
    }
    return false;
  }
  if (completion.items.length === 0) return false; // a hint claims nothing
  if (e.key === "ArrowDown") {
    e.preventDefault();
    completion.sel = Math.min(completion.sel + 1, completion.items.length - 1);
    drawCompletion();
    return true;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    completion.sel = Math.max(completion.sel - 1, 0);
    drawCompletion();
    return true;
  }
  if (e.key === "Enter" || e.key === "Tab") {
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
  hideCompletion();

  const edit = await call(() =>
    api().RunCommand(page.rel, offset, page.hash, cmd.name, arg, text, from, to)
  );
  if (!edit) return;
  applied(edit);
}

// --- navigation -------------------------------------------------------------

async function openRel(rel) {
  hideCompletion();
  show(await call(() => api().OpenRel(rel)));
}

async function open_(name) {
  hideCompletion();
  show(await call(() => api().OpenPage(name)));
}

async function openToday() {
  hideCompletion();
  show(await call(() => api().TodayJournal()));
}

async function shiftDay(delta) {
  show(await call(() => api().Journal(page.rel, delta)));
}

function markActive() {
  document.querySelectorAll("#nav li").forEach((li) => {
    li.classList.toggle("active", li.dataset.rel === page.rel);
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

// Navigation happens on mousedown, not click: by the time a click arrives the
// block has taken focus, swapped itself to raw text and destroyed the very
// span that was pressed. preventDefault stops that focus from happening.
document.addEventListener("mousedown", (e) => {
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

document.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === "f") {
    e.preventDefault();
    $("search").focus();
    $("search").select();
  }
  if ((e.metaKey || e.ctrlKey) && e.key === ",") {
    e.preventDefault();
    openSettings();
  }
  if (e.key === "Escape" && !$("settings").hidden) {
    $("settings").hidden = true;
  }
  if ((e.metaKey || e.ctrlKey) && e.key === "t") {
    e.preventDefault();
    openToday();
  }
});

// Wails reports a file drop here. The files go on the shared shelf and their
// links into the page currently open.
window.runtime && window.runtime.OnFileDrop(async (x, y, paths) => {
  if (!paths || !paths.length || !page) return;
  document.body.classList.remove("dropping");
  const edit = await call(() => api().AttachFiles(page.rel, paths));
  if (edit) {
    show(edit.page);
    refreshIndex();
  }
}, true);

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

if (window.runtime && window.runtime.EventsOn) {
  window.runtime.EventsOn("notes:changed", async (ev) => {
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

(async function start() {
  const root = await call(() => api().Root());
  if (root) $("root").textContent = root;
  await refreshIndex();
  await openToday();
})();
