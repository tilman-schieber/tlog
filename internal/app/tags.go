package app

import (
	"strings"

	"github.com/tilman-schieber/tlog/internal/markdown"
	"github.com/tilman-schieber/tlog/internal/store"
)

// A tag says what a page is. It is a page itself — #repo and [[repo]] name the
// same thing — and a page carries it in its frontmatter, `tags: person, repo`,
// or by being linked as [[Page #repo]] anywhere. These are the operations that
// put a tag on a page and take it off again, so a script can do what a person
// does by typing frontmatter.

// TaggedPage is one page that carries a tag.
type TaggedPage struct {
	Name   string   `json:"name"`
	Rel    string   `json:"rel"`
	Exists bool     `json:"exists"` // a page only ever linked has no file yet
	Tags   []string `json:"tags"`
}

// Tagged lists every page that carries a tag, alphabetically.
func (s *Service) Tagged(tag string) ([]TaggedPage, error) {
	tag, err := cleanTag(tag)
	if err != nil {
		return nil, err
	}
	l, err := s.Lookup()
	if err != nil {
		return nil, err
	}
	out := []TaggedPage{}
	for _, name := range l.Tagged(tag) {
		t := TaggedPage{Name: name, Tags: l.g.PageTags(name)}
		if p, ok := l.g.Page(name); ok {
			t.Rel, t.Exists = p.Rel, true
		} else if rel, err := s.Store.PageRel(name); err == nil {
			t.Rel = rel
		}
		out = append(out, t)
	}
	return out, nil
}

// Tag puts a tag in a page's frontmatter, after the ones already there. A page
// that has never been written gets a file holding just that, because saying
// what a page is is a reason to have one. Tagging twice changes nothing.
func (s *Service) Tag(rel, tag string) error {
	tag, err := cleanTag(tag)
	if err != nil {
		return err
	}
	d, err := s.Load(rel)
	if err != nil {
		return err
	}
	key, tags := pageTags(d.Doc)
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return nil
		}
	}
	d.Doc.SetFrontmatter(key, strings.Join(append(tags, tag), ", "))
	return s.Save(d)
}

// Untag takes a tag out of a page's frontmatter, and the tags: line with it
// when it was the last one. A tag asserted by a [[Page #tag]] link elsewhere is
// said in that note, not on the page, and is not touched: refused rather than
// pretended away.
func (s *Service) Untag(rel, tag string) error {
	tag, err := cleanTag(tag)
	if err != nil {
		return err
	}
	d, err := s.Load(rel)
	if err != nil {
		return err
	}
	key, tags := pageTags(d.Doc)
	kept := tags[:0:0]
	for _, t := range tags {
		if !strings.EqualFold(t, tag) {
			kept = append(kept, t)
		}
	}
	if len(kept) == len(tags) {
		return refuse("%s does not carry %q in its frontmatter — a [[Page #%s]] link elsewhere says it there", store.Title(rel), tag, tag)
	}
	if len(kept) == 0 {
		fm := d.Doc.Frontmatter[:0:0]
		for _, p := range d.Doc.Frontmatter {
			if p.Key != key {
				fm = append(fm, p)
			}
		}
		d.Doc.Frontmatter = fm
	} else {
		d.Doc.SetFrontmatter(key, strings.Join(kept, ", "))
	}
	return s.Save(d)
}

// pageTags reads the frontmatter's tags, keeping the key as it is spelled.
func pageTags(d *markdown.Document) (key string, tags []string) {
	key = "tags"
	for _, p := range d.Frontmatter {
		if strings.EqualFold(p.Key, "tags") {
			key = p.Key
			for _, t := range strings.FieldsFunc(p.Value, func(r rune) bool { return r == ',' || r == ';' }) {
				if t = strings.TrimPrefix(strings.TrimSpace(t), "#"); t != "" {
					tags = append(tags, t)
				}
			}
		}
	}
	return key, tags
}

// cleanTag takes "#repo" or "repo" and refuses what cannot be a tag in a
// frontmatter list.
func cleanTag(tag string) (string, error) {
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
	if tag == "" {
		return "", refuse("which tag?")
	}
	if strings.ContainsAny(tag, ",;\n[]") {
		return "", refuse("%q cannot be a tag: no commas, semicolons, brackets or line breaks", tag)
	}
	return tag, nil
}
