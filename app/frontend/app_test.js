// Tests for the frontend's pure helpers, run with `node app/frontend/app_test.js`.
//
// The functions are lifted out of app.js rather than exported from it, so that
// app.js stays a plain script the WebView loads with no build step and no
// module system.

const fs = require("fs");
const path = require("path");

const src = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");

// Lift a top-level declaration by matching braces from its first one. Works for
// `function f(...) {...}` and `const X = {...}` alike.
function lift(decl) {
  const i = src.indexOf(decl);
  if (i < 0) throw new Error("app.js has no " + decl);
  let depth = 0;
  for (let k = src.indexOf("{", i); k < src.length; k++) {
    if (src[k] === "{") depth++;
    else if (src[k] === "}" && --depth === 0) {
      return src.slice(i, src[k + 1] === ";" ? k + 2 : k + 1);
    }
  }
  throw new Error("unbalanced braces in " + decl);
}

const escapeHTML = (s) =>
  s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));

// `const` declared inside eval stays inside it; `var` reaches this module.
eval(lift("const RE =").replace(/^const /, "var "));
eval(lift("function decorateInline"));
eval(lift("function tableHTML"));
eval(lift("function gridHTML"));
eval(lift("function fenceHTML"));
eval(lift("function decorate"));
eval(lift("function linkPrefix"));

let failures = 0;
function eq(label, got, want) {
  const g = typeof got === "string" ? got : JSON.stringify(got);
  const w = typeof want === "string" ? want : JSON.stringify(want);
  if (g !== w) {
    console.log(`FAIL  ${label}\n  got   ${g}\n  want  ${w}`);
    failures++;
  }
}

const ln = (s) => `<span class="ln">${s}</span>`;

// --- links, as they look when you are not in the block ----------------------

eq("a link shows its text, not its brackets",
  decorateInline("see [[Project Foo]]"),
  'see <span class="link" data-page="Project Foo">Project Foo</span>');

eq("an anchor is machinery and stays hidden",
  decorateInline("[[Rust#^a1b2c3]]"),
  '<span class="link" data-page="Rust">Rust</span>');

eq("a typed link shows the page and what it is",
  decorateInline("[[Ada Lovelace #person]]"),
  '<span class="link" data-page="Ada Lovelace">Ada Lovelace</span>' +
  '<span class="tag" data-tag="person">#person</span>');

eq("several types",
  decorateInline("[[A #person #colleague]]"),
  '<span class="link" data-page="A">A</span>' +
  '<span class="tag" data-tag="person">#person</span>' +
  '<span class="tag" data-tag="colleague">#colleague</span>');

eq("an ordinary markdown link keeps its label",
  decorateInline("see [the minutes](https://example.test/minutes)"),
  'see <span class="link" data-url="https://example.test/minutes">the minutes</span>');

eq("a bare url is a link",
  decorateInline("see https://github.com/tilman-schieber/pidboy now"),
  'see <span class="link" data-url="https://github.com/tilman-schieber/pidboy">' +
  "https://github.com/tilman-schieber/pidboy</span> now");

eq("an att file link shows its path, not the scheme",
  decorateInline("file:///Users/x/.att/store/report.pdf"),
  '<span class="link" data-url="file:///Users/x/.att/store/report.pdf">' +
  "/Users/x/.att/store/report.pdf</span>");

eq("an image is not read as a link",
  decorateInline("![a chart](chart.png)"),
  '<img src="chart.png" alt="a chart" loading="lazy">');

eq("a bare tag is a mention",
  decorateInline("about #research"),
  'about <span class="tag" data-tag="research">#research</span>');

// --- inline emphasis --------------------------------------------------------

eq("code", decorateInline("run `just install` now"), "run <code>just install</code> now");
eq("bold", decorateInline("this is **important**"), "this is <strong>important</strong>");
eq("italic", decorateInline("this is *subtle*"), "this is <em>subtle</em>");
eq("underscore italic", decorateInline("this is _subtle_"), "this is <em>subtle</em>");
eq("struck", decorateInline("~~dropped~~"), "<del>dropped</del>");

// --- things that must not be mangled ----------------------------------------

eq("html is escaped", decorateInline("a < b & c"), "a &lt; b &amp; c");
eq("a sharp in prose is not a tag", decorateInline("C# is fine"), "C# is fine");
eq("underscores in a page name are part of the name",
  decorateInline("[[a_b_c]]"),
  '<span class="link" data-page="a_b_c">a_b_c</span>');
eq("no emphasis inside code", decorateInline("`a_b_c`"), "<code>a_b_c</code>");
eq("a task marker is left for the checkbox", decorateInline("[ ] a task"), "[ ] a task");

eq("everything at once",
  decorateInline("**met** [[Ada #person]] re [notes](https://x.test) `v2`"),
  "<strong>met</strong> " +
  '<span class="link" data-page="Ada">Ada</span>' +
  '<span class="tag" data-tag="person">#person</span> re ' +
  '<span class="link" data-url="https://x.test">notes</span> <code>v2</code>');

