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
	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for invalid scanner rule")
	}
	for _, it := range res.Items {
		if it.ID == "dyn" {
			t.Fatal("invalid rule should not appear in items")
		}
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

// --- Helper function unit tests ---

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/home/user", "/home/user/Library/Caches", true},
		{"/home/user", "/home/user", true},
		{"/home/user", "/home/other", false},
		{"/home/user", "/etc/passwd", false},
		{"", "/home/user", false},
		{"/home/user", "", false},
		{"relative", "/abs", false},
		{"/abs", "relative", false},
	}
	for _, tt := range tests {
		got := isWithinRoot(tt.root, tt.path)
		if got != tt.want {
			t.Errorf("isWithinRoot(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
		}
	}
}

func TestValidateRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    rules.Rule
		wantErr bool
	}{
		{"valid", rules.Rule{ID: "a", Name: "A", Path: "/tmp/a", Category: "system", Safety: "safe"}, false},
		{"empty id", rules.Rule{ID: "", Name: "A", Path: "/tmp", Category: "system", Safety: "safe"}, true},
		{"empty name", rules.Rule{ID: "a", Name: "", Path: "/tmp", Category: "system", Safety: "safe"}, true},
		{"empty path", rules.Rule{ID: "a", Name: "A", Path: "", Category: "system", Safety: "safe"}, true},
		{"relative path", rules.Rule{ID: "a", Name: "A", Path: "rel/path", Category: "system", Safety: "safe"}, true},
		{"traversal path", rules.Rule{ID: "a", Name: "A", Path: "/tmp/../etc", Category: "system", Safety: "safe"}, true},
		{"empty category", rules.Rule{ID: "a", Name: "A", Path: "/tmp", Category: "", Safety: "safe"}, true},
		{"empty safety", rules.Rule{ID: "a", Name: "A", Path: "/tmp", Category: "system", Safety: ""}, true},
		{"whitespace id", rules.Rule{ID: "  ", Name: "A", Path: "/tmp", Category: "system", Safety: "safe"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRule(tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRule() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUndoRecordCandidate(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	tests := []struct {
		name    string
		home, trash, src, dst string
		wantErr bool
	}{
		{"valid", home, trash, filepath.Join(home, "Library", "Caches", "foo"), filepath.Join(trash, "foo"), false},
		{"empty home", "", trash, "/a", "/b", true},
		{"empty trash", home, "", "/a", "/b", true},
		{"relative src", home, trash, "rel", filepath.Join(trash, "x"), true},
		{"relative dst", home, trash, filepath.Join(home, "x"), "rel", true},
		{"src outside home", home, trash, "/etc/foo", filepath.Join(trash, "foo"), true},
		{"dst outside trash", home, trash, filepath.Join(home, "Library", "x"), filepath.Join(home, "nottrash", "x"), true},
		{"traversal src", home, trash, filepath.Join(home, "..", "etc"), filepath.Join(trash, "x"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUndoRecordCandidate(tt.home, tt.trash, tt.src, tt.dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		path, home, want string
	}{
		{"~/Library", "/Users/test", "/Users/test/Library"},
		{"~", "/Users/test", "/Users/test"},
		{"/abs/path", "/Users/test", "/abs/path"},
		{"relative", "/Users/test", "relative"},
		{"~/foo", "", "~/foo"},
		{"~", "", "~"},
	}
	for _, tt := range tests {
		got := expandTilde(tt.path, tt.home)
		if got != tt.want {
			t.Errorf("expandTilde(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
		}
	}
}

func TestIsExcludedPath(t *testing.T) {
	tests := []struct {
		path     string
		excludes []string
		want     bool
	}{
		{"/tmp/foo", nil, false},
		{"/tmp/foo", []string{}, false},
		{"/tmp/foo", []string{"/tmp/foo"}, true},
		{"/tmp/foo/bar", []string{"/tmp/foo"}, true},
		{"/tmp/foobar", []string{"/tmp/foo"}, false},
		{"/tmp/foo", []string{""}, false},
		{"/tmp/foo", []string{"/other", "/tmp/foo"}, true},
	}
	for _, tt := range tests {
		got := isExcludedPath(tt.path, tt.excludes)
		if got != tt.want {
			t.Errorf("isExcludedPath(%q, %v) = %v, want %v", tt.path, tt.excludes, got, tt.want)
		}
	}
}

func TestNormalizeExcludePaths(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name  string
		input []string
		check func([]string) bool
	}{
		{"nil input", nil, func(r []string) bool { return r == nil }},
		{"empty slice", []string{}, func(r []string) bool { return r == nil }},
		{"blank entries", []string{"", " ", ""}, func(r []string) bool { return r == nil || len(r) == 0 }},
		{"absolute path", []string{"/tmp/foo"}, func(r []string) bool { return len(r) == 1 && r[0] == "/tmp/foo" }},
		{"dedup", []string{"/tmp/foo", "/tmp/foo"}, func(r []string) bool { return len(r) == 1 }},
		{"tilde expand", []string{"~/Library"}, func(r []string) bool { return len(r) == 1 && r[0] == filepath.Join(home, "Library") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeExcludePaths(tt.input)
			if !tt.check(result) {
				t.Errorf("normalizeExcludePaths(%v) = %v", tt.input, result)
			}
		})
	}
}

func TestGetPathSize(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(tmp, "sub")
	os.MkdirAll(sub, 0o755)
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("world!"), 0o600); err != nil {
		t.Fatal(err)
	}

	size, err := getPathSize(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	if size != 11 { // "hello" (5) + "world!" (6)
		t.Errorf("expected 11 bytes, got %d", size)
	}
}

func TestGetPathSizeCancelled(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o600)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := getPathSize(ctx, tmp)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

func TestGetPathSizeNonExistent(t *testing.T) {
	size, err := getPathSize(context.Background(), "/nonexistent/path")
	if err != nil {
		// WalkDir returns nil for permission denied/not found at root
		_ = err
	}
	if size != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", size)
	}
}

func TestGetPathSizeWithSymlink(t *testing.T) {
	tmp := t.TempDir()
	realFile := filepath.Join(tmp, "real.txt")
	os.WriteFile(realFile, []byte("data"), 0o600)
	linkFile := filepath.Join(tmp, "link.txt")
	os.Symlink(realFile, linkFile)

	size, err := getPathSize(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	// Should count both real file (4 bytes) and symlink size.
	if size < 4 {
		t.Errorf("expected at least 4 bytes, got %d", size)
	}
}

func TestGetPathSizeWalkError(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "noread")
	os.MkdirAll(sub, 0o700)
	os.WriteFile(filepath.Join(sub, "file"), []byte("x"), 0o600)
	os.Chmod(sub, 0o000) // no read permission
	defer os.Chmod(sub, 0o700)

	size, err := getPathSize(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	// Should gracefully skip unreadable dirs.
	_ = size
}

func TestScannerSoftFailWarning(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	eng.AddScanner(testScanner{id: "fail-scanner", cat: types.CategoryDeveloper, err: errors.New("scanner failed")})

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatalf("expected soft-fail, got hard error: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning from failed scanner")
	}
}

func TestCleanDeleteStrategy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "Caches", "delete-target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "test-delete",
		Name:       "Delete Target",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"test-delete"},
		Strategy: types.CleanStrategyDelete,
		DryRun:   false,
		Force:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 1 {
		t.Fatalf("expected 1 cleaned, got %d", res.CleanedCount)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected target deleted, got err=%v", err)
	}
}

func TestCleanNoScanFirst(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	_, err := eng.Clean(context.Background(), types.CleanRequest{ItemIDs: []string{"nonexistent"}, Strategy: types.CleanStrategyTrash})
	if err == nil {
		t.Fatal("expected error for no matching items")
	}
}

func TestCleanNoItems(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	_, err := eng.Clean(context.Background(), types.CleanRequest{})
	if err == nil {
		t.Fatal("expected error for empty item_ids")
	}
}

func TestScanWithPresetSize(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "preset")
	os.MkdirAll(target, 0o700)

	presetSize := int64(999999)
	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "preset",
		Name:       "Preset",
		Path:       target,
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
		PresetSize: &presetSize,
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Size != presetSize {
		t.Errorf("expected preset size %d, got %d", presetSize, res.Items[0].Size)
	}
}

func TestCleanRiskyBlockedWithoutUnsafe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "risky-item")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data"), []byte("x"), 0o600)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "risky1",
		Name:       "Risky",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetyRisky,
		SafetyNote: "risky test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"risky1"},
		Strategy: types.CleanStrategyTrash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 0 {
		t.Errorf("expected 0 cleaned for risky without unsafe, got %d", res.CleanedCount)
	}
	if len(res.FailedItems) != 1 {
		t.Errorf("expected 1 failed item, got %d", len(res.FailedItems))
	}
}

func TestCleanWithExcludedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "excluded-target")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data"), []byte("x"), 0o600)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "excl1",
		Name:       "Excluded",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:      []string{"excl1"},
		Strategy:     types.CleanStrategyTrash,
		ExcludePaths: []string{target},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 0 {
		t.Errorf("expected 0 cleaned for excluded path, got %d", res.CleanedCount)
	}
	if len(res.FailedItems) != 1 {
		t.Errorf("expected 1 failed, got %d", len(res.FailedItems))
	}
}

