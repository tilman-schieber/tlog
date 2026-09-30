// Does the window actually come up?
//
// app_test.js checks the pure helpers — decorate, slashAt, shouldReload — and
// found real bugs. It could not find these, because nothing here had ever run
// the page: three separate crashes, all present since the first commit, all of
// which left the window drawing an empty pane.
//
//   - markActive read page.rel before the first page was open, so start() threw
//     and never opened today's journal
//   - markdown.Prop had no json tags, so the wire said Key/Value while this
//     file reads p.key, and any page with a block property threw while drawing
//   - the checkbox was drawn beside a task and the [ ] marker left in the text
//
// So this boots the real index.html and the real app.js against a stubbed Go
// API and asserts the page is there. jsdom is not vendored: without it the file
// says so and passes, because a missing test dependency is not a failing test.

let JSDOM;
try {
  ({ JSDOM } = require("jsdom"));
} catch {
  console.log("frontend smoke: skipped (npm install jsdom to enable)");
  process.exit(0);
}

const fs = require("fs");
const path = require("path");
const here = __dirname;

// One page with everything that has ever broken drawing: a task, a deadline, a
// block property, a fence, a link, a tag, a block reference and an embed.
const PAGE = {
  rel: "journals/2026-09-25.md",
  title: "2026-09-25",
  hash: "abc123",
  isJournal: true,
  tags: [],
  blocks: [
    {
      addr: "journals/2026-09-25.md:0@abc123", offset: 0, depth: 0,
      text: "meeting with [[Ada Lovelace]] about [[analytical engine #project]]",
      body: "meeting with [[Ada Lovelace]] about [[analytical engine #project]]",
      hasChildren: true,
    },
    {
      addr: "journals/2026-09-25.md:70@abc123", offset: 70, depth: 1,
      text: "[ ] she will send the notes", body: "she will send the notes",
      task: "open", hasChildren: false,
      props: [{ key: "Deadline", value: "2026-09-26" }],
      due: "2026-09-26", dueState: "soon", dueLabel: "morgen",
    },
    {
      addr: "journals/2026-09-25.md:130@abc123", offset: 130, depth: 0,
      text: "a fence:\n```go\nfunc main() {}\n```", body: "a fence:\n```go\nfunc main() {}\n```",
      hasChildren: false,
      fences: [{ lang: "go", code: "func main() {}", tokens: [] }],
    },
    {
      addr: "journals/2026-09-25.md:200@abc123", offset: 200, depth: 0,
      text: "remember: [[Timetable#^k3f9q2]]", body: "remember: [[Timetable#^k3f9q2]]",
      hasChildren: false,
      embeds: [{ anchor: "k3f9q2", page: "Timetable", rel: "pages/Timetable.md",
                 addr: "pages/Timetable.md:12@def456", text: "the lecture is at nine" }],
    },
    {
      addr: "journals/2026-09-25.md:250@abc123", offset: 250, depth: 0,
      text: "[[Timetable#^k3f9q2]]", body: "[[Timetable#^k3f9q2]]",
      hasChildren: false, isEmbed: true,
      embeds: [{ anchor: "k3f9q2", page: "Timetable", rel: "pages/Timetable.md",
                 addr: "pages/Timetable.md:12@def456", text: "the lecture is at nine" }],
      embedKids: [{ anchor: "", page: "Timetable", rel: "pages/Timetable.md",
                    addr: "pages/Timetable.md:40@def456", text: "room B103", depth: 1 }],
    },
  ],
  tagged: [],
  refs: [
    { addr: "pages/Ada Lovelace.md:0@x", page: "Ada Lovelace", rel: "pages/Ada Lovelace.md",
      offset: 0, text: "she mentioned it", depth: 0, head: true },
    { addr: "pages/Ada Lovelace.md:30@x", page: "Ada Lovelace", rel: "pages/Ada Lovelace.md",
      offset: 30, text: "[ ] follow up", body: "follow up", task: "open", depth: 1 },
  ],
};

const OTHER = {
  rel: "pages/Timetable.md", title: "Timetable", hash: "def456",
  isJournal: false, tags: [], tagged: [], refs: [],
  blocks: [{ addr: "pages/Timetable.md:0@def456", offset: 0, depth: 0,
             text: "the lecture is at nine", body: "the lecture is at nine", hasChildren: false }],
};

const opened = [];

