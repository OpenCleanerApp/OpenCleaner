package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/opencleaner/opencleaner/internal/analyzer"
	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/cleaner"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/safety"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type Engine struct {
	rules    []rules.Rule
	scanners []rules.Scanner
	broker   *stream.Broker
	audit    *audit.Logger

	jobMu sync.Mutex

	mu       sync.Mutex
	lastScan map[string]types.ScanItem
	lastUndo *cleaner.UndoManifest
}

func New(rules []rules.Rule, broker *stream.Broker, auditLogger *audit.Logger) *Engine {
	return &Engine{rules: rules, broker: broker, audit: auditLogger, lastScan: map[string]types.ScanItem{}}
}

func (e *Engine) AddScanner(s rules.Scanner) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scanners = append(e.scanners, s)
}

func (e *Engine) Scan(ctx context.Context) (types.ScanResult, error) {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	start := time.Now()
	e.broker.Publish(types.ProgressEvent{Type: "scanning", Progress: 0, Message: "starting"})

	e.mu.Lock()
	staticRules := append([]rules.Rule(nil), e.rules...)
	scanners := append([]rules.Scanner(nil), e.scanners...)
	e.mu.Unlock()

	if len(staticRules) == 0 && len(scanners) == 0 {
		res := types.ScanResult{TotalSize: 0, Items: []types.ScanItem{}, ScanDurationMs: time.Since(start).Milliseconds(), CategorizedSize: map[types.Category]int64{}}
		e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "scan complete"})
		return res, nil
	}

	allRules := make([]rules.Rule, 0, len(staticRules))
	seenIDs := map[string]struct{}{}
	var scanWarnings []string
	for _, r := range staticRules {
		if err := validateRule(r); err != nil {
			return types.ScanResult{}, fmt.Errorf("invalid static rule %q: %w", r.ID, err)
		}
		if _, ok := seenIDs[r.ID]; ok {
			return types.ScanResult{}, fmt.Errorf("duplicate rule id %q", r.ID)
		}
		seenIDs[r.ID] = struct{}{}
		allRules = append(allRules, r)
	}

	for _, sc := range scanners {
		rs, err := sc.Scan(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return types.ScanResult{}, ctx.Err()
			}
			scanWarnings = append(scanWarnings, fmt.Sprintf("scanner %s: %s", sc.ID(), err.Error()))
			continue
		}
		for _, r := range rs {
			if r.Category == "" {
				r.Category = sc.Category()
			}
			if err := validateRule(r); err != nil {
				scanWarnings = append(scanWarnings, fmt.Sprintf("scanner %s returned invalid rule %q: %s", sc.ID(), r.ID, err.Error()))
				continue
			}
			if _, ok := seenIDs[r.ID]; ok {
				continue
			}
			seenIDs[r.ID] = struct{}{}
			allRules = append(allRules, r)
		}
	}

	// Filter to existing targets first (cheap), then size them concurrently.
	existing := make([]rules.Rule, 0, len(allRules))
	for _, r := range allRules {
		if _, err := os.Lstat(r.Path); err == nil {
			existing = append(existing, r)
		}
	}
	if len(existing) == 0 {
		res := types.ScanResult{TotalSize: 0, Items: []types.ScanItem{}, ScanDurationMs: time.Since(start).Milliseconds(), CategorizedSize: map[types.Category]int64{}, Warnings: scanWarnings}
		e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "scan complete"})
		return res, nil
	}

	type scanJob struct {
		r rules.Rule
	}
	type scanOut struct {
		r          rules.Rule
		size       int64
		lastAccess *time.Time
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
				var sz int64
				if j.r.PresetSize != nil {
					sz = *j.r.PresetSize
				} else {
					sz, _ = getPathSize(ctx, j.r.Path)
				}
				var la *time.Time
				if info, err := os.Stat(j.r.Path); err == nil {
					mt := info.ModTime()
					la = &mt
				}
				results <- scanOut{r: j.r, size: sz, lastAccess: la}
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
			LastAccess:  out.lastAccess,
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
		Warnings:        scanWarnings,
		Suggestions:     analyzer.New().Analyze(items),
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

	excludes := normalizeExcludePaths(req.ExcludePaths)

	var cleanedSize int64
	cleanedCount := 0
	failed := []string{}
	undoEntries := make([]cleaner.UndoEntry, 0, len(items))

	undoHome := ""
	undoTrash := ""
	undoRecordEnabled := false
	if !req.DryRun && (req.Strategy == types.CleanStrategyTrash || req.Strategy == "") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			undoHome = filepath.Clean(home)
			trash, err := cleaner.TrashDir()
			if err == nil {
				undoTrash = filepath.Clean(trash)
				if isWithinRoot(undoHome, undoTrash) && safety.ValidateNoSymlinkAncestorsWithin(undoHome, undoTrash) == nil {
					undoRecordEnabled = true
				}
			}
		}
	}

	for i, it := range items {
		select {
		case <-ctx.Done():
			return types.CleanResult{}, ctx.Err()
		default:
		}

		progress := float64(i) / float64(len(items))
		e.broker.Publish(types.ProgressEvent{Type: "cleaning", Current: it.Path, Progress: progress})

		if isExcludedPath(it.Path, excludes) {
			failed = append(failed, it.ID)
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "blocked_exclude", SrcPath: it.Path, Bytes: it.Size, DryRun: req.DryRun, OK: false, Error: "excluded by user"})
			continue
		}

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
			if undoRecordEnabled {
				if err := validateUndoRecordCandidate(undoHome, undoTrash, it.Path, dst); err == nil {
					undoEntries = append(undoEntries, cleaner.UndoEntry{SrcPath: it.Path, DstPath: dst, Bytes: it.Size, Time: time.Now().UTC()})
				} else {
					_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_unavailable", SrcPath: it.Path, DstPath: dst, Bytes: it.Size, DryRun: req.DryRun, OK: false, Error: err.Error()})
				}
			}
		}
		_ = e.audit.Append(entry)
	}

	if !req.DryRun && len(undoEntries) > 0 {
		if err := cleaner.SaveManifest(undoEntries, ""); err != nil {
			e.broker.Publish(types.ProgressEvent{Type: "warning", Message: "failed to save undo manifest: " + err.Error()})
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_manifest", Bytes: cleanedSize, DryRun: req.DryRun, OK: false, Error: err.Error()})
		} else {
			e.mu.Lock()
			e.lastUndo = &cleaner.UndoManifest{Version: 1, Entries: undoEntries}
			e.mu.Unlock()
		}
	}
	if !req.DryRun && cleanedCount > 0 && req.Strategy == types.CleanStrategyDelete {
		if err := cleaner.ClearManifest(""); err != nil {
			e.broker.Publish(types.ProgressEvent{Type: "warning", Message: "failed to clear undo manifest: " + err.Error()})
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_manifest_clear", Bytes: cleanedSize, DryRun: req.DryRun, OK: false, Error: err.Error()})
		} else {
			e.mu.Lock()
			e.lastUndo = nil
			e.mu.Unlock()
		}
	}

	e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "clean complete"})
	return types.CleanResult{CleanedSize: cleanedSize, CleanedCount: cleanedCount, FailedItems: failed, AuditLogPath: e.audit.Path(), DryRun: req.DryRun}, nil
}