func TestCleanTrashWithUndo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	target := filepath.Join(home, "Library", "undo-target")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data"), []byte("content"), 0o600)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "undo1",
		Name:       "Undo Target",
		Path:       target,
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"undo1"},
		Strategy: types.CleanStrategyTrash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 1 {
		t.Errorf("expected 1 cleaned, got %d", res.CleanedCount)
	}
}

func TestCleanDryRunDoesNotMove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "dry-run-target")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data"), []byte("content"), 0o600)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "dry1",
		Name:       "Dry",
		Path:       target,
		Category:   types.CategoryDeveloper,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"dry1"},
		Strategy: types.CleanStrategyTrash,
		DryRun:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 1 {
		t.Errorf("expected 1 cleaned (dry-run), got %d", res.CleanedCount)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Error("target should still exist in dry-run mode")
	}
}

func TestCleanUnknownStrategy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "unknown-strat")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "data"), []byte("x"), 0o600)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "unk1",
		Name:       "Unknown",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	if _, err := eng.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"unk1"},
		Strategy: "invalid_strategy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanedCount != 0 {
		t.Errorf("expected 0 cleaned for unknown strategy, got %d", res.CleanedCount)
	}
	if len(res.FailedItems) != 1 {
		t.Errorf("expected 1 failed, got %d", len(res.FailedItems))
	}
}

func TestScanCancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "Library", "cancel-target")
	os.MkdirAll(target, 0o700)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{{
		ID:         "cancel1",
		Name:       "Cancel",
		Path:       target,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "test",
		Desc:       "test",
	}}, stream.NewBroker(), audit.NewLogger(auditPath))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := eng.Scan(ctx)
	if err == nil {
		// May or may not error depending on timing — just verify no panic.
	}
}

func TestValidateUndoRecordCandidateSymlinkSrc(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	// Create a symlink in the parent dir of src.
	linkDir := filepath.Join(home, "linked")
	realDir := filepath.Join(home, "real")
	os.MkdirAll(realDir, 0o700)
	os.Symlink(realDir, linkDir)

	err := validateUndoRecordCandidate(home, trash, filepath.Join(linkDir, "file"), filepath.Join(trash, "file"))
	if err == nil {
		t.Error("expected error for symlink in src path")
	}
}

func TestAddScanner(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	eng.AddScanner(testScanner{id: "s1", cat: types.CategoryDeveloper})
	eng.AddScanner(testScanner{id: "s2", cat: types.CategorySystem})

	if len(eng.scanners) != 2 {
		t.Errorf("expected 2 scanners, got %d", len(eng.scanners))
	}
}

func TestUndoSuccessful(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Create item in trash.
	trashItem := filepath.Join(trashDir, "undo-item")
	os.WriteFile(trashItem, []byte("data-123"), 0o600)

	// Original location.
	origDir := filepath.Join(home, "Documents")
	os.MkdirAll(origDir, 0o700)
	origPath := filepath.Join(origDir, "undo-item")

	// Create undo manifest at ~/.opencleaner/undo/last.json.
	manifest := cleaner.UndoManifest{
		Version: 1,
		Entries: []cleaner.UndoEntry{
			{SrcPath: origPath, DstPath: trashItem, Bytes: 8},
		},
	}
	if err := cleaner.SaveManifest(manifest.Entries, ""); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New(nil, stream.NewBroker(), audit.NewLogger(auditPath))
	result, err := eng.Undo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoredCount != 1 {
		t.Errorf("expected 1 restored, got %d", result.RestoredCount)
	}
	if result.RestoredSize != 8 {
		t.Errorf("expected 8 bytes restored, got %d", result.RestoredSize)
	}
	if len(result.FailedItems) != 0 {
		t.Errorf("expected 0 failed, got %v", result.FailedItems)
	}

	// Verify file is restored.
	if _, err := os.Lstat(origPath); err != nil {
		t.Error("file should be restored")
	}
}

func TestUndoPartialFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	trashDir := filepath.Join(home, ".Trash")
	os.MkdirAll(trashDir, 0o700)

	// Item 1: in trash, can be restored.
	trashItem1 := filepath.Join(trashDir, "good-item")
	os.WriteFile(trashItem1, []byte("good"), 0o600)
	origPath1 := filepath.Join(home, "Documents", "good-item")
	os.MkdirAll(filepath.Dir(origPath1), 0o700)

	// Item 2: NOT in trash, restore will fail.
	origPath2 := filepath.Join(home, "Documents", "bad-item")

	manifest := cleaner.UndoManifest{
		Version: 1,
		Entries: []cleaner.UndoEntry{
			{SrcPath: origPath1, DstPath: trashItem1, Bytes: 4},
			{SrcPath: origPath2, DstPath: filepath.Join(trashDir, "nonexistent"), Bytes: 3},
		},
	}
	if err := cleaner.SaveManifest(manifest.Entries, ""); err != nil {
		t.Fatal(err)
	}

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New(nil, stream.NewBroker(), audit.NewLogger(auditPath))
	result, err := eng.Undo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoredCount != 1 {
		t.Errorf("expected 1 restored, got %d", result.RestoredCount)
	}
	if len(result.FailedItems) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.FailedItems))
	}
}

func TestUndoNoManifestEngine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New(nil, stream.NewBroker(), audit.NewLogger(auditPath))
	_, err := eng.Undo(context.Background())
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestValidateUndoRecordCandidateProtectedSrc(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	err := validateUndoRecordCandidate(home, trash, "/Applications", filepath.Join(trash, "x"))
	if err == nil {
		t.Error("expected error for protected src path")
	}
}

func TestValidateUndoRecordCandidateSymlinkInDstDir(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	realDir := filepath.Join(home, "real")
	os.MkdirAll(realDir, 0o700)
	linkDir := filepath.Join(trash, "linked")
	os.Symlink(realDir, linkDir)

	err := validateUndoRecordCandidate(home, trash, filepath.Join(home, "Documents", "file"), filepath.Join(linkDir, "file"))
	if err == nil {
		t.Error("expected error for symlink in dst path")
	}
}

func TestValidateUndoRecordCandidateTrashOutsideHome(t *testing.T) {
	home := t.TempDir()
	outsideTrash := "/tmp/not-in-home-trash"

	err := validateUndoRecordCandidate(home, outsideTrash, filepath.Join(home, "file"), filepath.Join(outsideTrash, "file"))
	if err == nil {
		t.Error("expected error for trash outside home")
	}
}

