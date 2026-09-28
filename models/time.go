package models

import "time"

const TimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func NowRFC3339() string {
	return time.Now().UTC().Format(TimeLayout)
}