func (e *Engine) Undo(ctx context.Context) (types.UndoResult, error) {
	e.jobMu.Lock()
	defer e.jobMu.Unlock()

	e.broker.Publish(types.ProgressEvent{Type: "undoing", Progress: 0, Message: "starting"})
	m, err := cleaner.LoadManifest("")
	if err != nil {
		return types.UndoResult{}, err
	}

	restored, failed, err := cleaner.Restore(ctx, m)
	if err != nil {
		return types.UndoResult{}, err
	}

	failedSet := map[string]struct{}{}
	for _, p := range failed {
		failedSet[filepath.Clean(p)] = struct{}{}
	}
	var restoredSize int64
	for _, ent := range m.Entries {
		if _, ok := failedSet[filepath.Clean(ent.SrcPath)]; ok {
			continue
		}
		restoredSize += ent.Bytes
		_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo", SrcPath: ent.DstPath, DstPath: ent.SrcPath, Bytes: ent.Bytes, DryRun: false, OK: true})
	}
	for _, p := range failed {
		_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo", SrcPath: p, Bytes: 0, DryRun: false, OK: false, Error: "restore failed"})
	}

	if len(failed) == 0 {
		if err := cleaner.ClearManifest(""); err != nil {
			e.broker.Publish(types.ProgressEvent{Type: "warning", Message: "failed to clear undo manifest: " + err.Error()})
			_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_manifest_clear", DryRun: false, OK: false, Error: err.Error()})
		}
		e.mu.Lock()
		e.lastUndo = nil
		e.mu.Unlock()
	} else {
		remaining := make([]cleaner.UndoEntry, 0, len(failed))
		for _, ent := range m.Entries {
			if _, ok := failedSet[filepath.Clean(ent.SrcPath)]; ok {
				remaining = append(remaining, ent)
			}
		}
		if len(remaining) == 0 {
			if err := cleaner.ClearManifest(""); err != nil {
				e.broker.Publish(types.ProgressEvent{Type: "warning", Message: "failed to clear undo manifest: " + err.Error()})
				_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_manifest_clear", DryRun: false, OK: false, Error: err.Error()})
			}
		} else {
			if err := cleaner.SaveManifest(remaining, ""); err != nil {
				e.broker.Publish(types.ProgressEvent{Type: "warning", Message: "failed to save undo manifest: " + err.Error()})
				_ = e.audit.Append(audit.Entry{Time: time.Now().UTC(), Op: "undo_manifest", DryRun: false, OK: false, Error: err.Error()})
			}
		}
		e.mu.Lock()
		e.lastUndo = &cleaner.UndoManifest{Version: m.Version, Entries: remaining}
		e.mu.Unlock()
	}

	e.broker.Publish(types.ProgressEvent{Type: "complete", Progress: 1, Message: "undo complete"})
	return types.UndoResult{RestoredCount: restored, RestoredSize: restoredSize, FailedItems: failed}, nil
}

