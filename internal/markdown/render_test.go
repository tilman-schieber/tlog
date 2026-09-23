package markdown

import (
	"strings"
	"testing"
)

// canonical inputs must survive a parse/render round trip byte for byte. This
// is the invariant the whole write path depends on: because tlog owns the
// format, a full re-render of a canonical file differs only where something
// actually changed, which is what keeps git diffs small without a
// source-preserving patcher.
func TestRoundTripCanonical(t *testing.T) {
	cases := map[string]string{
		"flat":         "- one\n\n- two\n",
		"nested":       "- one\n  - two\n    - three\n\n- four\n",
		"multiline":    "- first\n  second\n  third\n",
		"props":        "- task\n  status:: doing\n  due:: 2026-09-20\n",
		"anchor":       "- named ^k3f9q2\n",
		"task":         "- [ ] open\n\n- [x] done\n",
		"fence":        "- code\n  ```go\n  x := 1\n  if x > 0 {\n  \tfmt.Println(x)\n  }\n  ```\n",
		"front":        "---\ntitle: Foo\n---\n\n- body\n",
		"mixed":        "---\ntitle: Foo\n---\n\n- a\n  cont\n  k:: v\n  - b ^abc123\n\n- c\n",
		"empty-bullet": "-\n\n- x\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := string(Render(Parse([]byte(src))))
			if got != src {
				t.Fatalf("round trip changed the file\n--- got ---\n%s\n--- want ---\n%s", got, src)
			}
			if !IsCanonical([]byte(src)) {
				t.Fatal("IsCanonical disagrees with the round trip")
			}
		})
	}
}

// Rendering must be idempotent even for input that is not canonical: the first
// write normalises, every later write is a no-op.
func TestRenderIsIdempotent(t *testing.T) {
	messy := []string{
		"*   not our bullet\n",
		"- a\n\t- b\n\t\t\t- c\n",
		"- a\n\n\n\n- b\n",
		"   - indented root\n",
		"- trailing spaces   \n  cont   \n",
		"- a\n- b",
	}
	for _, src := range messy {
		once := Render(Parse([]byte(src)))
		twice := Render(Parse(once))
		if string(once) != string(twice) {
			t.Fatalf("normalisation not stable for %q\nonce:  %q\ntwice: %q", src, once, twice)
		}
	}
}

func TestRenderNormalisesIndentation(t *testing.T) {
	got := string(Render(Parse([]byte("- a\n\t- b\n\t\t- c\n"))))
	want := "- a\n  - b\n    - c\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderKeepsFenceIndentation(t *testing.T) {
	src := "- x\n  ```\n  outer\n    inner\n  ```\n"
	got := string(Render(Parse([]byte(src))))
	if got != src {
		t.Fatalf("fence indentation lost\ngot  %q\nwant %q", got, src)
	}
}

func TestRenderBlockSubtree(t *testing.T) {
	doc := Parse([]byte("- a\n  - b\n"))
	got := string(RenderBlock(doc.Blocks[0], 2))
	want := "    - a\n      - b\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNoContentIsLostForRealisticInput(t *testing.T) {
	// A shape lifted from the Logseq mirror: nested outline, a leading TODO
	// marker, an indented property child, a wikilink.
	src := strings.Join([]string{
		"- [[Grace Hopper]]",
		"  - Projekt 1.9.26 - 31.8.2027",
		"    - Zwanzig Stunden für [[Difference Engine]]",
		"    - TODO Stunden vormerken",
		"      Deadline:: 2026-09-16",
		"",
	}, "\n")
	doc := Parse([]byte(src))
	var texts []string
	doc.Walk(func(b *Block) bool {
		texts = append(texts, b.Text)
		return true
	})
	joined := strings.Join(texts, "\n")
	for _, must := range []string{"Grace Hopper", "Projekt", "Difference Engine", "Stunden vormerken"} {
		if !strings.Contains(joined, must) {
			t.Fatalf("lost %q from realistic input", must)
		}
	}
	deep := doc.Blocks[0].Children[0].Children[1]
	if v, ok := deep.Prop("Deadline"); !ok || v != "2026-09-16" {
		t.Fatalf("deadline property: %q %v", v, ok)
	}
}
