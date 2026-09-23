package markdown

import "testing"

func rowsEqual(t *testing.T, got [][]string, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("row %d width: got %v want %v", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("cell %d,%d: got %q want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestCSVAsTypedByHand(t *testing.T) {
	rows, ok := CSVTable("csv", "eins, zwei, drei\n1,2,3\n")
	if !ok {
		t.Fatal("did not parse")
	}
	rowsEqual(t, rows, [][]string{{"eins", "zwei", "drei"}, {"1", "2", "3"}})
}

func TestCSVDetectsTheSeparator(t *testing.T) {
	// A spreadsheet exported on a German-locale machine.
	rows, ok := CSVTable("csv", "Kürzel;Fachgebiet\nBZ;Zahlentheorie\n")
	if !ok {
		t.Fatal("did not parse")
	}
	rowsEqual(t, rows, [][]string{
		{"Kürzel", "Fachgebiet"},
		{"BZ", "Zahlentheorie"},
	})

	rows, ok = CSVTable("tsv", "a\tb\n1\t2\n")
	if !ok {
		t.Fatal("tsv did not parse")
	}
	rowsEqual(t, rows, [][]string{{"a", "b"}, {"1", "2"}})
}

func TestCSVQuotedFieldsKeepTheirCommas(t *testing.T) {
	rows, ok := CSVTable("csv", "name,note\n\"Lovelace, Ada\",\"said \"\"yes\"\"\"\n")
	if !ok {
		t.Fatal("did not parse")
	}
	rowsEqual(t, rows, [][]string{
		{"name", "note"},
		{"Lovelace, Ada", `said "yes"`},
	})
}

func TestCSVPadsRaggedRows(t *testing.T) {
	// A table being typed is ragged for most of its life.
	rows, ok := CSVTable("csv", "a,b,c\n1,2\n")
	if !ok {
		t.Fatal("did not parse")
	}
	rowsEqual(t, rows, [][]string{{"a", "b", "c"}, {"1", "2", ""}})
}

func TestCSVRejectsNothing(t *testing.T) {
	for _, body := range []string{"", "   ", "\n\n"} {
		if _, ok := CSVTable("csv", body); ok {
			t.Fatalf("empty body %q parsed as a table", body)
		}
	}
}

func TestCSVSingleColumnIsStillATable(t *testing.T) {
	rows, ok := CSVTable("csv", "name\nAda\nGrace\n")
	if !ok {
		t.Fatal("did not parse")
	}
	rowsEqual(t, rows, [][]string{{"name"}, {"Ada"}, {"Grace"}})
}

func TestIsCSVLang(t *testing.T) {
	for _, l := range []string{"csv", "CSV", " tsv ", "psv"} {
		if !IsCSVLang(l) {
			t.Fatalf("%q should be tabular", l)
		}
	}
	for _, l := range []string{"go", "", "sh"} {
		if IsCSVLang(l) {
			t.Fatalf("%q should not be tabular", l)
		}
	}
}

func TestFencesAreFoundInOrder(t *testing.T) {
	text := "intro\n```go\nx := 1\n```\nbetween\n```csv\na,b\n1,2\n```\ntail"
	got := Fences(text)
	if len(got) != 2 {
		t.Fatalf("fences: %+v", got)
	}
	if got[0].Lang != "go" || got[0].Body != "x := 1" {
		t.Fatalf("first: %+v", got[0])
	}
	if got[1].Lang != "csv" || got[1].Body != "a,b\n1,2" {
		t.Fatalf("second: %+v", got[1])
	}
	if len(Fences("no fences here")) != 0 {
		t.Fatal("found a fence in plain text")
	}
}

func TestUnclosedFenceIsStillAFence(t *testing.T) {
	got := Fences("```sh\necho hi")
	if len(got) != 1 || got[0].Lang != "sh" || got[0].Body != "echo hi" {
		t.Fatalf("got %+v", got)
	}
}
