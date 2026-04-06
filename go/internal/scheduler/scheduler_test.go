package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateSchedule(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Schedule
		wantErr bool
	}{
		{"valid daily", Schedule{Interval: IntervalDaily, TimeOfDay: "03:00"}, false},
		{"valid weekly", Schedule{Interval: IntervalWeekly, TimeOfDay: "09:30", DayOfWeek: 1}, false},
		{"valid monthly", Schedule{Interval: IntervalMonthly, TimeOfDay: "12:00"}, false},
		{"invalid interval", Schedule{Interval: "biweekly", TimeOfDay: "03:00"}, true},
		{"weekly bad day", Schedule{Interval: IntervalWeekly, TimeOfDay: "03:00", DayOfWeek: 7}, true},
		{"weekly negative day", Schedule{Interval: IntervalWeekly, TimeOfDay: "03:00", DayOfWeek: -1}, true},
		{"invalid time", Schedule{Interval: IntervalDaily, TimeOfDay: "nottime"}, true},
		{"empty time", Schedule{Interval: IntervalDaily, TimeOfDay: ""}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSchedule(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSchedule(%+v) error = %v, wantErr = %v", tt.cfg, err, tt.wantErr)
			}
		})
	}
}

func TestParseTimeOfDay(t *testing.T) {
	tests := []struct {
		input    string
		wantH, wantM int
	}{
		{"03:00", 3, 0},
		{"23:59", 23, 59},
		{"0:0", 0, 0},
		{"12:30", 12, 30},
		{"", 3, 0},        // defaults (n < 2)
		{"invalid", 3, 0}, // defaults (n < 2)
		{"25:00", 3, 0},   // hour out of range → default
	}
	for _, tt := range tests {
		h, m := parseTimeOfDay(tt.input)
		if h != tt.wantH || m != tt.wantM {
			t.Errorf("parseTimeOfDay(%q) = (%d,%d), want (%d,%d)", tt.input, h, m, tt.wantH, tt.wantM)
		}
	}
}

func TestNextRunTimeDaily(t *testing.T) {
	// If current time is before the schedule time, next run is today.
	now := time.Date(2025, 1, 15, 2, 0, 0, 0, time.Local)
	cfg := Schedule{Interval: IntervalDaily, TimeOfDay: "03:00"}

	next := nextRunTime(cfg, now)
	if next.Hour() != 3 || next.Day() != 15 {
		t.Errorf("expected today at 03:00, got %v", next)
	}

	// If current time is after, next run is tomorrow.
	now = time.Date(2025, 1, 15, 4, 0, 0, 0, time.Local)
	next = nextRunTime(cfg, now)
	if next.Hour() != 3 || next.Day() != 16 {
		t.Errorf("expected tomorrow at 03:00, got %v", next)
	}
}

func TestNextRunTimeWeekly(t *testing.T) {
	// Wednesday = 3
	now := time.Date(2025, 1, 13, 10, 0, 0, 0, time.Local) // Monday
	cfg := Schedule{Interval: IntervalWeekly, TimeOfDay: "09:00", DayOfWeek: 3}

	next := nextRunTime(cfg, now)
	if next.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %v", next.Weekday())
	}
	if !next.After(now) {
		t.Error("next should be after now")
	}
}

func TestNextRunTimeMonthly(t *testing.T) {
	now := time.Date(2025, 1, 15, 4, 0, 0, 0, time.Local)
	cfg := Schedule{Interval: IntervalMonthly, TimeOfDay: "03:00"}

	next := nextRunTime(cfg, now)
	// Already past today's 03:00, so next month.
	if next.Month() != time.February {
		t.Errorf("expected February, got %v", next.Month())
	}
}

func TestConfigPersistence(t *testing.T) {
	tmp := t.TempDir()

	cfg := Schedule{
		Enabled:   true,
		Interval:  IntervalDaily,
		TimeOfDay: "03:30",
		Notify:    true,
	}

	if err := SaveConfig(tmp, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Interval != cfg.Interval {
		t.Errorf("interval: got %s, want %s", loaded.Interval, cfg.Interval)
	}
	if loaded.TimeOfDay != cfg.TimeOfDay {
		t.Errorf("time: got %s, want %s", loaded.TimeOfDay, cfg.TimeOfDay)
	}
	if loaded.Notify != cfg.Notify {
		t.Errorf("notify: got %v, want %v", loaded.Notify, cfg.Notify)
	}
}

func TestConfigPathLocation(t *testing.T) {
	p := configPath("/Users/test")
	expected := filepath.Join("/Users/test", ".opencleaner", "scheduler.json")
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestRemoveConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := Schedule{Interval: IntervalDaily, TimeOfDay: "03:00"}
	if err := SaveConfig(tmp, cfg); err != nil {
		t.Fatal(err)
	}

	if err := RemoveConfig(tmp); err != nil {
		t.Fatal(err)
	}

	// Should not exist
	_, err := os.Stat(configPath(tmp))
	if !os.IsNotExist(err) {
		t.Error("config file should be removed")
	}
}

func TestHumanSizeScheduler(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500 B"},
		{5 * 1024 * 1024, "5.0 MB"},
		{2 * 1024 * 1024 * 1024, "2.0 GB"},
	}
	for _, tt := range tests {
		got := humanSizeScheduler(tt.input)
		if got != tt.want {
			t.Errorf("humanSizeScheduler(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
