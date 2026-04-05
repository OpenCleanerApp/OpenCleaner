package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/cleaner"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/safety"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type Engine struct {
	rules  []rules.Rule
	broker *stream.Broker
	audit  *audit.Logger

	jobMu sync.Mutex

	mu       sync.Mutex
	lastScan map[string]types.ScanItem
}

func New(rules []rules.Rule, broker *stream.Broker, auditLogger *audit.Logger) *Engine {
	return &Engine{rules: rules, broker: broker, audit: auditLogger, lastScan: map[string]types.ScanItem{}}
}

func (e *Engine) Scan(ctx context.Context) (types.ScanResult, error) {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	start := time.Now()
	e.broker.Publish(types.ProgressEvent{Type: "scanning", Progress: 0, Message: "starting"})

	if len(e.rules) == 0 {
		res := types.ScanResult{TotalSize: 0, Items: []types.ScanItem{}, ScanDurationMs: time.Since(start).Milliseconds(), CategorizedSize: map[types.Category]int64{}}
		e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "scan complete"})
		return res, nil
	}

	// Filter to existing targets first (cheap), then size them concurrently.
	existing := make([]rules.Rule, 0, len(e.rules))
	for _, r := range e.rules {
		if _, err := os.Lstat(r.Path); err == nil {
			existing = append(existing, r)
		}
	}
	if len(existing) == 0 {
		res := types.ScanResult{TotalSize: 0, Items: []types.ScanItem{}, ScanDurationMs: time.Since(start).Milliseconds(), CategorizedSize: map[types.Category]int64{}}
		e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "scan complete"})
		return res, nil
	}

	type scanJob struct {
		r rules.Rule
	}
	type scanOut struct {
		r    rules.Rule
		size int64
	}

	jobs := make(chan scanJob)
	results := make(chan scanOut, len(existing))

	workerCount := runtime.NumCPU()
	if workerCount > 4 {
		workerCount = 4
	}
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(existing) {
		workerCount = len(existing)
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				sz, _ := getPathSize(ctx, j.r.Path)
				results <- scanOut{r: j.r, size: sz}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		defer close(jobs)
		for _, r := range existing {
			select {
			case <-ctx.Done():
				return
			case jobs <- scanJob{r: r}:
			}
		}
	}()

	items := make([]types.ScanItem, 0, len(existing))
	categorized := map[types.Category]int64{}

	done := 0
	for out := range results {
		done++
		progress := float64(done) / float64(len(existing))
		e.broker.Publish(types.ProgressEvent{Type: "scanning", Current: out.r.Path, Progress: progress})

		it := types.ScanItem{
			ID:          out.r.ID,
			Name:        out.r.Name,
			Path:        out.r.Path,
			Size:        out.size,
			Category:    out.r.Category,
			SafetyLevel: out.r.Safety,
			SafetyNote:  out.r.SafetyNote,
			Description: out.r.Desc,
		}
		items = append(items, it)
		categorized[out.r.Category] += out.size
	}

	if ctx.Err() != nil {
		return types.ScanResult{}, ctx.Err()
	}

	var total int64
	for _, v := range categorized {
		total += v
	}

	res := types.ScanResult{
		TotalSize:       total,
		Items:           items,
		ScanDurationMs:  time.Since(start).Milliseconds(),
		CategorizedSize: categorized,
	}

	e.mu.Lock()
	e.lastScan = map[string]types.ScanItem{}
	for _, it := range items {
		e.lastScan[it.ID] = it
	}
	e.mu.Unlock()

	e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "scan complete"})
	return res, nil
}

func (e *Engine) Clean(ctx context.Context, req types.CleanRequest) (types.CleanResult, error) {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	if len(req.ItemIDs) == 0 {
		return types.CleanResult{}, errors.New("no item_ids")
	}

	e.broker.Publish(types.ProgressEvent{Type: "cleaning", Progress: 0, Message: "starting"})

	e.mu.Lock()
	items := make([]types.ScanItem, 0, len(req.ItemIDs))
	for _, id := range req.ItemIDs {
		it, ok := e.lastScan[id]
		if ok {
			items = append(items, it)
		}
	}
	e.mu.Unlock()

	if len(items) == 0 {
		return types.CleanResult{}, errors.New("no matching items (run scan first)")
	}

	var cleanedSize int64
	cleanedCount := 0
	failed := []string{}

	for i, it := range items {
		select {
		case <-ctx.Done():
			return types.CleanResult{}, ctx.Err()
		default:
		}

		progress := float64(i) / float64(len(items))
		e.broker.Publish(types.ProgressEvent{Type: "cleaning", Current: it.Path, Progress: progress})

		if it.SafetyLevel == types.SafetyRisky && !req.Unsafe {
			failed = append(failed, it.ID)
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "blocked_risky", SrcPath: it.Path, Bytes: it.Size, DryRun: req.DryRun, OK: false, Error: "risky requires unsafe"})
			continue
		}

		if err := safety.ValidatePathSafety(it.Path); err != nil {
			failed = append(failed, it.ID)
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "blocked_safety", SrcPath: it.Path, Bytes: it.Size, DryRun: req.DryRun, OK: false, Error: err.Error()})
			continue
		}

		var dst string
		var err error
		switch req.Strategy {
		case types.CleanStrategyTrash, "":
			dst, err = cleaner.MoveToTrash(it.Path, req.DryRun)
		case types.CleanStrategyDelete:
			if !req.Force {
				err = errors.New("hard delete requires force")
				break
			}
			err = safety.SafeRemove(it.Path, req.DryRun)
		default:
			err = errors.New("unknown strategy")
		}

		ok := err == nil
		entry := audit.Entry{Time: time.Now().UTC(), Op: string(req.Strategy), SrcPath: it.Path, DstPath: dst, Bytes: it.Size, DryRun: req.DryRun, OK: ok}
		if err != nil {
			entry.Error = err.Error()
			failed = append(failed, it.ID)
		} else {
			cleanedSize += it.Size
			cleanedCount++
		}
		_ = e.audit.Append(entry)
	}

	e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "clean complete"})
	return types.CleanResult{CleanedSize: cleanedSize, CleanedCount: cleanedCount, FailedItems: failed, AuditLogPath: e.audit.Path(), DryRun: req.DryRun}, nil
}

func getPathSize(ctx context.Context, root string) (int64, error) {
	var total int64

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		info, err := os.Lstat(path)
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			total += info.Size()
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
