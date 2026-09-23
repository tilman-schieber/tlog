package markdown

import "strings"

// Syntax highlighting lives here rather than in either adapter, so the outliner
// and the desktop app colour code the same way and a JSON caller gets the same
// tokens. It is deliberately a small generic scanner driven by a per-language
// table: enough to read a snippet in a note, not a compiler front end.
//
// A language that is not in the table yields one plain token, which renders as
// monospace — exactly what happens today.

// TokenKind is the semantic role of a run of code, mapped to a colour by
// whichever adapter is drawing.
type TokenKind string

const (
	TokPlain   TokenKind = ""
	TokKeyword TokenKind = "keyword"
	TokType    TokenKind = "type"
	TokString  TokenKind = "string"
	TokNumber  TokenKind = "number"
	TokComment TokenKind = "comment"
)

// Token is a run of code with one role.
type Token struct {
	Kind TokenKind `json:"kind,omitempty"`
	Text string    `json:"text"`
}

type langSpec struct {
	lineComment  []string
	blockComment [2]string
	quotes       string // characters that open and close a string
	keywords     map[string]bool
	types        map[string]bool
}

func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(s) {
		out[w] = true
	}
	return out
}

// The languages actually written in these notes. Adding one is a table entry,
// not code — and until a missing language is a real complaint rather than an
// imagined one, that is the whole of it.
var langs = map[string]*langSpec{
	"go": {
		lineComment:  []string{"//"},
		blockComment: [2]string{"/*", "*/"},
		quotes:       "\"'`",
		keywords: words(`break case chan const continue default defer else fallthrough for func go goto
			if import interface map package range return select struct switch type var`),
		types: words(`bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64
			rune string uint uint8 uint16 uint32 uint64 uintptr any nil true false iota make new len cap append`),
	},
	"js": {
		lineComment:  []string{"//"},
		blockComment: [2]string{"/*", "*/"},
		quotes:       "\"'`",
		keywords: words(`async await break case catch class const continue debugger default delete do else
			export extends finally for from function if import in instanceof let new of return static super
			switch this throw try typeof var void while yield`),
		types: words(`true false null undefined NaN Infinity console document window Promise Array Object
			String Number Boolean Math JSON`),
	},
	"python": {
		lineComment: []string{"#"},
		quotes:      "\"'",
		keywords: words(`and as assert async await break class continue def del elif else except finally
			for from global if import in is lambda nonlocal not or pass raise return try while with yield match case`),
		types: words(`True False None self bool bytes dict float int list object set str tuple len range
			print open enumerate zip map filter`),
	},
	"sh": {
		lineComment: []string{"#"},
		quotes:      "\"'",
		keywords: words(`if then elif else fi for while until do done case esac function return in
			select time coproc set export local readonly declare source alias unset trap`),
		types: words(`echo cd ls cat grep sed awk rm cp mv mkdir test true false exit printf read
			pushd popd which command just go node python3`),
	},
	"sql": {
		lineComment: []string{"--"},
		quotes:      "'\"",
		keywords: words(`SELECT FROM WHERE INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE INDEX VIEW
			DROP ALTER ADD JOIN LEFT RIGHT INNER OUTER FULL ON GROUP BY ORDER HAVING LIMIT OFFSET UNION ALL
			AS AND OR NOT NULL IS IN LIKE BETWEEN DISTINCT COUNT SUM AVG MIN MAX CASE WHEN THEN ELSE END
			PRIMARY KEY FOREIGN REFERENCES UNIQUE DEFAULT WITH RETURNING`),
		types: words(`INTEGER INT TEXT VARCHAR CHAR BOOLEAN REAL FLOAT DOUBLE DECIMAL NUMERIC DATE
			TIMESTAMP BLOB SERIAL TRUE FALSE`),
	},
	"json": {
		quotes: "\"",
		types:  words(`true false null`),
	},
	"yaml": {
		lineComment: []string{"#"},
		quotes:      "\"'",
		types:       words(`true false null yes no on off`),
	},
	"toml": {
		lineComment: []string{"#"},
		quotes:      "\"'",
		types:       words(`true false`),
	},
}

// aliases keep the table short without making the caller normalise.
var aliases = map[string]string{
	"golang": "go", "javascript": "js", "typescript": "js", "ts": "js",
	"jsx": "js", "tsx": "js", "node": "js",
	"py": "python", "python3": "python",
	"bash": "sh", "zsh": "sh", "fish": "sh", "shell": "sh", "console": "sh",
	"postgres": "sql", "postgresql": "sql", "sqlite": "sql", "mysql": "sql",
	"yml": "yaml", "jsonc": "json",
}

