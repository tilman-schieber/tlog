package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Logseq's DB version keeps the truth in SQLite and writes the markdown mirror
// as a lossy export: the mirror carries no page tags at all, so importing from
// it alone silently drops the whole typing structure — every person, project and
// topic you have classified.
//
// This file reads the classes back out of the database. The store is a single
// kvs table of transit-json documents holding datascript datoms, so the work is
// a transit reader plus a pass looking for :block/tags.
//
// It is Logseq's internal format and may change between versions. Nothing else
// depends on it: when it cannot be read, the import proceeds without tags and
// says so.

// classesFor returns page title -> the classes it is tagged with, read from a
// Logseq graph's database. A graph with no readable database yields no classes
// and no error, because importing from the mirror alone is still useful.
func classesFor(graphDir string) (map[string][]string, error) {
	db := findDB(graphDir)
	if db == "" {
		return nil, nil
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, fmt.Errorf("found %s but sqlite3 is not installed, so page tags cannot be read", db)
	}

	// Work on a copy: Logseq may be running, and its write-ahead log belongs to
	// it. Reading a snapshot cannot disturb a live editor.
	tmp, err := os.MkdirTemp("", "tlog-logseqdb-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	local := filepath.Join(tmp, "db.sqlite")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = copyFile(db+suffix, local+suffix)
	}

	out, err := exec.Command("sqlite3", local, "select content from kvs").Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", db, err)
	}
	return parseDatoms(out), nil
}

func findDB(graphDir string) string {
	for _, c := range []string{
		filepath.Join(graphDir, "db.sqlite"),
		// The caller may have been pointed at the markdown mirror inside a graph.
		filepath.Join(graphDir, "..", "..", "db.sqlite"),
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// entity is what one pass over the datoms collects about a datascript entity.
type entity struct {
	title  string
	name   string // present on pages, absent on ordinary blocks
	ident  string // :db/ident, set on Logseq's own built-in classes
	tags   []int64
	isPage bool
}

// parseDatoms decodes the transit documents and returns page title -> classes.
func parseDatoms(out []byte) map[string][]string {
	ents := map[int64]*entity{}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var doc any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			continue
		}
		resolved := resolveCache(doc, &[]string{}, false)
		collectDatoms(resolved, ents)
	}

	// A class is only worth carrying over if a user made it. Logseq's own
	// Page/Journal/Property/Task classes describe machinery, not meaning.
	classTitle := func(id int64) string {
		e := ents[id]
		if e == nil || e.title == "" || strings.HasPrefix(e.ident, "~:logseq.class/") {
			return ""
		}
		return e.title
	}

	res := map[string][]string{}
	for _, e := range ents {
		if !e.isPage || e.title == "" {
			continue
		}
		var tags []string
		seen := map[string]bool{}
		for _, t := range e.tags {
			name := classTitle(t)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			tags = append(tags, name)
		}
		if len(tags) > 0 {
			res[e.title] = tags
		}
	}
	return res
}

func ent(ents map[int64]*entity, id int64) *entity {
	e := ents[id]
	if e == nil {
		e = &entity{}
		ents[id] = e
	}
	return e
}

// collectDatoms finds [entity, attribute, value, tx] vectors anywhere in a
// decoded document and records the three attributes that matter.
func collectDatoms(node any, ents map[int64]*entity) {
	list, ok := node.([]any)
	if !ok {
		return
	}
	if len(list) == 4 {
		if e, ok := asInt(list[0]); ok {
			if attr, ok := list[1].(string); ok {
				if _, ok := asInt(list[3]); ok {
					switch attr {
					case "~:block/title":
						if v, ok := list[2].(string); ok {
							if x := ent(ents, e); x.title == "" {
								x.title = v
							}
						}
					case "~:block/name":
						if v, ok := list[2].(string); ok {
							x := ent(ents, e)
							x.name, x.isPage = v, true
						}
					case "~:db/ident":
						if v, ok := list[2].(string); ok {
							ent(ents, e).ident = v
						}
					case "~:block/tags":
						if v, ok := asInt(list[2]); ok {
							x := ent(ents, e)
							x.tags = append(x.tags, v)
						}
					}
					return
				}
			}
		}
	}
	for _, v := range list {
		collectDatoms(v, ents)
	}
}

func asInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

// resolveCache expands transit's write cache. A cacheable string is added to
// the cache the first time it appears, in document order, and "^N" refers back
// to one; without replaying that exactly, attribute names are unreadable.
func resolveCache(node any, cache *[]string, asKey bool) any {
	switch v := node.(type) {
	case string:
		if v != mapMarker && strings.HasPrefix(v, "^") {
			if i, ok := cacheIndex(v); ok && i < len(*cache) {
				return (*cache)[i]
			}
			return v
		}
		if isCacheable(v, asKey) {
			*cache = append(*cache, v)
		}
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok && s == mapMarker {
				out := make([]any, 0, len(v))
				out = append(out, s)
				for i, x := range v[1:] {
					out = append(out, resolveCache(x, cache, i%2 == 0))
				}
				return out
			}
		}
		out := make([]any, 0, len(v))
		for _, x := range v {
			out = append(out, resolveCache(x, cache, false))
		}
		return out
	default:
		return node
	}
}

const mapMarker = "^ "

// isCacheable mirrors transit's rule: map keys, and tagged or keyword-like
// values, once they are long enough to be worth caching.
func isCacheable(s string, asKey bool) bool {
	if len(s) < 4 {
		return false
	}
	if asKey {
		return true
	}
	if s[0] != '~' {
		return false
	}
	return s[1] == ':' || s[1] == '$' || s[1] == '#'
}

// cacheIndex decodes "^N", whose digits are base 44 starting at '0'.
func cacheIndex(s string) (int, bool) {
	switch len(s) {
	case 2:
		return int(s[1]) - 48, true
	case 3:
		return (int(s[1])-48)*44 + (int(s[2]) - 48), true
	}
	return 0, false
}