const API = {
  Root: async () => "/home/ada/notes",
  Index: async () => ({ journals: ["2026-09-25"], pages: ["Ada Lovelace", "Timetable"], tags: ["project"] }),
  TodayJournal: async () => { opened.push("TodayJournal"); return PAGE; },
  Today: async () => { opened.push("Today"); return PAGE; },
  InsertBefore: async () => ({ page: PAGE, offset: 0 }),
  SetText: async () => ({ page: PAGE, offset: 0 }),
  DeleteBlock: async () => ({ page: PAGE, offset: 0 }),
  SplitBlock: async () => ({ page: PAGE, offset: 0 }),
  Indent: async () => ({ page: PAGE, offset: 0 }),
  Outdent: async () => ({ page: PAGE, offset: 0 }),
  Move: async () => ({ page: PAGE, offset: 0 }),
  MergeIntoPrevious: async () => ({ page: PAGE, offset: 0 }),
  OpenRel: async (rel) => (rel === "pages/Timetable.md" ? OTHER : PAGE),
  OpenPage: async () => OTHER,
  Journal: async () => PAGE,
  Search: async () => [{ addr: "a:0@b", rel: "a", page: "Ada Lovelace", offset: 0, hash: "b", text: "a hit" }],
  Due: async () => [{ addr: "a:0@b", rel: "a", page: "Ada Lovelace", offset: 0, hash: "b",
                      text: "follow up", due: "2026-09-26", state: "soon", label: "morgen" }],
  Sync: async () => ({ lastError: "" }),
  Settings: async () => [
    { key: "notes", value: "~/notes", kind: "text", hint: "where the notes live", source: "config" },
    { key: "git.autocommit", value: "true", kind: "bool", hint: "commit as you write", source: "config" },
  ],
  ConfigPath: async () => "~/.config/tlog/config.toml",
  Commands: async () => [],
  CompletePages: async () => [],
  CompleteTags: async () => [],
  Blocks: async () => [],
  AttachDir: async () => "~/.att",
  Attachments: async () => [],
};

let failures = 0;
function ok(label, cond, detail) {
  if (!cond) {
    console.log(`FAIL  ${label}${detail ? "\n  " + detail : ""}`);
    failures++;
  }
}

