package publisher

import (
	"testing"
	"time"
)

func TestRenewDueAfterThirtyDays(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour)
	recent := now.Add(-29 * 24 * time.Hour)
	if !RenewDue(now, &old) || RenewDue(now, &recent) || !RenewDue(now, nil) {
		t.Fatal("incorrect renewal deadline")
	}
}
