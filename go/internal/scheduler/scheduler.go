package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/opencleaner/opencleaner/internal/engine"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type Interval string

const (
	IntervalDaily   Interval = "daily"
	IntervalWeekly  Interval = "weekly"
	IntervalMonthly Interval = "monthly"
)

// Schedule defines when automatic scans run.
type Schedule struct {
	Enabled   bool     `json:"enabled"`
	Interval  Interval `json:"interval"`
	TimeOfDay string   `json:"time"`            // "HH:MM" 24h format
	DayOfWeek int      `json:"day,omitempty"`    // 0=Sun..6=Sat (for weekly)
	Notify    bool     `json:"notify,omitempty"` // macOS notification on completion
}

// ScheduleStatus is the API response for schedule queries.
type ScheduleStatus struct {
	Schedule
	NextRun *time.Time `json:"next_run,omitempty"`
	Running bool       `json:"running"`
}

// Scheduler manages periodic scans.
type Scheduler struct {
	engine *engine.Engine
	broker *stream.Broker
	logger *slog.Logger

	mu      sync.Mutex
	config  *Schedule
	timer   *time.Timer
	stopCh  chan struct{}
	running bool
}

// New creates a Scheduler (initially stopped).
func New(eng *engine.Engine, broker *stream.Broker, logger *slog.Logger) *Scheduler {
	return &Scheduler{engine: eng, broker: broker, logger: logger}
}

// Start begins the scheduling loop with the given configuration.
func (s *Scheduler) Start(cfg Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateSchedule(cfg); err != nil {
		return err
	}

	s.stop()
	cfg.Enabled = true
	s.config = &cfg
	s.stopCh = make(chan struct{})
	s.scheduleNext()
	return nil
}

// Stop disables the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
}

func (s *Scheduler) stop() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
		s.stopCh = nil
	}
	if s.config != nil {
		s.config.Enabled = false
	}
}

// UpdateConfig updates the schedule configuration. Returns error if invalid.
func (s *Scheduler) UpdateConfig(cfg Schedule) error {
	if cfg.Enabled {
		return s.Start(cfg)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stop()
	s.config = &cfg
	return nil
}

// Config returns the current schedule configuration.
func (s *Scheduler) Config() *Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config == nil {
		return &Schedule{}
	}
	cfg := *s.config
	return &cfg
}

// Status returns the full schedule status including next run time.
func (s *Scheduler) Status() ScheduleStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := ScheduleStatus{Running: s.running}
	if s.config != nil {
		st.Schedule = *s.config
	}
	if s.config != nil && s.config.Enabled {
		next := nextRunTime(*s.config, time.Now())
		st.NextRun = &next
	}
	return st
}

func (s *Scheduler) scheduleNext() {
	if s.config == nil || !s.config.Enabled {
		return
	}

	next := nextRunTime(*s.config, time.Now())
	delay := time.Until(next)
	if delay < time.Second {
		delay = time.Second
	}

	s.logger.Info("scheduler: next scan", "at", next, "in", delay.Round(time.Second))

	stopCh := s.stopCh
	s.timer = time.AfterFunc(delay, func() {
		s.runScan(stopCh)
	})
}

func (s *Scheduler) runScan(stopCh chan struct{}) {
	select {
	case <-stopCh:
		return
	default:
	}

	s.mu.Lock()
	if s.running {
		s.logger.Info("scheduler: skipping (scan already running)")
		s.scheduleNext()
		s.mu.Unlock()
		return
	}
	s.running = true
	notify := s.config != nil && s.config.Notify
	s.mu.Unlock()

	s.logger.Info("scheduler: starting scan")
	s.broker.Publish(types.ProgressEvent{Type: "scheduled_scan", Message: "scheduled scan starting"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	res, err := s.engine.Scan(ctx)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if err != nil {
		s.logger.Error("scheduler: scan failed", "err", err)
		s.broker.Publish(types.ProgressEvent{Type: "scheduled_scan", Message: "scheduled scan failed: " + err.Error()})
	} else {
		msg := fmt.Sprintf("Scheduled scan complete: %d items, %s reclaimable",
			len(res.Items), humanSizeScheduler(res.TotalSize))
		s.logger.Info("scheduler: scan complete", "items", len(res.Items), "size", res.TotalSize)
		s.broker.Publish(types.ProgressEvent{Type: "scheduled_scan", Message: msg})

		if notify {
			sendMacOSNotification("OpenCleaner Scan Complete", msg)
		}
	}

	s.mu.Lock()
	s.scheduleNext()
	s.mu.Unlock()
}

func nextRunTime(cfg Schedule, now time.Time) time.Time {
	hour, min := parseTimeOfDay(cfg.TimeOfDay)

	target := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())

	switch cfg.Interval {
	case IntervalDaily:
		if !target.After(now) {
			target = target.Add(24 * time.Hour)
		}
	case IntervalWeekly:
		for target.Weekday() != time.Weekday(cfg.DayOfWeek) || !target.After(now) {
			target = target.Add(24 * time.Hour)
		}
	case IntervalMonthly:
		if !target.After(now) {
			target = target.AddDate(0, 1, 0)
		}
	default:
		target = target.Add(24 * time.Hour) // fallback: daily
	}
	return target
}

func parseTimeOfDay(s string) (hour, min int) {
	n, _ := fmt.Sscanf(s, "%d:%d", &hour, &min)
	if n < 2 {
		return 3, 0 // default: 03:00
	}
	if hour < 0 || hour > 23 {
		hour = 3
	}
	if min < 0 || min > 59 {
		min = 0
	}
	return
}

func validateSchedule(cfg Schedule) error {
	switch cfg.Interval {
	case IntervalDaily, IntervalWeekly, IntervalMonthly:
	default:
		return fmt.Errorf("invalid interval: %q", cfg.Interval)
	}
	if cfg.TimeOfDay == "" {
		return fmt.Errorf("time is required (use HH:MM)")
	}
	n, _ := fmt.Sscanf(cfg.TimeOfDay, "%d:%d", new(int), new(int))
	if n < 2 {
		return fmt.Errorf("invalid time: %q (use HH:MM)", cfg.TimeOfDay)
	}
	if cfg.Interval == IntervalWeekly && (cfg.DayOfWeek < 0 || cfg.DayOfWeek > 6) {
		return fmt.Errorf("invalid day of week: %d (use 0=Sun..6=Sat)", cfg.DayOfWeek)
	}
	return nil
}

func sendMacOSNotification(title, message string) {
	script := fmt.Sprintf(`display notification %q with title %q`, message, title)
	_ = exec.Command("osascript", "-e", script).Run()
}

func humanSizeScheduler(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// Config persistence.

func configPath(home string) string {
	return filepath.Join(home, ".opencleaner", "scheduler.json")
}

// LoadConfig loads scheduler config from ~/.opencleaner/scheduler.json.
func LoadConfig(home string) (*Schedule, error) {
	data, err := os.ReadFile(configPath(home))
	if err != nil {
		return nil, err
	}
	var cfg Schedule
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig persists scheduler config to ~/.opencleaner/scheduler.json.
func SaveConfig(home string, cfg Schedule) error {
	p := configPath(home)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// RemoveConfig removes the scheduler config file.
func RemoveConfig(home string) error {
	return os.Remove(configPath(home))
}
