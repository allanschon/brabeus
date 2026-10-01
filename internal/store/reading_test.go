package store

import (
	"testing"
	"time"
)

func TestReadDerivesStateFromDeadlineAndSize(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		layout := "2006-01-02 15:04"
		if len(s) == 10 {
			layout = "2006-01-02"
		}
		tm, err := time.ParseInLocation(layout, s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	n := func(i int) *int { return &i }
	tracker := func(since string) Claim {
		return Claim{Text: "t", Adapter: AdapterTracker, Args: map[string]string{"min": "6", "since": since}}
	}
	manual := func(c Claim) Claim { c.Adapter = AdapterManual; return c }
	res := func(s ClaimState, count *int) ClaimResult { return ClaimResult{State: s, Count: count} }

	cases := []struct {
		name     string
		claim    Claim
		result   ClaimResult
		goalBy   string
		now      string
		want     ClaimState
		unread   string
		measured ClaimState
		daysLeft *int
		expected *int
		deadline string
		standing bool
	}{
		{name: "1 first day, nothing expected", claim: tracker("2026-09-28"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-09-29 12:00", want: Open, expected: n(0)},
		// Row 2: elapsed 4 of 13 expects 1; the half rounds down to 0, so
		// nothing is behind until two items are expected.
		{name: "2 expected one, half zero", claim: tracker("2026-09-28"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-10-02 12:00", want: Open, expected: n(1)},
		{name: "3 expected two, half one, count zero", claim: tracker("2026-09-28"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-10-04 12:00", want: Behind, expected: n(2)},
		{name: "4 count three is on pace", claim: tracker("2026-09-28"), result: res(Fail, n(3)), goalBy: "2026-10-10", now: "2026-10-04 12:00", want: Open},
		{name: "5 deadline day is still open", claim: tracker("2026-09-28"), result: res(Fail, n(5)), goalBy: "2026-10-10", now: "2026-10-10 23:30", want: Open, daysLeft: n(1)},
		{name: "6 day after deadline fails", claim: tracker("2026-09-28"), result: res(Fail, n(5)), goalBy: "2026-10-10", now: "2026-10-11 00:30", want: Fail},
		{name: "7 pass stays pass", claim: tracker("2026-09-28"), result: res(Pass, n(6)), goalBy: "2026-10-10", now: "2026-10-11", want: Pass},
		{name: "8 no evidence stays", claim: tracker("2026-09-28"), result: res(NoEvidence, nil), goalBy: "2026-10-10", now: "2026-10-04", want: NoEvidence},
		{name: "9 old result without count reads as yes-or-no", claim: tracker("2026-09-28"), result: res(Fail, nil), goalBy: "2026-10-10", now: "2026-10-09", want: Open},
		{name: "10 effort fits", claim: Claim{Adapter: AdapterManual, Effort: "7d"}, result: res(Unchecked, nil), goalBy: "2026-11-08", now: "2026-10-31 12:00", want: Open, daysLeft: n(9)},
		{name: "11 effort does not fit", claim: Claim{Adapter: AdapterManual, Effort: "7d"}, result: res(Unchecked, nil), goalBy: "2026-11-08", now: "2026-11-02 12:00", want: Behind, daysLeft: n(7)},
		{name: "12 no effort, deadline day", claim: Claim{Adapter: AdapterManual}, result: res(Fail, nil), goalBy: "2026-11-08", now: "2026-11-08 12:00", want: Open},
		{name: "13 never answered, past deadline", claim: Claim{Adapter: AdapterManual}, result: res(Unchecked, nil), goalBy: "2026-11-08", now: "2026-11-09", want: Fail},
		{name: "14 standing unchecked", claim: Claim{Adapter: AdapterManual, Standing: true}, result: res(Unchecked, nil), goalBy: "2026-11-08", now: "2026-12-01", want: Unchecked, standing: true},
		{name: "15 standing fail", claim: Claim{Adapter: AdapterManual, Standing: true}, result: res(Fail, nil), goalBy: "2026-11-08", now: "2026-10-01", want: Fail, standing: true},
		{name: "16 date claim is standing", claim: Claim{Adapter: AdapterDate, Args: map[string]string{"before": "2026-12-01"}}, result: res(Pass, nil), goalBy: "2026-11-08", now: "2026-10-01", want: Pass, standing: true},
		{name: "17 the claim's own deadline", claim: Claim{Adapter: AdapterManual, By: "2026-10-05"}, result: res(Fail, nil), goalBy: "2026-11-08", now: "2026-10-06", want: Fail, deadline: "2026-10-05"},
		{name: "18 unreadable deadline", claim: Claim{Adapter: AdapterManual}, result: res(Fail, nil), goalBy: "October", now: "2026-10-01", want: Behind, unread: "deadline"},
		{name: "19 manual of 1200", claim: manual(Claim{Args: map[string]string{"of": "1200", "since": "2026-09-01"}}), result: res(Fail, n(100)), goalBy: "2026-12-31", now: "2026-10-31", want: Behind, expected: n(590)},
		{name: "20 since after deadline", claim: tracker("2026-10-20"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-10-04", want: Behind, unread: "window"},
		{name: "21 today before since", claim: tracker("2026-10-06"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-10-04", want: Open, expected: n(0)},
		{name: "22 yes-or-no tracker without effort", claim: Claim{Adapter: AdapterTracker, Args: map[string]string{"done": "false", "min": "3", "since": "2026-09-28"}}, result: res(Fail, n(1)), goalBy: "2026-10-10", now: "2026-10-09", want: Open},
		{name: "23 manual count never measured, behind", claim: manual(Claim{Args: map[string]string{"of": "1200", "since": "2026-09-01"}}), result: res(Unchecked, nil), goalBy: "2026-12-31", now: "2026-10-31", want: Behind, expected: n(590)},
		{name: "24 manual count never measured, first day, nothing due", claim: manual(Claim{Args: map[string]string{"of": "1200", "since": "2026-09-01"}}), result: res(Unchecked, nil), goalBy: "2026-12-31", now: "2026-09-01", want: Open, expected: n(0)},
		{name: "30 manual count never measured, half an item due", claim: manual(Claim{Args: map[string]string{"of": "1200", "since": "2026-09-01"}}), result: res(Unchecked, nil), goalBy: "2026-12-31", now: "2026-09-02", want: Behind, expected: n(9)},
		{name: "25 zero-value result reads as unchecked", claim: Claim{Adapter: AdapterManual}, result: ClaimResult{}, goalBy: "2026-11-08", now: "2026-11-09", want: Fail, measured: Unchecked},
		{name: "26 unparseable since", claim: tracker("soon"), result: res(Fail, n(0)), goalBy: "2026-10-10", now: "2026-10-04", want: Behind, unread: "since"},
		{name: "27 invalid effort", claim: Claim{Adapter: AdapterManual, Effort: "a week"}, result: res(Unchecked, nil), goalBy: "2026-11-08", now: "2026-10-31", want: Behind, unread: "effort"},
		{name: "28 empty goal by", claim: Claim{Adapter: AdapterManual}, result: res(Fail, nil), goalBy: "", now: "2026-10-01", want: Behind, unread: "deadline"},
		{name: "29 deadline text is trimmed", claim: Claim{Adapter: AdapterManual, By: " 2026-10-05 "}, result: res(Fail, nil), goalBy: "2026-11-08", now: "2026-10-01", want: Open, deadline: "2026-10-05"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Read(tc.claim, tc.result, tc.goalBy, at(tc.now))
			if got.State != tc.want {
				t.Fatalf("state = %q, want %q (%+v)", got.State, tc.want, got)
			}
			if got.Unreadable != tc.unread {
				t.Errorf("Unreadable = %q, want %q", got.Unreadable, tc.unread)
			}
			wantMeasured := tc.measured
			if wantMeasured == "" {
				wantMeasured = tc.result.State
			}
			if got.Measured != wantMeasured {
				t.Errorf("Measured = %q, want %q", got.Measured, wantMeasured)
			}
			if got.Standing != tc.standing {
				t.Errorf("Standing = %v, want %v", got.Standing, tc.standing)
			}
			if tc.daysLeft != nil && (got.DaysLeft == nil || *got.DaysLeft != *tc.daysLeft) {
				t.Errorf("DaysLeft = %v, want %d", got.DaysLeft, *tc.daysLeft)
			}
			if tc.expected != nil && (got.Expected == nil || *got.Expected != *tc.expected) {
				t.Errorf("Expected = %v, want %d", got.Expected, *tc.expected)
			}
			if tc.deadline != "" && got.Deadline != tc.deadline {
				t.Errorf("Deadline = %q, want %q", got.Deadline, tc.deadline)
			}
		})
	}
}

// Calendar days are exact across a daylight-saving change and for a span that
// runs backwards, where rounding the hours toward zero would be off by one.
func TestDaysIsWholeCalendarDays(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	d := func(s string) time.Time { tm, _ := ParseDay(s, loc); return tm }
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2026-10-31", "2026-11-02", 2}, // 49 hours: the clocks go back on 11-01
		{"2026-03-07", "2026-03-09", 2}, // 47 hours: the clocks go forward on 03-08
		{"2026-10-06", "2026-10-04", -2},
		{"2026-10-04", "2026-10-04", 0},
	} {
		if got := days(d(c.a), d(c.b)); got != c.want {
			t.Errorf("days(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