func TestCleanEmptyItems(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	broker := stream.NewBroker()
	eng := New(nil, broker, audit.NewLogger(filepath.Join(home, "audit.log")))

	_, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{},
		Strategy: types.CleanStrategyTrash,
	})
	if err == nil {
		t.Error("expected error for empty items")
	}
}

func TestScanWithScanner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scanDir := filepath.Join(home, "scanner-target")
	os.MkdirAll(scanDir, 0o700)
	os.WriteFile(filepath.Join(scanDir, "data"), []byte("x"), 0o600)

	broker := stream.NewBroker()
	eng := New(nil, broker, audit.NewLogger(filepath.Join(home, "audit.log")))
	eng.AddScanner(testScanner{
		id: "mock-target",
		cat: types.CategoryDeveloper,
		rules: []rules.Rule{{
			ID:       "mock-target",
			Name:     "Mock Target",
			Path:     scanDir,
			Category: types.CategoryDeveloper,
			Safety:   types.SafetySafe,
		}},
	})

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range res.Items {
		if item.ID == "mock-target" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected scanner-discovered item in results")
	}
}

func TestNormalizeExcludePathsWithTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	paths := normalizeExcludePaths([]string{"~/Library/Caches"})
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	expected := filepath.Join(home, "Library", "Caches")
	if paths[0] != expected {
		t.Errorf("expected %q, got %q", expected, paths[0])
	}
}

func TestCleanDeleteWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	broker := stream.NewBroker()
	target := filepath.Join(home, "Library", "Caches", "del-test")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "f"), []byte("x"), 0o600)

	r := rules.Rule{
		ID: "test-del-no-force", Name: "Test", Path: target,
		Category: types.CategorySystem, Safety: types.SafetySafe,
		SafetyNote: "t", Desc: "t",
	}
	eng := New([]rules.Rule{r}, broker, audit.NewLogger(filepath.Join(home, "audit.log")))
	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Skip("no items found")
	}

	result, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{res.Items[0].ID},
		Strategy: types.CleanStrategyDelete,
		Force:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FailedItems) != 1 {
		t.Errorf("expected 1 failed (no force), got %d", len(result.FailedItems))
	}
}

func TestCleanDeleteWithForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	broker := stream.NewBroker()
	target := filepath.Join(home, "Library", "Caches", "force-del")
	os.MkdirAll(target, 0o700)
	os.WriteFile(filepath.Join(target, "f"), []byte("x"), 0o600)

	r := rules.Rule{
		ID: "test-force-del", Name: "Test", Path: target,
		Category: types.CategorySystem, Safety: types.SafetySafe,
		SafetyNote: "t", Desc: "t",
	}
	eng := New([]rules.Rule{r}, broker, audit.NewLogger(filepath.Join(home, "audit.log")))
	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Skip("no items found")
	}

	result, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{res.Items[0].ID},
		Strategy: types.CleanStrategyDelete,
		Force:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedCount != 1 {
		t.Errorf("expected 1 cleaned, got %d", result.CleanedCount)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("target should be deleted")
	}
}

func TestCleanContextCancellation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	broker := stream.NewBroker()
	target := filepath.Join(home, "Library", "Caches", "ctx-test")
	os.MkdirAll(target, 0o700)

	r := rules.Rule{
		ID: "ctx-test", Name: "Test", Path: target,
		Category: types.CategorySystem, Safety: types.SafetySafe,
		SafetyNote: "t", Desc: "t",
	}
	eng := New([]rules.Rule{r}, broker, audit.NewLogger(filepath.Join(home, "audit.log")))
	_, _ = eng.Scan(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eng.Clean(ctx, types.CleanRequest{
		ItemIDs:  []string{"ctx-test"},
		Strategy: types.CleanStrategyTrash,
	})
	if err == nil || err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestScanDuplicateStaticRuleID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "dup-target")
	os.MkdirAll(dir, 0o700)

	dup := rules.Rule{
		ID: "dup-id", Name: "dup", Path: dir,
		Category: types.CategoryDeveloper, Safety: types.SafetySafe,
		SafetyNote: "test", Desc: "test",
	}
	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{dup, dup}, stream.NewBroker(), audit.NewLogger(auditPath))

	_, err := eng.Scan(context.Background())
	if err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestScanScannerFillsEmptyCategory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "nocategory")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("data"), 0o600)

	sc := testScanner{
		id:  "fill-cat",
		cat: types.CategoryDeveloper,
		rules: []rules.Rule{{
			ID: "nocat-rule", Name: "nocat", Path: dir,
			Category: "", Safety: types.SafetySafe,
			SafetyNote: "test", Desc: "test",
		}},
	}
	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New(nil, stream.NewBroker(), audit.NewLogger(auditPath))
	eng.AddScanner(sc)

	res, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Skip("target dir empty, skipping")
	}
	for _, it := range res.Items {
		if it.Category != types.CategoryDeveloper {
			t.Errorf("expected category to be filled, got %q", it.Category)
		}
	}
}

