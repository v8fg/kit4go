package datetime

import (
	"errors"
	"time"
)

// Parse time use the Local location
func Parse(layout, value string) (time.Time, error) {
	return ParseInLocation(layout, value, time.Local)
}

// ParseMostOne parse time most one use the Local location
func ParseMostOne(layouts []string, value string) (time.Time, error) {
	for _, layout := range layouts {
		if t, err := ParseInLocation(layout, value, time.Local); err == nil {
			return t, err
		}
	}
	return time.Time{}, errors.New("parse time failed for all layouts")
}

// ParseInLocation parse time uses the given location
func ParseInLocation(layout, value string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	return time.ParseInLocation(layout, value, loc)
}

// ParseMostOneInLocation parse time most one uses the given location
func ParseMostOneInLocation(layouts []string, value string, loc *time.Location) (time.Time, error) {
	for _, layout := range layouts {
		if t, err := ParseInLocation(layout, value, loc); err == nil {
			return t, err
		}
	}
	return time.Time{}, errors.New("parse time with location failed for all layouts")
}
