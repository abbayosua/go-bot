package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed five-field cron expression: minute, hour, day of month,
// month, day of week.
type Schedule struct {
	minute, hour, dom, month, dow field
	domStar, dowStar              bool
}

type field struct {
	values map[int]bool
}

func Parse(expr string) (Schedule, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("cron needs five fields, got %d", len(parts))
	}
	minute, err := parseField(parts[0], 0, 59)
	if err != nil {
		return Schedule{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := parseField(parts[1], 0, 23)
	if err != nil {
		return Schedule{}, fmt.Errorf("hour: %w", err)
	}
	dom, err := parseField(parts[2], 1, 31)
	if err != nil {
		return Schedule{}, fmt.Errorf("day of month: %w", err)
	}
	month, err := parseField(parts[3], 1, 12)
	if err != nil {
		return Schedule{}, fmt.Errorf("month: %w", err)
	}
	dow, err := parseField(parts[4], 0, 6)
	if err != nil {
		return Schedule{}, fmt.Errorf("day of week: %w", err)
	}
	return Schedule{
		minute: minute, hour: hour, dom: dom, month: month, dow: dow,
		domStar: parts[2] == "*", dowStar: parts[4] == "*",
	}, nil
}

func parseField(raw string, min, max int) (field, error) {
	values := map[int]bool{}
	for _, piece := range strings.Split(raw, ",") {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			return field{}, fmt.Errorf("empty value in %q", raw)
		}
		step := 1
		if base, stepRaw, ok := strings.Cut(piece, "/"); ok {
			parsed, err := strconv.Atoi(stepRaw)
			if err != nil || parsed <= 0 {
				return field{}, fmt.Errorf("bad step %q", stepRaw)
			}
			step = parsed
			piece = base
		}
		start, end := min, max
		switch {
		case piece == "*":
		case strings.Contains(piece, "-"):
			lowRaw, highRaw, _ := strings.Cut(piece, "-")
			low, err := strconv.Atoi(lowRaw)
			if err != nil {
				return field{}, fmt.Errorf("bad range start %q", lowRaw)
			}
			high, err := strconv.Atoi(highRaw)
			if err != nil {
				return field{}, fmt.Errorf("bad range end %q", highRaw)
			}
			start, end = low, high
		default:
			value, err := strconv.Atoi(piece)
			if err != nil {
				return field{}, fmt.Errorf("bad value %q", piece)
			}
			start, end = value, value
		}
		if start < min || end > max || start > end {
			return field{}, fmt.Errorf("value out of range %d-%d: %q", min, max, piece)
		}
		for value := start; value <= end; value += step {
			values[value] = true
		}
	}
	return field{values: values}, nil
}

// Next returns the first matching minute strictly after t, in t's location.
func (s Schedule) Next(t time.Time) (time.Time, error) {
	start := t.Truncate(time.Minute).Add(time.Minute)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	for offset := 0; offset <= 366*5; offset++ {
		current := day.AddDate(0, 0, offset)
		if !s.month.values[int(current.Month())] {
			continue
		}
		if !s.dayMatches(current) {
			continue
		}
		for hour := 0; hour < 24; hour++ {
			if !s.hour.values[hour] {
				continue
			}
			for minute := 0; minute < 60; minute++ {
				if !s.minute.values[minute] {
					continue
				}
				candidate := time.Date(current.Year(), current.Month(), current.Day(), hour, minute, 0, 0, current.Location())
				if candidate.After(t) {
					return candidate, nil
				}
			}
		}
	}
	return time.Time{}, fmt.Errorf("no matching time within five years")
}

func (s Schedule) dayMatches(day time.Time) bool {
	domMatch := s.dom.values[day.Day()]
	dowMatch := s.dow.values[int(day.Weekday())]
	if s.domStar && s.dowStar {
		return true
	}
	if s.domStar {
		return dowMatch
	}
	if s.dowStar {
		return domMatch
	}
	return domMatch || dowMatch
}
