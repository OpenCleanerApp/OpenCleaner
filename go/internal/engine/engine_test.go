package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/cleaner"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

func newTestEngine(t *testing.T, ruleSet []rules.Rule) (*Engine, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	auditPath, err := audit.DefaultAuditLogPath()
	if err != nil {
		t.Fatal(err)
	}
	eng := New(ruleSet, stream.NewBroker(), audit.NewLogger(auditPath))
	return eng, home
}

type testScanner struct {
	id    string
	cat   types.Category
	rules []rules.Rule
	err   error
}

func (s testScanner) ID() string               { return s.id }
func (s testScanner) Name() string             { return s.id }
func (s testScanner) Category() types.Category { return s.cat }
func (s testScanner) Scan(ctx context.Context) ([]rules.Rule, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]rules.Rule(nil), s.rules...), nil
}

func TestScanEmpty(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(res.Items))
	}
	if res.TotalSize != 0 {
		t.Fatalf("expected total size 0, got %d", res.TotalSize)
	}
}

func TestScanWithTempTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "scan-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "b.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Size != int64(len("abc")+len("hello")) {
		t.Fatalf("unexpected size: %d", res.Items[0].Size)
	}
}

func TestScanMergesScannerRulesAndDedupes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	staticTarget := filepath.Join(home, "Library", "Caches", "static")
	scannerTarget := filepath.Join(home, "Library", "Caches", "scanner")
	if err := os.MkdirAll(staticTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(scannerTarget, 0o700); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "static",
		Name:       "Static",
		Path:       staticTarget,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))
	eng.AddScanner(testScanner{id: "scanner", cat: types.CategoryDeveloper, rules: []rules.Rule{{
		ID:         "static",
		Name:       "Duplicate",
		Path:       "/does/not/matter",
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}, {
		ID:         "dyn",
		Name:       "Dynamic",
		Path:       scannerTarget,
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}})

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(res.Items))
	}
	ids := map[string]struct{}{}
	for _, it := range res.Items {
		ids[it.ID] = struct{}{}
	}
	if _, ok := ids["static"]; !ok {
		t.Fatalf("expected static rule")
	}
	if _, ok := ids["dyn"]; !ok {
		t.Fatalf("expected dynamic scanner rule")
	}
}

func TestScanRejectsInvalidStaticRule(t *testing.T) {
	eng, _ := newTestEngine(t, []rules.Rule{{
		ID:       "",
		Name:     "Bad",
		Path:     "/tmp",
		Category: types.CategorySystem,
		Safety:   types.SafetySafe,
	}})
	if _, err := eng.Scan(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestScanRejectsInvalidScannerRule(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	eng.AddScanner(testScanner{id: "scanner", cat: types.CategoryDeveloper, rules: []rules.Rule{{
		ID:         "dyn",
		Name:       "Bad",
		Path:       "relative/path",
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}})
	if _, err := eng.Scan(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestCleanTrashStrategy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "clean-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 scan item, got %d", len(res.Items))
	}

	_, err = eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target moved, got err=%v", err)
	}
	trash, err := cleaner.TrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(trash, filepath.Base(target))); err != nil {
		t.Fatalf("expected item in trash: %v", err)
	}
	if _, err := cleaner.LoadManifest(""); err != nil {
		t.Fatalf("expected undo manifest to exist: %v", err)
	}
}

func TestCleanTrashOutsideHomeDoesNotWriteManifest(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	other := filepath.Join(tmp, "other")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	target := filepath.Join(other, "Library", "Caches", "outside-home")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: false}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target moved, got err=%v", err)
	}
	trash, err := cleaner.TrashDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(trash, filepath.Base(target))); err != nil {
		t.Fatalf("expected item in trash: %v", err)
	}
	if _, err := cleaner.LoadManifest(""); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no manifest for outside-home trash clean, got err=%v", err)
	}
}

func TestCleanDryRunDoesNotMoveOrWriteManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "dryrun-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target to remain: %v", err)
	}
	if _, err := cleaner.LoadManifest(""); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no manifest, got err=%v", err)
	}
}

func TestCleanExcludePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "exclude-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: true, ExcludePaths: []string{target}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedItems) != 1 {
		t.Fatalf("expected 1 failed item, got %v", res.FailedItems)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target to remain: %v", err)
	}
}

func TestCleanRiskyRequiresUnsafe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "risky-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetyRisky,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedItems) != 1 {
		t.Fatalf("expected 1 failed item, got %v", res.FailedItems)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target to remain: %v", err)
	}
}

func TestUndoRestoresFromTrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "undo-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-target",
		Name:       "Test Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"test-target"}, Strategy: types.CleanStrategyTrash, DryRun: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target moved, got err=%v", err)
	}

	undoRes, err := eng.Undo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if undoRes.RestoredCount != 1 {
		t.Fatalf("expected restored=1, got %d", undoRes.RestoredCount)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target restored: %v", err)
	}
	if _, err := cleaner.LoadManifest(""); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected manifest cleared after undo, got err=%v", err)
	}
}

func TestUndoNoManifest(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	_, err := eng.Undo(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
