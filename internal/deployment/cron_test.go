package deployment

import (
	"testing"
	"time"
)

func TestCronNextUsesFiveFieldUTC(t *testing.T) {
	after := time.Date(2026, 9, 9, 10, 7, 0, 0, time.FixedZone("local", 7*60*60))
	next, err := CronNext("*/15 * * * *", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 9, 3, 15, 0, 0, time.UTC)
	if !next.Equal(want) || next.Location() != time.UTC {
		t.Fatalf("next=%s want=%s", next, want)
	}
	for _, invalid := range []string{"", "@hourly", "* * * *", "61 * * * *"} {
		if _, err = CronNext(invalid, after); err == nil {
			t.Fatalf("accepted invalid schedule %q", invalid)
		}
	}
}
