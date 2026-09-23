package markdown

import (
	"encoding/csv"
	"strings"
)

// A pipe table is miserable to type and worse to edit — every column has to be
// re-aligned by hand. A ```csv fence is not: you type rows, and tlog draws the
// table. It is still standard markdown, so anything else shows it as a code
// block rather than as nonsense, and nothing new has to be stored.
//
//	```csv
//	eins, zwei, drei
//	1, 2, 3
//	```

// CSVLangs are the fence languages rendered as a table rather than as code.
var CSVLangs = map[string]bool{"csv": true, "tsv": true, "psv": true}

// IsCSVLang reports whether a fence tagged with this language is tabular.
func IsCSVLang(lang string) bool {
	return CSVLangs[strings.ToLower(strings.TrimSpace(lang))]
}

// CSVTable parses a fence body into rows. The second result is false when the
// text does not parse, in which case the caller should fall back to showing it
// as code: a half-typed table is still worth seeing.
//
// The delimiter is detected rather than assumed, because a spreadsheet exported
// on a German-locale machine uses semicolons and nobody should have to care.
func CSVTable(lang, body string) ([][]string, bool) {
	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) == "" {
		return nil, false
	}

	r := csv.NewReader(strings.NewReader(body))
	r.Comma = delimiterFor(lang, body)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // ragged rows are a work in progress, not an error

	rows, err := r.ReadAll()
	if err != nil || len(rows) == 0 {
		return nil, false
	}

	// Pad every row to the widest, so a half-finished line still lines up.
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	for i, row := range rows {
		for len(row) < width {
			row = append(row, "")
		}
		for j := range row {
			row[j] = strings.TrimSpace(row[j])
		}
		rows[i] = row
	}
	return rows, true
}

// delimiterFor honours an explicit tsv or psv tag, and otherwise picks whichever
// separator appears most often in the first line.
func delimiterFor(lang, body string) rune {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "tsv":
		return '\t'
	case "psv":
		return '|'
	}
	first := body
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		first = body[:i]
	}
	best, bestN := ',', strings.Count(first, ",")
	for _, c := range []rune{';', '\t', '|'} {
		if n := strings.Count(first, string(c)); n > bestN {
			best, bestN = c, n
		}
	}
	return best
}

// Fence is one fenced region of a block's text.
type Fence struct {
	Lang string
	Body string
}

// Fences finds the fenced regions of a block's text, in order. Both adapters
// scan with this same rule, so the nth fence means the same thing on either
// side of the bridge.
func Fences(text string) []Fence {
	if !strings.Contains(text, "```") {
		return nil
	}
	var out []Fence
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		if !isFenceLine(lines[i]) {
			continue
		}
		f := Fence{Lang: strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(lines[i]), "`"))}
		var body []string
		i++
		for i < len(lines) && !isFenceLine(lines[i]) {
			body = append(body, lines[i])
			i++
		}
		f.Body = strings.Join(body, "\n")
		out = append(out, f)
	}
	return out
}

func isFenceLine(l string) bool { return strings.HasPrefix(strings.TrimLeft(l, " \t"), "```") }
