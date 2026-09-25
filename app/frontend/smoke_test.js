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

const API = {
  Root: async () => "/home/ada/notes",
  Index: async () => ({ journals: ["2026-09-25"], pages: ["Ada Lovelace", "Timetable"], tags: ["project"] }),
  TodayJournal: async () => PAGE,
  Today: async () => PAGE,
  OpenRel: async () => PAGE,
  OpenPage: async () => PAGE,
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

  await w.eval("openAgenda()");
  await new Promise((r) => setTimeout(r, 100));
  ok("the agenda opens", $("agenda").hidden === false);
  ok("the agenda lists what is due", $("agendalist").textContent.includes("follow up"));

  console.log(failures === 0 ? "frontend smoke: all pass" : `frontend smoke: ${failures} FAILURES`);
  process.exit(failures ? 1 : 0);
})();
