package sqlite

import "time"

const timeFormat = time.RFC3339

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeFormat, s)
}
