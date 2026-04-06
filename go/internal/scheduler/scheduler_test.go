package scheduler

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
)

func newTestScheduler(t *testing.T) (*Scheduler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	broker := stream.NewBroker()
	auditPath := filepath.Join(home, ".opencleaner", "logs", "audit.log")
	eng := engine.New([]rules.Rule{}, broker, audit.NewLogger(auditPath))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(eng, broker, logger), home
}

func TestNewScheduler(t *testing.T) {
	s, _ := newTestScheduler(t)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	s, _ := newTestScheduler(t)

	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00", Notify: false}
	if err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}

	st := s.Status()
	if !st.Enabled {
		t.Error("expected enabled after start")
	}
	if st.NextRun == nil {
		t.Error("expected next_run after start")
	}

	s.Stop()
	st = s.Status()
	if st.Enabled {
		t.Error("expected disabled after stop")
	}
}

func TestSchedulerStartInvalidConfig(t *testing.T) {
	s, _ := newTestScheduler(t)
	err := s.Start(Schedule{Interval: "bad"})
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestSchedulerConfig(t *testing.T) {
	s, _ := newTestScheduler(t)

	// Before start, config should return empty schedule.
	cfg := s.Config()
	if cfg.Enabled {
		t.Error("expected disabled before start")
	}

	s.Start(Schedule{Enabled: true, Interval: IntervalWeekly, TimeOfDay: "09:00", DayOfWeek: 3})
	cfg = s.Config()
	if cfg.Interval != IntervalWeekly {
		t.Errorf("expected weekly, got %s", cfg.Interval)
	}
	if cfg.TimeOfDay != "09:00" {
		t.Errorf("expected 09:00, got %s", cfg.TimeOfDay)
	}
}

func TestSchedulerUpdateConfigEnable(t *testing.T) {
	s, _ := newTestScheduler(t)

	err := s.UpdateConfig(Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "04:00"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Status().Enabled {
		t.Error("expected enabled")
	}
}

func TestSchedulerUpdateConfigDisable(t *testing.T) {
	s, _ := newTestScheduler(t)

	s.Start(Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "04:00"})
	err := s.UpdateConfig(Schedule{Enabled: false, Interval: IntervalDaily, TimeOfDay: "04:00"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Status().Enabled {
		t.Error("expected disabled")
	}
}

func TestSchedulerStatusRunning(t *testing.T) {
	s, _ := newTestScheduler(t)
	st := s.Status()
	if st.Running {
		t.Error("expected not running initially")
	}
}

func TestSchedulerDoubleStop(t *testing.T) {
	s, _ := newTestScheduler(t)
	s.Start(Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"})
	s.Stop()
	s.Stop() // should not panic
}

func TestSchedulerDoubleStart(t *testing.T) {
	s, _ := newTestScheduler(t)
	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}
	s.Start(cfg)
	if err := s.Start(cfg); err != nil {
		t.Errorf("double start should succeed: %v", err)
	}
	s.Stop()
}

func TestSchedulerRunScanIntegration(t *testing.T) {
	s, home := newTestScheduler(t)

	// Create a target for the engine to scan.
	target := filepath.Join(home, "Library", "Caches", "test-target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	// Start scheduler with very short next run (we force via direct call).
	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00", Notify: false}
	s.Start(cfg)

	// Directly invoke runScan to test the scan path.
	stopCh := make(chan struct{})
	s.runScan(stopCh)

	// After runScan, running should be false.
	if s.Status().Running {
		t.Error("expected not running after scan completes")
	}
}

func TestSchedulerRunScanSkipsWhenStopped(t *testing.T) {
	s, _ := newTestScheduler(t)
	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}
	s.Start(cfg)

	stopCh := make(chan struct{})
	close(stopCh) // simulate stop
	s.runScan(stopCh)
	// Should return immediately without running scan.
}

func TestSchedulerRunScanSkipsIfBusy(t *testing.T) {
	s, _ := newTestScheduler(t)
	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}
	s.Start(cfg)

	// Simulate busy.
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	stopCh := make(chan struct{})
	s.runScan(stopCh)

	s.mu.Lock()
	// running should still be true (we set it).
	if !s.running {
		t.Error("expected still running (skip path)")
	}
	s.running = false
	s.mu.Unlock()
}

func TestSchedulerRunScanWithNotify(t *testing.T) {
	s, home := newTestScheduler(t)
	target := filepath.Join(home, "Library", "Caches", "test-notify")
	os.MkdirAll(target, 0o755)

	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00", Notify: true}
	s.Start(cfg)

	stopCh := make(chan struct{})
	// This will call sendMacOSNotification which may fail silently (no osascript).
	s.runScan(stopCh)
}

func TestScheduleNextWithNilConfig(t *testing.T) {
	s, _ := newTestScheduler(t)
	s.mu.Lock()
	s.config = nil
	s.scheduleNext() // should not panic
	s.mu.Unlock()
}

func TestNextRunTimeDefaultInterval(t *testing.T) {
	now := time.Date(2025, 1, 15, 2, 0, 0, 0, time.Local)
	cfg := Schedule{Interval: "unknown", TimeOfDay: "03:00"}
	next := nextRunTime(cfg, now)
	if !next.After(now) {
		t.Error("expected next to be after now for unknown interval (fallback)")
	}
}

func TestSendMacOSNotification(t *testing.T) {
	// Just ensure it doesn't panic. osascript may not be available in CI.
	sendMacOSNotification("Test", "test message")
}

func TestSchedulerScanContext(t *testing.T) {
	s, _ := newTestScheduler(t)
	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}
	s.Start(cfg)

	// Run scan in a goroutine and verify it completes.
	done := make(chan struct{})
	stopCh := make(chan struct{})
	go func() {
		s.runScan(stopCh)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(10 * time.Second):
		t.Fatal("runScan did not complete in time")
	}
}

func TestSchedulerStatusNextRunPresent(t *testing.T) {
	s, _ := newTestScheduler(t)
	s.Start(Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"})

	st := s.Status()
	if st.NextRun == nil {
		t.Fatal("expected next_run")
	}
	if !st.NextRun.After(time.Now()) {
		t.Error("next_run should be in the future")
	}
}

func TestSchedulerRunScanPublishesEvents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	broker := stream.NewBroker()
	eng := engine.New([]rules.Rule{}, broker, audit.NewLogger(filepath.Join(home, "audit.log")))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(eng, broker, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := broker.Subscribe(ctx)

	cfg := Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}
	s.Start(cfg)

	stopCh := make(chan struct{})
	s.runScan(stopCh)

	// Should have received at least the scheduled_scan events.
	gotScheduled := false
	for {
		select {
		case evt := <-ch:
			if evt.Type == "scheduled_scan" {
				gotScheduled = true
			}
		default:
			goto done
		}
	}
done:
	if !gotScheduled {
		t.Error("expected scheduled_scan event from broker")
	}
}

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

func TestSaveAndLoadConfig(t *testing.T) {
	home := t.TempDir()
	cfg := Schedule{Enabled: true, Interval: IntervalWeekly, TimeOfDay: "14:30", DayOfWeek: 3, Notify: true}
	if err := SaveConfig(home, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Interval != IntervalWeekly {
		t.Errorf("expected weekly, got %s", loaded.Interval)
	}
	if loaded.TimeOfDay != "14:30" {
		t.Errorf("expected 14:30, got %s", loaded.TimeOfDay)
	}
	if loaded.DayOfWeek != 3 {
		t.Errorf("expected day 3, got %d", loaded.DayOfWeek)
	}
}

func TestLoadConfigBadJSON(t *testing.T) {
	home := t.TempDir()
	p := configPath(home)
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte("not-json"), 0o600)
	_, err := LoadConfig(home)
	if err == nil {
		t.Error("expected error for bad JSON config")
	}
}

func TestLoadConfigNoFile(t *testing.T) {
	home := t.TempDir()
	_, err := LoadConfig(home)
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestParseTimeOfDayEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		wantHour int
		wantMin  int
	}{
		{"00:00", 0, 0},
		{"23:59", 23, 59},
		{"12:00", 12, 0},
		{"25:00", 3, 0},   // out-of-range hour defaults to 3
		{"12:60", 12, 0},  // out-of-range minute defaults to 0
		{"abc", 3, 0},     // invalid defaults to 3:00
		{"", 3, 0},        // empty defaults to 3:00
		{"03:00", 3, 0},
	}
	for _, tt := range tests {
		h, m := parseTimeOfDay(tt.input)
		if h != tt.wantHour || m != tt.wantMin {
			t.Errorf("parseTimeOfDay(%q) = (%d, %d), want (%d, %d)", tt.input, h, m, tt.wantHour, tt.wantMin)
		}
	}
}

func TestSaveConfigReadOnlyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create .opencleaner as a file to block MkdirAll.
	ocFile := filepath.Join(home, ".opencleaner")
	os.WriteFile(ocFile, []byte("block"), 0o600)

	err := SaveConfig(home, Schedule{Enabled: true})
	if err == nil {
		t.Error("expected error saving to blocked dir")
	}
}

func TestRemoveConfigNoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := RemoveConfig(home)
	if err == nil {
		t.Error("expected error removing non-existent config")
	}
}

func TestScheduleNextComputation(t *testing.T) {
	s, _ := newTestScheduler(t)

	cfg := Schedule{
		Enabled:   true,
		Interval:  "daily",
		TimeOfDay: "03:00",
	}
	if err := s.Start(cfg); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	time.Sleep(50 * time.Millisecond)

	got := s.Config()
	if !got.Enabled {
		t.Error("schedule should be enabled")
	}
}

func TestSchedulerRunScanError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	broker := stream.NewBroker()
	auditPath := filepath.Join(home, ".opencleaner", "logs", "audit.log")

	dir := filepath.Join(home, "test")
	os.MkdirAll(dir, 0o700)
	dup := rules.Rule{
		ID: "dup", Name: "dup", Path: dir,
		Category: "developer", Safety: "safe",
		SafetyNote: "t", Desc: "t",
	}
	eng := engine.New([]rules.Rule{dup, dup}, broker, audit.NewLogger(auditPath))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(eng, broker, logger)
	s.config = &Schedule{Enabled: true, Interval: IntervalDaily, TimeOfDay: "03:00"}

	ch := broker.Subscribe(context.Background())
	stopCh := make(chan struct{})
	defer close(stopCh)
	go s.runScan(stopCh)

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case evt := <-ch:
			if evt.Type == "scheduled_scan" && evt.Message != "scheduled scan starting" {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for scheduled scan error")
		}
	}
}
