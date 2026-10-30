package publisher

import "time"

func RenewDue(now time.Time, last *time.Time) bool {
	return last == nil || !now.Before(last.Add(30*24*time.Hour))
}
