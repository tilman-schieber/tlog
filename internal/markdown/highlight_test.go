package markdown

import (
	"strings"
	"testing"
)

// join reassembles the tokens; nothing may be lost or invented, whatever the
// language. This is the invariant that matters — a highlighter that drops a
// character silently corrupts what the reader sees.
func join(toks []Token) string {
	var sb strings.Builder
	for _, t := range toks {
		sb.WriteString(t.Text)
	}
	return sb.String()
}

func kindOf(toks []Token, text string) TokenKind {
	for _, t := range toks {
		if t.Text == text {
			return t.Kind
		}
	}
	return "missing"
}

func TestHighlightLosesNothing(t *testing.T) {
	samples := map[string]string{
		"go":     "func main() {\n\tx := 1 // count\n\tfmt.Println(\"hi\", x)\n}\n",
		"js":     "const a = `t${x}`; // note\n/* block */ let b = 0x1f;\n",
		"python": "def f(x):\n    # comment\n    return 'a' + str(x)\n",
		"sh":     "for f in *.md; do\n  echo \"$f\"  # each\ndone\n",
		"sql":    "select * from t where a = 'x' -- why\n",
		"json":   "{\"a\": 1, \"b\": null}\n",
		"yaml":   "key: value  # note\nlist:\n  - true\n",
		"":       "anything at all\n",
		"brainf": "+++[->+++<]\n",
	}
	for lang, code := range samples {
		got := join(Highlight(lang, code))
		if got != code {
			t.Fatalf("%s: tokens do not reassemble\ngot  %q\nwant %q", lang, got, code)
		}
	}
}

func TestHighlightClassifies(t *testing.T) {
	toks := Highlight("go", "func f() { s := \"hi\" // done\n\tn := 42 }")
	if k := kindOf(toks, "func"); k != TokKeyword {
		t.Fatalf("func: %q", k)
	}
	if k := kindOf(toks, `"hi"`); k != TokString {
		t.Fatalf("string: %q", k)
	}
	if k := kindOf(toks, "42"); k != TokNumber {
		t.Fatalf("number: %q", k)
	}
	if k := kindOf(toks, "// done"); k != TokComment {
		t.Fatalf("comment: %q", k)
	}
}

func TestSQLIsCaseInsensitive(t *testing.T) {
	for _, code := range []string{"select a from t", "SELECT a FROM t"} {
		toks := Highlight("sql", code)
		if kindOf(toks, strings.Fields(code)[0]) != TokKeyword {
			t.Fatalf("%q: first word not a keyword", code)
		}
	}
}

func TestUnknownLanguageIsOnePlainToken(t *testing.T) {
	toks := Highlight("cobol", "MOVE X TO Y.")
	if len(toks) != 1 || toks[0].Kind != TokPlain {
		t.Fatalf("got %+v", toks)
	}
	if HighlightSupported("cobol") {
		t.Fatal("cobol should not claim support")
	}
	if !HighlightSupported("Go") || !HighlightSupported("bash") {
		t.Fatal("aliases and casing should resolve")
	}
}

func TestUnterminatedConstructsDoNotSwallowEverything(t *testing.T) {
	// Notes are full of half-pasted code.
	toks := Highlight("go", "s := \"unclosed\nx := 1\n")
	if join(toks) != "s := \"unclosed\nx := 1\n" {
		t.Fatal("text lost")
	}
	if kindOf(toks, "x") == TokString {
		t.Fatal("an unterminated quote swallowed the next line")
	}

	toks = Highlight("go", "/* unclosed\nstill comment")
	if join(toks) != "/* unclosed\nstill comment" {
		t.Fatal("text lost")
	}
}

func TestIdentifiersWithDigitsAreNotNumbers(t *testing.T) {
	toks := Highlight("go", "utf8 x1 = 12")
	if kindOf(toks, "12") != TokNumber {
		t.Fatal("12 should be a number")
	}
	for _, tok := range toks {
		if tok.Kind == TokNumber && strings.ContainsAny(tok.Text, "utfx") {
			t.Fatalf("identifier split into a number: %+v", tok)
		}
	}
}

func TestEmptyCode(t *testing.T) {
	if toks := Highlight("go", ""); len(toks) != 0 {
		t.Fatalf("got %+v", toks)
	}
}