// --- block-level constructs -------------------------------------------------

eq("a plain block is one run", decorate("just text"), ln("just text"));

eq("a fence becomes a code block, verbatim",
  decorate("run this\n```sh\n# a comment, not a tag\nrm *.tmp\n```"),
  ln("run this") +
  '<pre class="code" data-lang="sh"><code># a comment, not a tag\nrm *.tmp</code></pre>');

eq("a fence with no language",
  decorate("```\nplain\n```"),
  '<pre class="code"><code>plain</code></pre>');

eq("an unclosed fence still ends the block",
  decorate("```\n#not a tag"),
  '<pre class="code"><code>#not a tag</code></pre>');

eq("a quote run",
  decorate("> Dear all,\n> the meeting is moved."),
  "<blockquote>Dear all,\nthe meeting is moved.</blockquote>");

eq("numbered lines become an ordered list",
  decorate("Die Regeln sind:\n1. first\n2. second"),
  ln("Die Regeln sind:") + "<ol><li>first</li><li>second</li></ol>");

eq("a list that does not start at one keeps its number",
  decorate("5. fifth\n6. sixth"),
  '<ol start="5"><li>fifth</li><li>sixth</li></ol>');

eq("dashes become a bulleted list",
  decorate("und:\n- one\n- two"),
  ln("und:") + "<ul><li>one</li><li>two</li></ul>");

eq("a table needs its separator row",
  decorate("| Kürzel | Fachgebiet |\n|--------|-------------|\n| BZ | Zahlentheorie |"),
  "<table><thead><tr><th>Kürzel</th><th>Fachgebiet</th></tr></thead>" +
  "<tbody><tr><td>BZ</td><td>Zahlentheorie</td></tr></tbody></table>");

eq("pipes without a separator row are just text",
  decorate("| not | a table |"),
  ln("| not | a table |"));

eq("a heading", decorate("## Gemeinsame Bausteine"),
  '<b class="h h2">Gemeinsame Bausteine</b>');

eq("a horizontal rule", decorate("above\n---\nbelow"),
  ln("above") + "<hr>" + ln("below"));

eq("runs are kept apart",
  decorate("intro\n1. one\ntail\n- a"),
  ln("intro") + "<ol><li>one</li></ol>" + ln("tail") + "<ul><li>a</li></ul>");

eq("inline syntax still works inside a list",
  decorate("1. see [[Project Foo]] and **this**"),
  '<ol><li>see <span class="link" data-page="Project Foo">Project Foo</span> and ' +
  "<strong>this</strong></li></ol>");

// --- fences prepared by the core ---------------------------------------------

// The core scans fences with the same rule, so the nth fence here is the nth
// FenceView there. These are the shapes it sends.

eq("a highlighted fence uses the tokens it was given",
  decorate("```go\nx := 1\n```", [
    { lang: "go", code: "x := 1", tokens: [
      { text: "x " }, { kind: "keyword", text: "func" }, { text: " 1" },
    ] },
  ]),
  '<pre class="code" data-lang="go"><code>x <span class="t-keyword">func</span> 1</code></pre>');

eq("a csv fence becomes a table",
  decorate("```csv\neins, zwei\n1,2\n```", [
    { lang: "csv", code: "eins, zwei\n1,2", rows: [["eins", "zwei"], ["1", "2"]] },
  ]),
  '<table class="grid" data-lang="csv"><thead><tr><th>eins</th><th>zwei</th></tr></thead>' +
  "<tbody><tr><td>1</td><td>2</td></tr></tbody></table>");

eq("a fence with nothing prepared still shows its code",
  decorate("```\nplain\n```", null),
  '<pre class="code"><code>plain</code></pre>');

eq("code is escaped even when highlighted",
  decorate("```go\n```", [{ lang: "go", code: "", tokens: [{ text: "a < b" }] }]),
  '<pre class="code" data-lang="go"><code>a &lt; b</code></pre>');

eq("cell contents are still decorated",
  decorate("```csv\nx\n```", [{ lang: "csv", rows: [["who"], ["[[Ada]]"]] }]),
  '<table class="grid" data-lang="csv"><thead><tr><th>who</th></tr></thead><tbody>' +
  '<tr><td><span class="link" data-page="Ada">Ada</span></td></tr></tbody></table>');

// --- the completion trigger -------------------------------------------------

eq("page prefix", linkPrefix("see [[Pro"), { kind: "page", prefix: "Pro" });
eq("just opened", linkPrefix("see [["), { kind: "page", prefix: "" });
eq("a closed link does not trigger", linkPrefix("see [[Done]] and"), null);
eq("no brackets", linkPrefix("nothing here"), null);
eq("a hash switches to tags", linkPrefix("[[Ada #per"), { kind: "tag", prefix: "per" });
eq("a newline ends it", linkPrefix("[[multi\nline"), null);

console.log(failures === 0 ? "frontend: all pass" : `frontend: ${failures} FAILURES`);
process.exit(failures ? 1 : 0);