// Highlight splits code into coloured runs. An unknown language, or an empty
// one, yields a single plain token.
func Highlight(lang, code string) []Token {
	spec := specFor(lang)
	if spec == nil || code == "" {
		if code == "" {
			return nil
		}
		return []Token{{Text: code}}
	}
	return (&scanner{src: []rune(code), spec: spec}).run()
}

// HighlightSupported reports whether a language will actually be coloured, so a
// caller can say so rather than silently doing nothing.
func HighlightSupported(lang string) bool { return specFor(lang) != nil }

func specFor(lang string) *langSpec {
	l := strings.ToLower(strings.TrimSpace(lang))
	if alias, ok := aliases[l]; ok {
		l = alias
	}
	return langs[l]
}

type scanner struct {
	src  []rune
	spec *langSpec
	pos  int
	out  []Token
	buf  []rune
}

// emit flushes the pending plain run, then appends a classified one.
func (s *scanner) emit(kind TokenKind, text string) {
	s.flush()
	if text != "" {
		s.out = append(s.out, Token{Kind: kind, Text: text})
	}
}

func (s *scanner) flush() {
	if len(s.buf) > 0 {
		s.out = append(s.out, Token{Text: string(s.buf)})
		s.buf = nil
	}
}

func (s *scanner) rest() string { return string(s.src[s.pos:]) }

func (s *scanner) run() []Token {
	for s.pos < len(s.src) {
		if s.comment() || s.str() || s.number() || s.word() {
			continue
		}
		s.buf = append(s.buf, s.src[s.pos])
		s.pos++
	}
	s.flush()
	return s.out
}

func (s *scanner) comment() bool {
	rest := s.rest()
	for _, prefix := range s.spec.lineComment {
		if strings.HasPrefix(rest, prefix) {
			end := strings.IndexByte(rest, '\n')
			if end < 0 {
				end = len(rest)
			}
			s.emit(TokComment, rest[:end])
			s.pos += len([]rune(rest[:end]))
			return true
		}
	}
	open, close := s.spec.blockComment[0], s.spec.blockComment[1]
	if open != "" && strings.HasPrefix(rest, open) {
		end := strings.Index(rest[len(open):], close)
		if end < 0 {
			s.emit(TokComment, rest)
			s.pos = len(s.src)
			return true
		}
		text := rest[:len(open)+end+len(close)]
		s.emit(TokComment, text)
		s.pos += len([]rune(text))
		return true
	}
	return false
}

func (s *scanner) str() bool {
	q := s.src[s.pos]
	if !strings.ContainsRune(s.spec.quotes, q) {
		return false
	}
	i := s.pos + 1
	for i < len(s.src) {
		if s.src[i] == '\\' && q != '`' && i+1 < len(s.src) {
			i += 2
			continue
		}
		if s.src[i] == q {
			i++
			break
		}
		// An unterminated quote should not swallow the rest of the snippet;
		// people paste half-written code into notes all the time.
		if s.src[i] == '\n' && q != '`' {
			break
		}
		i++
	}
	s.emit(TokString, string(s.src[s.pos:i]))
	s.pos = i
	return true
}

func (s *scanner) number() bool {
	c := s.src[s.pos]
	if c < '0' || c > '9' {
		return false
	}
	if s.pos > 0 && isWordRune(s.src[s.pos-1]) {
		return false // part of an identifier, not a number
	}
	i := s.pos
	for i < len(s.src) && (isDigitRune(s.src[i]) || s.src[i] == '.' || s.src[i] == 'x' ||
		(s.src[i] >= 'a' && s.src[i] <= 'f') || (s.src[i] >= 'A' && s.src[i] <= 'F') ||
		s.src[i] == '_') {
		i++
	}
	s.emit(TokNumber, string(s.src[s.pos:i]))
	s.pos = i
	return true
}

func (s *scanner) word() bool {
	if !isWordStart(s.src[s.pos]) {
		return false
	}
	i := s.pos
	for i < len(s.src) && isWordRune(s.src[i]) {
		i++
	}
	word := string(s.src[s.pos:i])
	kind := TokPlain
	switch {
	case s.spec.keywords[word]:
		kind = TokKeyword
	case s.spec.types[word]:
		kind = TokType
	case s.spec.keywords[strings.ToUpper(word)]:
		kind = TokKeyword // SQL is written both ways
	case s.spec.types[strings.ToUpper(word)]:
		kind = TokType
	}
	if kind == TokPlain {
		s.buf = append(s.buf, s.src[s.pos:i]...)
	} else {
		s.emit(kind, word)
	}
	s.pos = i
	return true
}

func isWordStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isWordRune(r rune) bool { return isWordStart(r) || isDigitRune(r) }

func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }
