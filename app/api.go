package main

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	tapp "github.com/tilman-schieber/tlog/internal/app"
	"github.com/tilman-schieber/tlog/internal/store"
)

// API is the bridge to the frontend. Every method is a thin call into the core:
// if something can be done here and not through app.Service, that is a bug.
type API struct {
	svc *tapp.Service
	ctx context.Context
}

// Edit is what a mutation returns: the page as it now stands, and where the
// block that was touched ended up, so the frontend can put the caret back.
type Edit struct {
	Page   *tapp.PageView `json:"page"`
	Offset int            `json:"offset"`
}

// Root is the notes directory, shown so it is never a mystery where the files
// actually are.
func (a *API) Root() string { return a.svc.Store.Root }

func (a *API) Today() (*tapp.PageView, error) { return a.svc.ViewToday() }

func (a *API) OpenRel(rel string) (*tapp.PageView, error) { return a.svc.View(rel) }

// OpenPage follows a link by name, creating the page the first time anyone
// goes there.
func (a *API) OpenPage(name string) (*tapp.PageView, error) { return a.svc.ViewPage(name) }

// Journal steps to another day relative to the given one. Browsing never
// creates a file; typing does.
func (a *API) Journal(rel string, delta int) (*tapp.PageView, error) {
	day, ok := store.JournalDay(rel)
	if !ok {
		day = time.Now()
	}
	return a.svc.View(a.svc.JournalRel(day.AddDate(0, 0, delta)))
}

func (a *API) Index() (*tapp.Index, error) { return a.svc.Index() }

func (a *API) Search(query string) ([]tapp.SearchHit, error) { return a.svc.Search(query, 200) }

func (a *API) CompletePages(prefix string) ([]string, error) {
	return a.svc.CompletePages(prefix, 8)
}

func (a *API) CompleteTags(prefix string) ([]string, error) {
	return a.svc.CompleteTags(prefix, 8)
}

func addr(rel string, offset int, hash string) tapp.Addr {
	return tapp.Addr{Rel: rel, Offset: offset, Hash: hash}
}

// after turns a mutation result into the page the frontend should now draw.
func (a *API) after(res *tapp.Result, err error) (*Edit, error) {
	if err != nil {
		return nil, err
	}
	page, err := a.svc.View(res.Rel)
	if err != nil {
		return nil, err
	}
	return &Edit{Page: page, Offset: res.Offset}, nil
}

func (a *API) SetText(rel string, offset int, hash, text string) (*Edit, error) {
	return a.after(a.svc.SetText(addr(rel, offset, hash), text))
}

func (a *API) NewBlock(rel string, offset int, hash, text string) (*Edit, error) {
	return a.after(a.svc.InsertAfter(addr(rel, offset, hash), text))
}

func (a *API) AppendBlock(rel, hash, text string) (*Edit, error) {
	return a.after(a.svc.AppendBlock(rel, hash, text))
}

func (a *API) DeleteBlock(rel string, offset int, hash string) (*Edit, error) {
	return a.after(a.svc.DeleteBlock(addr(rel, offset, hash)))
}

func (a *API) Indent(rel string, offset int, hash string) (*Edit, error) {
	return a.after(a.svc.Indent(addr(rel, offset, hash)))
}

func (a *API) Outdent(rel string, offset int, hash string) (*Edit, error) {
	return a.after(a.svc.Outdent(addr(rel, offset, hash)))
}

func (a *API) Move(rel string, offset int, hash string, delta int) (*Edit, error) {
	return a.after(a.svc.Move(addr(rel, offset, hash), delta))
}

func (a *API) ToggleTask(rel string, offset int, hash string) (*Edit, error) {
	return a.after(a.svc.ToggleTask(addr(rel, offset, hash)))
}

// onClose flushes the pending auto-commit, so quitting never leaves work
// uncommitted in the notes repository.
func (a *API) onClose(ctx context.Context) bool {
	_ = a.svc.Commit()
	return false
}

// OpenURL hands a link to the system, which opens a web address in the browser
// and a file:// one in the file manager — that last is how `att` writes
// attachment links into these notes.
//
// A note is just a file and a file could name any scheme it likes, so only the
// schemes a link in prose is actually meant to be are opened.
func (a *API) OpenURL(url string) error {
	u, err := neturl.Parse(url)
	if err != nil {
		return fmt.Errorf("not a link: %q", url)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto", "file":
		runtime.BrowserOpenURL(a.ctx, url)
		return nil
	default:
		return fmt.Errorf("refusing to open a %q link", u.Scheme)
	}
}
