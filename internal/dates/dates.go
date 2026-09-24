// Package dates turns what a person types into a day. It is deliberately
// forgiving on input and strict on output: every shorthand here resolves to an
// ISO date, because that is what goes in the file and what has to sort, grep
// and mean the same thing in a year.
//
// German and English are both accepted, because these notes are both.
package dates

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Layout is the only form ever written to a file.
const Layout = "2006-01-02"

// Format renders a day for storage.
func Format(t time.Time) string { return t.Format(Layout) }

// weekdays maps every abbreviation a person might reasonably type.
var weekdays = map[string]time.Weekday{
	"mo": time.Monday, "mon": time.Monday, "montag": time.Monday, "monday": time.Monday,
	"di": time.Tuesday, "die": time.Tuesday, "tue": time.Tuesday, "dienstag": time.Tuesday, "tuesday": time.Tuesday,
	"mi": time.Wednesday, "mit": time.Wednesday, "wed": time.Wednesday, "mittwoch": time.Wednesday, "wednesday": time.Wednesday,
	"do": time.Thursday, "don": time.Thursday, "thu": time.Thursday, "donnerstag": time.Thursday, "thursday": time.Thursday,
	"fr": time.Friday, "fre": time.Friday, "fri": time.Friday, "freitag": time.Friday, "friday": time.Friday,
	"sa": time.Saturday, "sam": time.Saturday, "sat": time.Saturday, "samstag": time.Saturday, "saturday": time.Saturday,
	"so": time.Sunday, "son": time.Sunday, "sun": time.Sunday, "sonntag": time.Sunday, "sunday": time.Sunday,
}

var (
	isoRe = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)
	// The German form, with the year optional: 24.9. / 24.09. / 24.9.26 / 24.9.2026
	deRe = regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.?(\d{2}|\d{4})?\.?$`)
	// An offset: +3d, +2w, 3t, +1m, +1j
	offRe = regexp.MustCompile(`^\+?(\d{1,3})\s*([dtwmjy])$`)
	// A bare day of the month: 24. or 24
	domRe = regexp.MustCompile(`^(\d{1,2})\.?$`)
)

// Parse resolves a typed shorthand to a day, relative to now. The second result
// is false when nothing sensible could be made of it, which is the caller's cue
// to leave the typed text alone rather than guess.
func Parse(input string, now time.Time) (time.Time, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return time.Time{}, false
	}
	today := day(now)

	switch s {
	case "heute", "today", "h":
		return today, true
	case "morgen", "tomorrow", "m":
		return today.AddDate(0, 0, 1), true
	case "übermorgen", "uebermorgen", "overmorrow":
		return today.AddDate(0, 0, 2), true
	case "gestern", "yesterday":
		return today.AddDate(0, 0, -1), true
	case "eow", "ende der woche", "wochenende":
		// Friday, because a week ends when the work does.
		return next(today, time.Friday), true
	case "eom", "monatsende":
		return today.AddDate(0, 1, -today.Day()+1).AddDate(0, 0, -1), true
	case "eoy", "jahresende":
		return time.Date(today.Year(), 12, 31, 0, 0, 0, 0, today.Location()), true
	}

	if wd, ok := weekdays[s]; ok {
		return next(today, wd), true
	}
	// "nächsten freitag" is genuinely ambiguous in speech, so the rule here is
	// stated rather than guessed: it is always one week after plain "freitag".
	// Anyone who means something else can type the date.
	for _, prefix := range []string{"nächsten ", "naechsten ", "nächste ", "next ", "kommenden "} {
		if rest, found := strings.CutPrefix(s, prefix); found {
			if wd, ok := weekdays[strings.TrimSpace(rest)]; ok {
				return next(today, wd).AddDate(0, 0, 7), true
			}
		}
	}

	if m := isoRe.FindStringSubmatch(s); m != nil {
		return build(atoi(m[1]), atoi(m[2]), atoi(m[3]), today)
	}

	if m := deRe.FindStringSubmatch(s); m != nil {
		year := today.Year()
		if m[3] != "" {
			year = atoi(m[3])
			if year < 100 {
				year += 2000
			}
		}
		d, ok := build(year, atoi(m[2]), atoi(m[1]), today)
		if !ok {
			return d, false
		}
		// A bare day and month that has already gone by means next year, which
		// is almost always what someone typing "3.1." in December means.
		if m[3] == "" && d.Before(today) {
			d = d.AddDate(1, 0, 0)
		}
		return d, true
	}

	if m := offRe.FindStringSubmatch(s); m != nil {
		n := atoi(m[1])
		switch m[2] {
		case "d", "t": // days, Tage
			return today.AddDate(0, 0, n), true
		case "w":
			return today.AddDate(0, 0, 7*n), true
		case "m":
			return today.AddDate(0, n, 0), true
		case "j", "y": // Jahre, years
			return today.AddDate(n, 0, 0), true
		}
	}

	if m := domRe.FindStringSubmatch(s); m != nil {
		d, ok := build(today.Year(), int(today.Month()), atoi(m[1]), today)
		if !ok {
			return d, false
		}
		if d.Before(today) {
			d = d.AddDate(0, 1, 0)
		}
		return d, true
	}

	return time.Time{}, false
}

// build validates a calendar date, rejecting the likes of 31 February rather
// than rolling it into March.
func build(y, mo, d int, ref time.Time) (time.Time, bool) {
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, ref.Location())
	if int(t.Month()) != mo || t.Day() != d {
		return time.Time{}, false
	}
	return t, true
}

// next is the coming occurrence of a weekday, counting today as itself: typing
// "fr" on a Friday means today, which is what every other tool does.
func next(today time.Time, wd time.Weekday) time.Time {
	delta := (int(wd) - int(today.Weekday()) + 7) % 7
	return today.AddDate(0, 0, delta)
}

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// State is how a deadline stands relative to today.
type State string

const (
	Overdue State = "overdue"
	Today   State = "today"
	Soon    State = "soon" // within a week
	Later   State = "later"
)

// Status classifies a day, for colouring a task without every adapter
// re-deciding what "soon" means.
func Status(t, now time.Time) State {
	d := days(day(now), day(t))
	switch {
	case d < 0:
		return Overdue
	case d == 0:
		return Today
	case d <= 7:
		return Soon
	default:
		return Later
	}
}

// Describe says how far away a day is, in German, for showing beside a date.
func Describe(t, now time.Time) string {
	d := days(day(now), day(t))
	switch {
	case d == 0:
		return "heute"
	case d == 1:
		return "morgen"
	case d == -1:
		return "gestern"
	case d == 2:
		return "übermorgen"
	case d < 0:
		return fmt.Sprintf("%d Tage überfällig", -d)
	case d < 7:
		return fmt.Sprintf("in %d Tagen", d)
	case d < 14:
		return "nächste Woche"
	case d < 62:
		return fmt.Sprintf("in %d Wochen", (d+3)/7)
	default:
		return fmt.Sprintf("in %d Monaten", d/30)
	}
}

func days(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

// Weekday renders the two-letter German abbreviation, for a calendar header or
// a compact date.
func Weekday(t time.Time) string {
	return [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[int(t.Weekday())]
}

// Short renders a date the way it would be written by hand: "Fr 26.09."
func Short(t time.Time) string {
	return fmt.Sprintf("%s %02d.%02d.", Weekday(t), t.Day(), int(t.Month()))
}
