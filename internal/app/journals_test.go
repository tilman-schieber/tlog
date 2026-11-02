package app

import (
	"reflect"
	"testing"
	"time"
)

func TestJournalDaysAreNewestFirstWithTodayAlways(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	older := s.JournalRel(now.AddDate(0, 0, -3))
	oldest := s.JournalRel(now.AddDate(0, 0, -10))
	ahead := s.JournalRel(now.AddDate(0, 0, 2))
	for _, rel := range []string{oldest, ahead, older} {
		if _, err := s.Add(rel, "note"); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.JournalDays()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ahead, s.TodayRel(), older, oldest}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestOpenDaysAddressesEachBlockInItsOwnFile(t *testing.T) {
	s, _ := New(t.TempDir())
	a := s.JournalRel(time.Now().AddDate(0, 0, -1))
	b := s.JournalRel(time.Now().AddDate(0, 0, -2))
	s.Add(a, "first day")
	s.Add(b, "second day")

	days, err := s.OpenDays([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	// Editing a block through the day it came from changes that file only.
	d := days[1].Doc
	blk := d.Doc.Blocks[0]
	if _, err := s.SetText(Addr{Rel: d.Rel, Offset: blk.Start, Hash: d.Hash}, "changed"); err != nil {
		t.Fatal(err)
	}
	da, _ := s.Load(a)
	db, _ := s.Load(b)
	if da.Doc.Blocks[0].Text != "first day" || db.Doc.Blocks[0].Text != "changed" {
		t.Fatalf("a=%q b=%q", da.Doc.Blocks[0].Text, db.Doc.Blocks[0].Text)
	}
}
