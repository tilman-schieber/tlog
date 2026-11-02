package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRecentSurvivesARestart(t *testing.T) {
	t.Setenv("TLOG_STATE", filepath.Join(t.TempDir(), "recent.json"))
	root := t.TempDir()

	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, n := range []string{"A", "B", "C", "D", "E", "F"} {
		rel, err := s.OpenPage(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Visited(rel); err != nil {
			t.Fatal(err)
		}
		names = append([]string{rel}, names...)
	}

	again, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]string{}, names[:RecentPages]...), again.TodayRel(), JournalsRel)
	if got := again.Recent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRecentIsPerNotesDirectory(t *testing.T) {
	t.Setenv("TLOG_STATE", filepath.Join(t.TempDir(), "recent.json"))
	one, _ := New(t.TempDir())
	two, _ := New(t.TempDir())

	rel, _ := one.OpenPage("Only here")
	if _, err := one.Visited(rel); err != nil {
		t.Fatal(err)
	}
	if got := two.Recent(); !reflect.DeepEqual(got, []string{two.TodayRel(), JournalsRel}) {
		t.Fatalf("another directory's pages leaked in: %v", got)
	}
}

func TestRecentForgetsDeletedPagesButKeepsJournals(t *testing.T) {
	t.Setenv("TLOG_STATE", filepath.Join(t.TempDir(), "recent.json"))
	s, _ := New(t.TempDir())

	gone, _ := s.OpenPage("Gone")
	day := s.TodayRel() // never written: opening it is how it gets a file
	s.Visited(gone)
	s.Visited(day)
	if err := os.Remove(s.Store.Abs(gone)); err != nil {
		t.Fatal(err)
	}
	if got := s.Recent(); !reflect.DeepEqual(got, []string{day, JournalsRel}) {
		t.Fatalf("got %v", got)
	}
}

func TestABrokenStateFileIsAnEmptyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	t.Setenv("TLOG_STATE", path)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := New(t.TempDir())
	if got := s.Recent(); !reflect.DeepEqual(got, []string{s.TodayRel(), JournalsRel}) {
		t.Fatalf("got %v", got)
	}
	rel, _ := s.OpenPage("A")
	if _, err := s.Visited(rel); err != nil {
		t.Fatalf("a broken file should be replaced, not refused: %v", err)
	}
	if got := s.Recent(); !reflect.DeepEqual(got, []string{rel, s.TodayRel(), JournalsRel}) {
		t.Fatalf("got %v", got)
	}
}

func TestJournalDaysDoNotCrowdOutPages(t *testing.T) {
	t.Setenv("TLOG_STATE", filepath.Join(t.TempDir(), "recent.json"))
	s, _ := New(t.TempDir())
	var pages []string
	for _, n := range []string{"A", "B", "C", "D"} {
		rel, _ := s.OpenPage(n)
		s.Visited(rel)
		pages = append([]string{rel}, pages...)
	}
	s.Visited(JournalsRel)
	// A week of journal days: only the last one stays.
	var last string
	for i := 7; i >= 1; i-- {
		last = s.JournalRel(time.Now().AddDate(0, 0, -i))
		s.Visited(last)
	}
	want := append([]string{last, JournalsRel}, pages...)
	if got := s.Recent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}
