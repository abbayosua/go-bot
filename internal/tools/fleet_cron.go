package tools

import (
	"time"

	"gobot/internal/schedule"
)

// scheduleNext resolves a cron expression to its next firing time.
func scheduleNext(cron string) (int64, error) {
	parsed, err := schedule.Parse(cron)
	if err != nil {
		return 0, err
	}
	next, err := parsed.Next(time.Now())
	if err != nil {
		return 0, err
	}
	return next.Unix(), nil
}
