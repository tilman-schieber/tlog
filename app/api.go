package main

import (
	"context"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/tilman-schieber/tlog/internal/dates"

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
	Caret  int            `json:"caret,omitempty"` // where a command left the caret
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

// Sync reports whether the notes are reaching their remote, so a push that
// failed in the background is shown rather than silently forgotten.
func (a *API) Sync() tapp.SyncStatus { return a.svc.Sync() }

// Push sends the notes now.
func (a *API) Push() error {
	if err := a.svc.Commit(); err != nil {
		return err
	}
	return a.svc.Push()
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

// --- slash commands ---------------------------------------------------------

// Commands lists the slash menu. The list lives in the core so the app and the
// outliner cannot offer different things.
func (a *API) Commands(prefix string) []tapp.Command { return tapp.MatchCommands(prefix) }

// DatePreview resolves a typed date shorthand for showing in the menu, before
// anything is written. It answers with an empty string while the text is not
// yet a date, which is the normal state halfway through typing one.
type DatePreview struct {
	ISO   string `json:"iso"`
	Short string `json:"short"`
	Label string `json:"label"`
	State string `json:"state"`
}

func (a *API) DatePreview(arg string) DatePreview {
	d, ok := tapp.PreviewDate(arg)
	if !ok {
		return DatePreview{}
	}
	now := time.Now()
	return DatePreview{
		ISO:   dates.Format(d),
		Short: dates.Short(d),
		Label: dates.Describe(d, now),
		State: string(dates.Status(d, now)),
	}
}

// Calendar is a month laid out for drawing, Monday first.
type Calendar struct {
	Title string   `json:"title"`
	Days  []CalDay `json:"days"`
}

// CalDay is one cell. Blank cells pad the start of the month.
type CalDay struct {
	Day   int    `json:"day"`
	ISO   string `json:"iso,omitempty"`
	Today bool   `json:"today"`
	Sel   bool   `json:"sel"`
}

// Month builds the calendar around a date, so that a typed shorthand can be
// confirmed at a glance rather than trusted.
func (a *API) Month(iso string) Calendar {
	now := time.Now()
	sel, err := time.Parse(dates.Layout, iso)
	if err != nil {
		sel = now
	}
	first := time.Date(sel.Year(), sel.Month(), 1, 0, 0, 0, 0, sel.Location())
	lead := (int(first.Weekday()) + 6) % 7 // Monday first

	cal := Calendar{Title: fmt.Sprintf("%s %d", monthNames[first.Month()], first.Year())}
	for i := 0; i < lead; i++ {
		cal.Days = append(cal.Days, CalDay{})
	}
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		cal.Days = append(cal.Days, CalDay{
			Day:   d.Day(),
			ISO:   dates.Format(d),
			Today: dates.Format(d) == dates.Format(now),
			Sel:   dates.Format(d) == dates.Format(sel),
		})
	}
	return cal
}

var monthNames = map[time.Month]string{
	time.January: "Januar", time.February: "Februar", time.March: "März",
	time.April: "April", time.May: "Mai", time.June: "Juni",
	time.July: "Juli", time.August: "August", time.September: "September",
	time.October: "Oktober", time.November: "November", time.December: "Dezember",
}

// RunCommand applies a slash command to a block and cuts the command out of the
// text, in one write. from and to are rune offsets of the "/command argument"
// run, and the core does the cutting so both adapters cut identically.
func (a *API) RunCommand(rel string, offset int, hash, name, arg, text string, from, to int) (*Edit, error) {
	res, err := a.svc.RunCommand(addr(rel, offset, hash), name, arg, text, from, to)
	if err != nil {
		return nil, err
	}
	edit, err := a.after(res.Result, nil)
	if err != nil {
		return nil, err
	}
	edit.Caret = res.Caret
	return edit, nil
}

// Due lists everything dated and still open, soonest first.
func (a *API) Due(includeDone bool) ([]tapp.DueItem, error) { return a.svc.Due(includeDone) }

// --- attachments ------------------------------------------------------------

// AttachDir is the shared shelf, shown so it is never a mystery where a
// dropped file went.
func (a *API) AttachDir() string { return a.svc.AttachDir() }

// Attachments lists the shelf, narrowed by a query.
func (a *API) Attachments(query string) ([]tapp.AttachView, error) {
	return a.svc.Attachments(query, 12)
}

// AttachFiles puts files on the shelf and writes their links into a page as one
// block — what a drag and drop onto the window means.
func (a *API) AttachFiles(rel string, paths []string) (*Edit, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("nothing to attach")
	}
	if _, err := a.svc.AttachTo(rel, paths...); err != nil {
		return nil, err
	}
	page, err := a.svc.View(rel)
	if err != nil {
		return nil, err
	}
	return &Edit{Page: page}, nil
}

// ChooseFiles opens the system file dialog, for when there is nothing to drag.
func (a *API) ChooseFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Anhängen",
	})
}