func isWithinRoot(root, absPath string) bool {
	if root == "" || absPath == "" {
		return false
	}
	root = filepath.Clean(root)
	absPath = filepath.Clean(absPath)
	if !filepath.IsAbs(root) || !filepath.IsAbs(absPath) {
		return false
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func validateUndoRecordCandidate(home, trash, src, dst string) error {
	if home == "" || trash == "" {
		return errors.New("undo roots not available")
	}
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	if src == "" || dst == "" {
		return errors.New("empty path")
	}
	if !filepath.IsAbs(src) || !filepath.IsAbs(dst) {
		return errors.New("paths must be absolute")
	}
	if safety.HasTraversalPattern(src) || safety.HasTraversalPattern(dst) {
		return errors.New("path contains traversal pattern")
	}
	if !isWithinRoot(home, src) {
		return errors.New("src outside home")
	}
	if !isWithinRoot(home, trash) {
		return errors.New("trash outside home")
	}
	if !isWithinRoot(trash, dst) {
		return errors.New("dst outside trash")
	}
	if err := safety.ValidatePathSafety(src); err != nil {
		return err
	}
	if err := safety.ValidateNoSymlinkAncestorsWithin(home, filepath.Dir(src)); err != nil {
		return err
	}
	if err := safety.ValidateNoSymlinkAncestorsWithin(home, filepath.Dir(dst)); err != nil {
		return err
	}
	return nil
}

func validateRule(r rules.Rule) error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("empty id")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("empty name")
	}
	if strings.TrimSpace(r.Path) == "" {
		return errors.New("empty path")
	}
	if !filepath.IsAbs(r.Path) {
		return errors.New("path must be absolute")
	}
	if safety.HasTraversalPattern(r.Path) {
		return errors.New("path contains traversal pattern")
	}
	if r.Category == "" {
		return errors.New("empty category")
	}
	if r.Safety == "" {
		return errors.New("empty safety")
	}
	return nil
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

func normalizeExcludePaths(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	home, _ := os.UserHomeDir()
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}

	for _, raw := range in {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}

		p = expandTilde(p, home)
		if !filepath.IsAbs(p) && home != "" {
			p = filepath.Join(home, p)
		}
		p = filepath.Clean(p)
		if p == "" || p == "." {
			continue
		}

		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	return out
}

func expandTilde(p, home string) string {
	if home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	prefix := "~" + string(os.PathSeparator)
	if strings.HasPrefix(p, prefix) {
		return filepath.Join(home, strings.TrimPrefix(p, prefix))
	}
	return p
}

func isExcludedPath(path string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	p := filepath.Clean(path)
	for _, ex := range excludes {
		if ex == "" {
			continue
		}
		if p == ex {
			return true
		}
		if strings.HasPrefix(p, ex+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