(async () => {
  const dom = new JSDOM(fs.readFileSync(path.join(here, "index.html"), "utf8"), {
    runScripts: "outside-only",
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  // The stylesheet is linked, not inlined, and jsdom does not fetch it. It has
  // to be here: whether an element is actually hidden is a question about the
  // cascade, and [hidden] loses to any id selector that sets display.
  const style = dom.window.document.createElement("style");
  style.textContent = fs.readFileSync(path.join(here, "style.css"), "utf8");
  dom.window.document.head.appendChild(style);

  // jsdom has no innerText, and raw() is built on it — so without this shim
  // every editing path is silently unreachable here: splitting, indenting,
  // merging and saving on blur all start by reading the block's text and would
  // throw on undefined before doing anything. A contenteditable holds one text
  // node and <br> for the line breaks, which is what this reproduces.
  Object.defineProperty(dom.window.HTMLElement.prototype, "innerText", {
    configurable: true,
    get() {
      return [...this.childNodes]
        .map((n) => (n.nodeName === "BR" ? "\n" : n.textContent))
        .join("");
    },
    set(v) {
      this.textContent = v;
    },
  });

  const w = dom.window;
  w.go = { main: { API } };
  w.runtime = undefined;

  const errors = [];
  w.addEventListener("error", (e) => errors.push(e.error ? e.error.stack : e.message));
  w.addEventListener("unhandledrejection", (e) => errors.push(e.reason && e.reason.stack));
  // An exception in start() surfaces as a rejected promise, which by default
  // takes the whole process down before a single assertion has run — the
  // failure would be real but the report would be a stack trace and nothing
  // else. Recording it instead is what lets this file say which part broke.
  process.on("unhandledRejection", (r) => errors.push("unhandled: " + (r && r.stack ? r.stack : r)));
  process.on("uncaughtException", (e) => errors.push("uncaught: " + e.stack));

  try {
    w.eval(fs.readFileSync(path.join(here, "app.js"), "utf8"));
  } catch (e) {
    errors.push("threw while loading: " + e.stack);
  }
  await new Promise((r) => setTimeout(r, 300));

  const $ = (id) => w.document.getElementById(id);
  ok("the page comes up without throwing", errors.length === 0, errors.join("\n  "));

  // Not "the attribute is set" — whether it is actually off the screen.
  const shown = (id) => w.getComputedStyle($(id)).display !== "none";
  ok("the settings sheet is not covering the page", !shown("settings"));
  ok("the completion popup is not floating over the page", !shown("complete"));
  ok("the title is drawn", $("title").textContent === "2026-09-25", $("title").textContent);
  ok("the outline is drawn", $("outline").children.length > 0);
  ok("the sidebar is filled", $("pages").children.length === 2);

  // `startup = last` is a setting in all three menus. The window called
  // TodayJournal at launch, which always opens today, so the setting did
  // nothing here while the outliner honoured it.
  ok("the window opens where the startup setting says",
    opened[0] === "Today", JSON.stringify(opened));

  const outline = $("outline").textContent;
  ok("a task does not show its marker twice", !outline.includes("[ ]"), outline.slice(0, 200));
  ok("a task shows a checkbox", $("outline").querySelector(".check") !== null);
  ok("a block property is drawn", $("outline").querySelector(".props") === null ||
    $("outline").querySelector(".props").textContent.includes("Deadline"));
  ok("a deadline chip is drawn", $("outline").querySelector(".due") !== null);
  ok("a fence is highlighted", $("outline").querySelector("pre.code") !== null);
  ok("a link is drawn as its name", outline.includes("Ada Lovelace"));

  ok("a reference draws the block it points at", outline.includes("the lecture is at nine"),
    "got: " + outline.slice(-160));
  ok("a reference does not leak its anchor", !outline.includes("#^"));
  ok("an embed brings its subtree", outline.includes("room B103"));
  ok("embedded rows are marked as such", $("outline").querySelector(".block.embedded") !== null);

  const refs = $("reflist").textContent;
  ok("backlinks are drawn", refs.includes("she mentioned it"));
  ok("a task in a backlink does not show its marker", !refs.includes("[ ]"), refs);

  await w.eval("openSettings()");
  await new Promise((r) => setTimeout(r, 100));
  ok("settings open", $("settings").hidden === false);
  ok("settings are listed", $("settingslist").querySelectorAll(".setting").length === 2);
  ok("a text setting carries its value",
    $("settingslist").querySelector("input[type=text]").value === "~/notes");

  $("settings").hidden = true;

  await w.eval("openAgenda()");
  await new Promise((r) => setTimeout(r, 100));
  ok("the agenda opens", $("agenda").hidden === false);
  ok("the agenda lists what is due", $("agendalist").textContent.includes("follow up"));

  // --- going back ---------------------------------------------------------
  //
  // Following a link used to be a one-way door: the only route back was
  // finding the page again in the sidebar.
  await w.eval(`openRel("pages/Timetable.md")`);
  await new Promise((r) => setTimeout(r, 100));
  ok("following a link arrives", $("title").textContent === "Timetable");
  ok("back is offered once there is somewhere to go", $("back").disabled === false);

  await w.eval("goBack()");
  await new Promise((r) => setTimeout(r, 100));
  ok("back returns to where you were", $("title").textContent === "2026-09-25",
    $("title").textContent);
  ok("forward is offered after going back", $("fwd").disabled === false);
  ok("back is not offered at the start", $("back").disabled === true);

  await w.eval("goForward()");
  await new Promise((r) => setTimeout(r, 100));
  ok("forward lands where back came from", $("title").textContent === "Timetable");

  // Arriving at the page you are already on is not a move.
  const depth = await w.eval("history.length");
  await w.eval(`openRel("pages/Timetable.md")`);
  await new Promise((r) => setTimeout(r, 100));
  ok("re-opening the current page does not stack up history",
    (await w.eval("history.length")) === depth);

  // --- folding ------------------------------------------------------------
  //
  // The window could not fold anything: hasChildren changed a bullet's colour
  // and nothing else, so a long journal had to be read in full. Driven by
  // clicking the bullet, which is how anyone else would do it.
  await w.eval(`openRel("journals/2026-09-25.md")`);
  await new Promise((r) => setTimeout(r, 100));
  const rows = () => $("outline").querySelectorAll(".block").length;
  const parentBullet = () => $("outline").querySelector(".block .bullet.haskids");
  const before = rows();
  ok("the child is on screen to begin with", $("outline").textContent.includes("she will send"));
  ok("a parent has a foldable bullet", parentBullet() !== null);

  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));
  ok("folding hides the subtree", rows() < before, `${rows()} of ${before}`);
  ok("and the child is gone", !$("outline").textContent.includes("she will send"));
  ok("the parent is still there", $("outline").textContent.includes("meeting with"));
  ok("the bullet says it is folded", $("outline").querySelector(".bullet.folded") !== null);

  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));
  ok("unfolding brings it back", rows() === before);
  ok("and the child is back", $("outline").textContent.includes("she will send"));

  // A bullet folds and does nothing else. It used to make the block a task,
  // so a stray click rewrote a note into a checkbox.
  const plain = [...$("outline").querySelectorAll(".block .bullet")]
    .find((el) => !el.classList.contains("haskids"));
  ok("a childless block has an inert bullet", plain && plain.onclick === null);
  let toggled = false;
  const realToggle = API.ToggleTask;
  API.ToggleTask = async () => { toggled = true; return { page: PAGE, offset: 0 }; };
  if (plain) plain.click();
  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));
  ok("clicking a bullet never makes the block a task", toggled === false);
  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));

  // The checkbox still ticks off — that is what a checkbox is for.
  $("outline").querySelector(".check").click();
  await new Promise((r) => setTimeout(r, 50));
  ok("a checkbox still ticks off", toggled === true);
  API.ToggleTask = realToggle;

  // A fold belongs to the block, not to the row it happened to be on.
  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));
  await w.eval(`openRel("pages/Timetable.md")`);
  await new Promise((r) => setTimeout(r, 100));
  ok("a fold does not follow you to another page",
    $("outline").querySelector(".bullet.folded") === null);
  await w.eval("goBack()");
  await new Promise((r) => setTimeout(r, 100));
  ok("and is still there when you come back",
    $("outline").querySelector(".bullet.folded") !== null);
  parentBullet().click();
  await new Promise((r) => setTimeout(r, 50));

  // --- deleting, and inserting above ---------------------------------------
  //
  // The window could only delete an empty first block, so a paragraph could
  // not be thrown away at all; and it had no way to make a block above one.
  {
    let deleted = null, inserted = false;
    const realDelete = API.DeleteBlock, realInsert = API.InsertBefore;
    API.DeleteBlock = async (rel, offset) => { deleted = offset; return { page: PAGE, offset: 0 }; };
    API.InsertBefore = async () => { inserted = true; return { page: PAGE, offset: 0 }; };

    const block = $("outline").querySelector(".text");
    const key = (init) => block.dispatchEvent(new w.KeyboardEvent("keydown",
      { bubbles: true, cancelable: true, ...init }));

    key({ key: "Backspace", metaKey: true });
    await new Promise((r) => setTimeout(r, 60));
    ok("cmd-backspace deletes the block", deleted === 0, String(deleted));
    ok("and says git has it", $("error").textContent.includes("git has the previous version"),
      $("error").textContent);
    ok("it counts what goes with it", $("error").textContent.includes("child"),
      $("error").textContent);

    key({ key: "Enter", metaKey: true, shiftKey: true });
    await new Promise((r) => setTimeout(r, 60));
    ok("cmd-shift-enter makes a block above", inserted === true);

    API.DeleteBlock = realDelete;
    API.InsertBefore = realInsert;
  }

  // --- the keyboard -------------------------------------------------------
  await w.eval("toggleHelp()");
  ok("the keyboard sheet opens", $("help").hidden === false);
  ok("it lists the bindings from the table",
    $("helplist").textContent.includes("Search") &&
    $("helplist").textContent.includes("Back") &&
    $("helplist").textContent.includes("Agenda"));
  await w.eval("toggleHelp()");
  ok("and closes again", $("help").hidden === true);

  // --- what the window says ------------------------------------------------
  //
  // An error used to appear for six seconds and then be gone with no way to
  // get it back. If you were looking at the other screen when the push failed,
  // it never happened.
  $("help").hidden = true;
  $("log").hidden = true;

  await w.eval(`say("a quiet receipt")`);
  ok("a note is shown", $("error").hidden === false);
  ok("a note is not styled as a failure", !$("error").classList.contains("bad"));

  // A note removes itself, and the timer it sets is the evidence.
  ok("a note schedules its own disappearance", (await w.eval("!!say.timer")) === true);

  await w.eval("clearTimeout(say.timer); say.timer = undefined");
  await w.eval(`fail(new Error("push rejected"))`);
  ok("an error is shown", $("error").hidden === false);
  ok("an error is styled as one", $("error").classList.contains("bad"));
  ok("an error says what happened", $("error").textContent.includes("push rejected"));
  ok("an error does not schedule its own disappearance",
    (await w.eval("say.timer")) === undefined);

  $("error").click();
  await new Promise((r) => setTimeout(r, 50));
  ok("dismissing an error opens the log rather than losing it", $("log").hidden === false);
  ok("the log has both lines", $("loglist").textContent.includes("push rejected") &&
    $("loglist").textContent.includes("a quiet receipt"));
  ok("the log is newest first",
    $("loglist").textContent.indexOf("push rejected") <
      $("loglist").textContent.indexOf("a quiet receipt"));
  $("log").hidden = true;

  console.log(failures === 0 ? "frontend smoke: all pass" : `frontend smoke: ${failures} FAILURES`);
  process.exit(failures ? 1 : 0);
})().catch((e) => {
  // Recording unhandled rejections lets the page's own failures be reported,
  // but it also means a mistake in this file would end the run silently with
  // a success. It must not be possible to pass by not finishing.
  console.log("frontend smoke: the test itself threw\n" + (e && e.stack ? e.stack : e));
  process.exit(1);
});
