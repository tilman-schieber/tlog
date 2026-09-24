package dates

import (
	"testing"
	"time"
)

// A Thursday, so that "friday" is tomorrow and "wednesday" is nearly a week
// away — the two cases people get wrong.
var now = time.Date(2026, 9, 24, 15, 30, 0, 0, time.UTC)

func parse(t *testing.T, in string) string {
	t.Helper()
	got, ok := Parse(in, now)
	if !ok {
		t.Fatalf("%q was not understood", in)
	}
	return Format(got)
}

func TestTypedShorthand(t *testing.T) {
	cases := map[string]string{
		// relative
		"heute": "2026-09-24", "today": "2026-09-24", "h": "2026-09-24",
		"morgen": "2026-09-25", "tomorrow": "2026-09-25", "m": "2026-09-25",
		"übermorgen": "2026-09-26",
		"gestern":    "2026-09-23",

		// weekdays: the coming one, counting today as itself
		"fr": "2026-09-25", "freitag": "2026-09-25", "friday": "2026-09-25",
		"do":               "2026-09-24", // today is Thursday
		"mi":               "2026-09-30", // nearly a week away
		"mo":               "2026-09-28",
		"so":               "2026-09-27",
		"nächsten freitag": "2026-10-02",
		"next thursday":    "2026-10-01",

		// the German form people actually type
		"26.9.":      "2026-09-26",
		"26.09.":     "2026-09-26",
		"26.9":       "2026-09-26",
		"26.9.27":    "2027-09-26",
		"26.09.2027": "2027-09-26",

		// ISO
		"2026-10-11": "2026-10-11",
		"2026-1-5":   "2026-01-05",

		// offsets
		"+3d": "2026-09-27", "3d": "2026-09-27", "+3t": "2026-09-27",
		"+2w": "2026-10-08",
		"+1m": "2026-10-24",
		"+1j": "2027-09-24", "+1y": "2027-09-24",

		// ends of things
		"eow": "2026-09-25", // Friday, because a week ends when the work does
		"eom": "2026-09-30",
		"eoy": "2026-12-31",

		// a bare day of the month
		"30": "2026-09-30",
		"2.": "2026-10-02", // already gone this month, so next
	}
	for in, want := range cases {
		if got := parse(t, in); got != want {
			t.Errorf("%q → %s, want %s", in, got, want)
		}
	}
}

func TestADayAndMonthAlreadyGoneMeansNextYear(t *testing.T) {
	// Typing "3.1." in September means January, not eight months ago.
	if got := parse(t, "3.1."); got != "2027-01-03" {
		t.Fatalf("got %s", got)
	}
	// But an explicit year is believed.
	if got := parse(t, "3.1.26"); got != "2026-01-03" {
		t.Fatalf("got %s", got)
	}
}

func TestNonsenseIsRefusedRatherThanGuessed(t *testing.T) {
	for _, in := range []string{
		"", "   ", "banana", "32.1.", "1.13.", "2026-02-30", "29.2.26", "++", "d3",
	} {
		if got, ok := Parse(in, now); ok {
			t.Errorf("%q should not have parsed, got %s", in, Format(got))
		}
	}
	// A leap day in a leap year is fine.
	if _, ok := Parse("29.2.28", now); !ok {
		t.Error("29.2.28 is a real date")
	}
}

func TestCaseAndSpacingDoNotMatter(t *testing.T) {
	for _, in := range []string{"FR", "  Freitag  ", "Friday", "fReItAg"} {
		if got := parse(t, in); got != "2026-09-25" {
			t.Errorf("%q → %s", in, got)
		}
	}
}

func TestStatus(t *testing.T) {
	cases := map[string]State{
		"2026-09-20": Overdue,
		"2026-09-24": Today,
		"2026-09-25": Soon,
		"2026-10-01": Soon,
		"2026-10-11": Later,
	}
	for in, want := range cases {
		d, _ := Parse(in, now)
		if got := Status(d, now); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestDescribe(t *testing.T) {
	cases := map[string]string{
		"2026-09-24": "heute",
		"2026-09-25": "morgen",
		"2026-09-26": "übermorgen",
		"2026-09-23": "gestern",
		"2026-09-21": "3 Tage überfällig",
		"2026-09-28": "in 4 Tagen",
		"2026-10-02": "nächste Woche",
	}
	for in, want := range cases {
		d, _ := Parse(in, now)
		if got := Describe(d, now); got != want {
			t.Errorf("%s → %q, want %q", in, got, want)
		}
	}
}

func TestShortAndWeekday(t *testing.T) {
	d, _ := Parse("26.9.", now)
	if got := Short(d); got != "Sa 26.09." {
		t.Fatalf("got %q", got)
	}
	if got := Weekday(d); got != "Sa" {
		t.Fatalf("got %q", got)
	}
}

// Parsing must not depend on the time of day, or a deadline set at 23:50 would
// land on a different date than the same words at 00:10.
func TestTimeOfDayDoesNotMatter(t *testing.T) {
	for _, h := range []int{0, 9, 23} {
		at := time.Date(2026, 9, 24, h, 55, 0, 0, time.UTC)
		got, ok := Parse("morgen", at)
		if !ok || Format(got) != "2026-09-25" {
			t.Fatalf("at %02d:55 → %s", h, Format(got))
		}
	}
}
