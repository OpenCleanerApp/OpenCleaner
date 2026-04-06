package analyzer

import (
	"testing"
	"time"

	"github.com/opencleaner/opencleaner/pkg/types"
)

func TestAnalyzeEmpty(t *testing.T) {
	e := New()
	suggestions := e.Analyze(nil)
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for nil items, got %d", len(suggestions))
	}
}

func TestLargeAndOldSuggestion(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago
	items := []types.ScanItem{
		{
			ID:          "xcode-derived-data",
			Name:        "DerivedData",
			Size:        2 << 30, // 2 GB
			SafetyLevel: types.SafetySafe,
			LastAccess:  &old,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "xcode-derived-data" {
			found = true
			if s.Priority <= 0 {
				t.Error("expected positive priority")
			}
		}
	}
	if !found {
		t.Error("expected suggestion for large+old item")
	}
}

func TestDockerHeavySuggestion(t *testing.T) {
	items := []types.ScanItem{
		{
			ID:          "docker-images",
			Name:        "Docker images",
			Size:        8 << 30, // 8 GB
			SafetyLevel: types.SafetyModerate,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "docker-images" {
			found = true
			if s.Priority < 0.5 {
				t.Errorf("expected priority >= 0.5 for 8GB Docker, got %.2f", s.Priority)
			}
		}
	}
	if !found {
		t.Error("expected Docker heavy suggestion")
	}
}

func TestDerivedDataStaleSuggestion(t *testing.T) {
	old := time.Now().Add(-30 * 24 * time.Hour)
	items := []types.ScanItem{
		{
			ID:          "xcode-derived-data",
			Name:        "DerivedData",
			Size:        500 << 20, // 500 MB
			SafetyLevel: types.SafetySafe,
			LastAccess:  &old,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "xcode-derived-data" && s.Priority == 0.70 {
			found = true
		}
	}
	if !found {
		t.Error("expected DerivedData stale suggestion at priority 0.70")
	}
}

func TestSimRuntimesSuggestion(t *testing.T) {
	items := []types.ScanItem{
		{
			ID:          "xcode-simulator-runtimes",
			Name:        "Simulator runtimes",
			Size:        15 << 30, // 15 GB
			SafetyLevel: types.SafetyModerate,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "xcode-simulator-runtimes" {
			found = true
		}
	}
	if !found {
		t.Error("expected simulator runtimes suggestion")
	}
}

func TestHomebrewSuggestion(t *testing.T) {
	items := []types.ScanItem{
		{
			ID:          "homebrew-old-versions",
			Name:        "Homebrew outdated",
			Size:        100 << 20,
			SafetyLevel: types.SafetyModerate,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "homebrew-old-versions" {
			found = true
			if s.Priority != 0.50 {
				t.Errorf("expected priority 0.50, got %.2f", s.Priority)
			}
		}
	}
	if !found {
		t.Error("expected Homebrew suggestion")
	}
}

func TestQuickWinsAggregatePycache(t *testing.T) {
	items := []types.ScanItem{
		{ID: "python-pycache-aaa", Name: "pycache 1", Size: 5 << 20, SafetyLevel: types.SafetySafe},
		{ID: "python-pycache-bbb", Name: "pycache 2", Size: 3 << 20, SafetyLevel: types.SafetySafe},
		{ID: "python-pycache-ccc", Name: "pycache 3", Size: 2 << 20, SafetyLevel: types.SafetySafe},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "quickwin-__pycache__" {
			found = true
			if s.Priority != 0.60 {
				t.Errorf("expected priority 0.60, got %.2f", s.Priority)
			}
		}
	}
	if !found {
		t.Error("expected quick win aggregate for pycache")
	}
}

func TestQuickWinsAggregateNodeModules(t *testing.T) {
	items := []types.ScanItem{
		{ID: "nodejs-node-modules-aaa", Size: 200 << 20, SafetyLevel: types.SafetySafe},
		{ID: "nodejs-node-modules-bbb", Size: 150 << 20, SafetyLevel: types.SafetySafe},
	}

	e := New()
	suggestions := e.Analyze(items)

	found := false
	for _, s := range suggestions {
		if s.ItemID == "quickwin-node_modules" {
			found = true
		}
	}
	if !found {
		t.Error("expected quick win aggregate for node_modules")
	}
}

func TestQuickWinsNoAggregateForSingle(t *testing.T) {
	items := []types.ScanItem{
		{ID: "python-pycache-aaa", Size: 5 << 20, SafetyLevel: types.SafetySafe},
	}

	e := New()
	suggestions := e.Analyze(items)

	for _, s := range suggestions {
		if s.ItemID == "quickwin-__pycache__" {
			t.Error("should not aggregate quick win for single item")
		}
	}
}

func TestMaxSuggestions(t *testing.T) {
	old := time.Now().Add(-90 * 24 * time.Hour)
	items := make([]types.ScanItem, 20)
	for i := range items {
		items[i] = types.ScanItem{
			ID:          "docker-build-cache",
			Name:        "Large item",
			Size:        int64(i+1) << 30,
			SafetyLevel: types.SafetySafe,
			LastAccess:  &old,
		}
		// Give unique IDs to avoid dedup
		items[i].ID = "item-" + itoa(i)
	}

	e := New()
	suggestions := e.Analyze(items)

	if len(suggestions) > 10 {
		t.Errorf("expected max 10 suggestions, got %d", len(suggestions))
	}
}

func TestDeduplication(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour)
	// xcode-derived-data matches BOTH "large & old" and "derived data stale" rules
	items := []types.ScanItem{
		{
			ID:          "xcode-derived-data",
			Name:        "DerivedData",
			Size:        2 << 30,
			SafetyLevel: types.SafetySafe,
			LastAccess:  &old,
		},
	}

	e := New()
	suggestions := e.Analyze(items)

	count := 0
	for _, s := range suggestions {
		if s.ItemID == "xcode-derived-data" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 suggestion after dedup, got %d", count)
	}
}

func TestSortedByPriority(t *testing.T) {
	old := time.Now().Add(-90 * 24 * time.Hour)
	items := []types.ScanItem{
		{ID: "homebrew-old-versions", Size: 100 << 20, SafetyLevel: types.SafetyModerate},
		{ID: "docker-images", Size: 20 << 30, SafetyLevel: types.SafetyModerate},
		{ID: "big-item", Size: 5 << 30, SafetyLevel: types.SafetySafe, LastAccess: &old},
	}

	e := New()
	suggestions := e.Analyze(items)

	for i := 1; i < len(suggestions); i++ {
		if suggestions[i].Priority > suggestions[i-1].Priority {
			t.Errorf("suggestions not sorted by priority: [%d].Priority=%.2f > [%d].Priority=%.2f",
				i, suggestions[i].Priority, i-1, suggestions[i-1].Priority)
		}
	}
}

func TestCalcPriority(t *testing.T) {
	e := &SuggestionEngine{now: time.Now()}

	recent := time.Now().Add(-1 * time.Hour)
	old := time.Now().Add(-90 * 24 * time.Hour)

	// Small, safe, recent → low priority
	p1 := e.calcPriority(types.ScanItem{Size: 10 << 20, SafetyLevel: types.SafetySafe, LastAccess: &recent})
	// Large, safe, old → high priority
	p2 := e.calcPriority(types.ScanItem{Size: 10 << 30, SafetyLevel: types.SafetySafe, LastAccess: &old})
	// Large, risky, old → lower than safe
	p3 := e.calcPriority(types.ScanItem{Size: 10 << 30, SafetyLevel: types.SafetyRisky, LastAccess: &old})

	if p2 <= p1 {
		t.Errorf("large+old should have higher priority than small+recent: %.3f vs %.3f", p2, p1)
	}
	if p3 >= p2 {
		t.Errorf("risky should have lower priority than safe: %.3f vs %.3f", p3, p2)
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{2 * 1024 * 1024 * 1024, "2.0 GB"},
		{3 * 1024 * 1024 * 1024 * 1024, "3.0 TB"},
	}
	for _, tt := range tests {
		got := humanSize(tt.input)
		if got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClamp(t *testing.T) {
	if clamp(-0.5) != 0 {
		t.Error("clamp(-0.5) should be 0")
	}
	if clamp(0.5) != 0.5 {
		t.Error("clamp(0.5) should be 0.5")
	}
	if clamp(1.5) != 1 {
		t.Error("clamp(1.5) should be 1")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestAgeDaysFutureAccess(t *testing.T) {
	e := New()
	future := e.now.Add(24 * time.Hour)
	item := types.ScanItem{LastAccess: &future, Size: 100}
	days := e.ageDays(item)
	if days != 0 {
		t.Errorf("expected 0 days for future access, got %d", days)
	}
}

func TestCalcPriorityRiskySafety(t *testing.T) {
	e := New()
	item := types.ScanItem{
		Size:        1 << 20,
		SafetyLevel: types.SafetyRisky,
	}
	p := e.calcPriority(item)
	if p <= 0 {
		t.Errorf("expected positive priority for risky item, got %f", p)
	}
	// Risky safety should give lower priority than safe
	safeItem := types.ScanItem{
		Size:        1 << 20,
		SafetyLevel: types.SafetySafe,
	}
	sp := e.calcPriority(safeItem)
	if p >= sp {
		t.Errorf("risky priority %f should be less than safe priority %f", p, sp)
	}
}
