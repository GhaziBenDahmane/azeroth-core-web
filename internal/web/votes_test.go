package web

import (
	"testing"
	"time"
)

func TestConsecutiveVoteDays(t *testing.T) {
	now := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		days []time.Time
		want uint32
	}{
		{"none", nil, 0},
		{"today", []time.Time{now}, 1},
		{"continues from yesterday", []time.Time{now.AddDate(0, 0, -1), now.AddDate(0, 0, -2)}, 2},
		{"today with gap", []time.Time{now, now.AddDate(0, 0, -2)}, 1},
		{"expired", []time.Time{now.AddDate(0, 0, -2)}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := consecutiveVoteDays(test.days, now); got != test.want {
				t.Fatalf("streak = %d, want %d", got, test.want)
			}
		})
	}
}