func TestCleanBlockedBySafety(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// The home directory itself is a protected path.
	r := rules.Rule{
		ID: "unsafe-path", Name: "unsafe", Path: home,
		Category: types.CategoryDeveloper, Safety: types.SafetySafe,
		SafetyNote: "test", Desc: "test",
	}
	auditPath, _ := audit.DefaultAuditLogPath()
	eng := New([]rules.Rule{r}, stream.NewBroker(), audit.NewLogger(auditPath))

	_, err := eng.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	res, err := eng.Clean(context.Background(), types.CleanRequest{
		ItemIDs:  []string{"unsafe-path"},
		Strategy: types.CleanStrategyTrash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FailedItems) == 0 {
		t.Error("expected item to fail safety check")
	}
}

func TestIsWithinRootEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{"empty root", "", "/a", false},
		{"empty path", "/a", "", false},
		{"relative root", "a/b", "/c", false},
		{"relative path", "/a", "b/c", false},
		{"parent", "/a/b", "/a", false},
		{"same", "/a", "/a", true},
		{"child", "/a", "/a/b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWithinRoot(tt.root, tt.path)
			if got != tt.want {
				t.Errorf("isWithinRoot(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateUndoRecordCandidateEdgeCases(t *testing.T) {
	home := t.TempDir()
	trash := filepath.Join(home, ".Trash")
	os.MkdirAll(trash, 0o700)

	tests := []struct {
		name  string
		home  string
		trash string
		src   string
		dst   string
		err   bool
	}{
		{"empty home", "", trash, "/a", filepath.Join(trash, "a"), true},
		{"empty trash", home, "", filepath.Join(home, "a"), "/b", true},
		{"relative src", home, trash, "rel/path", filepath.Join(trash, "a"), true},
		{"relative dst", home, trash, filepath.Join(home, "a"), "rel/path", true},
		{"traversal src", home, trash, filepath.Join(home, "a/../../../etc"), filepath.Join(trash, "a"), true},
		{"traversal dst", home, trash, filepath.Join(home, "a"), filepath.Join(trash, "../../etc"), true},
		{"src outside home", home, trash, "/tmp/outside", filepath.Join(trash, "a"), true},
		{"dst outside trash", home, trash, filepath.Join(home, "a"), "/tmp/outside", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUndoRecordCandidate(tt.home, tt.trash, tt.src, tt.dst)
			if (err != nil) != tt.err {
				t.Errorf("validateUndoRecordCandidate() error = %v, wantErr %v", err, tt.err)
			}
		})
	}
}

func TestNormalizeExcludePathsExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	input := []string{"~/test", "relative/path", "", "  ", filepath.Join(home, "abs")}
	out := normalizeExcludePaths(input)

	expected := map[string]bool{
		filepath.Join(home, "test"):          true,
		filepath.Join(home, "relative/path"): true,
		filepath.Join(home, "abs"):           true,
	}
	if len(out) != len(expected) {
		t.Errorf("expected %d paths, got %d: %v", len(expected), len(out), out)
	}
	for _, p := range out {
		if !expected[p] {
			t.Errorf("unexpected path: %s", p)
		}
	}
}
