package analyzer

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/opencleaner/opencleaner/pkg/types"
)

// SuggestionEngine generates smart suggestions from scan results.
type SuggestionEngine struct {
	rules []suggestionRule
	now   time.Time
}

type suggestionRule struct {
	match    func(item types.ScanItem) bool
	generate func(item types.ScanItem) *types.Suggestion
}

// New creates a SuggestionEngine with built-in rules.
func New() *SuggestionEngine {
	e := &SuggestionEngine{now: time.Now()}
	e.registerBuiltinRules()
	return e
}

// Analyze returns prioritized suggestions for the given scan items.
// Returns at most maxSuggestions results sorted by priority descending.
func (e *SuggestionEngine) Analyze(items []types.ScanItem) []types.Suggestion {
	var suggestions []types.Suggestion
	for _, item := range items {
		for _, rule := range e.rules {
			if rule.match(item) {
				if s := rule.generate(item); s != nil {
					suggestions = append(suggestions, *s)
				}
			}
		}
	}

	// Aggregate quick wins (multiple __pycache__ or node_modules).
	suggestions = append(suggestions, e.aggregateQuickWins(items)...)

	// Deduplicate by ItemID.
	seen := map[string]struct{}{}
	deduped := make([]types.Suggestion, 0, len(suggestions))
	for _, s := range suggestions {
		if _, ok := seen[s.ItemID]; ok {
			continue
		}
		seen[s.ItemID] = struct{}{}
		deduped = append(deduped, s)
	}

	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Priority > deduped[j].Priority
	})
	if len(deduped) > 10 {
		deduped = deduped[:10]
	}
	return deduped
}

func (e *SuggestionEngine) registerBuiltinRules() {
	// Large & Old: >1GB and >30 days.
	e.rules = append(e.rules, suggestionRule{
		match: func(it types.ScanItem) bool {
			return it.Size > 1<<30 && e.ageDays(it) > 30
		},
		generate: func(it types.ScanItem) *types.Suggestion {
			return &types.Suggestion{
				ItemID:      it.ID,
				Message:     fmt.Sprintf("%s: %s unused for %d days", it.Name, humanSize(it.Size), e.ageDays(it)),
				Priority:    e.calcPriority(it),
				Rationale:   "Large item unused for over a month",
				SafetyLevel: it.SafetyLevel,
			}
		},
	})

	// Docker heavy: any Docker item > 5GB.
	e.rules = append(e.rules, suggestionRule{
		match: func(it types.ScanItem) bool {
			return strings.HasPrefix(it.ID, "docker-") && it.Size > 5<<30
		},
		generate: func(it types.ScanItem) *types.Suggestion {
			return &types.Suggestion{
				ItemID:      it.ID,
				Message:     fmt.Sprintf("Docker using %s — run cleanup to reclaim space", humanSize(it.Size)),
				Priority:    clamp(float64(it.Size)/(10<<30)*0.5 + 0.5),
				Rationale:   "Docker artifacts consuming significant disk space",
				SafetyLevel: it.SafetyLevel,
			}
		},
	})

	// DerivedData stale: >14 days old.
	e.rules = append(e.rules, suggestionRule{
		match: func(it types.ScanItem) bool {
			return it.ID == "xcode-derived-data" && e.ageDays(it) > 14
		},
		generate: func(it types.ScanItem) *types.Suggestion {
			return &types.Suggestion{
				ItemID:      it.ID,
				Message:     fmt.Sprintf("DerivedData %d days old, %s — rebuild takes ~2min", e.ageDays(it), humanSize(it.Size)),
				Priority:    0.70,
				Rationale:   "Stale build artifacts that Xcode regenerates",
				SafetyLevel: types.SafetySafe,
			}
		},
	})

	// Simulator runtimes.
	e.rules = append(e.rules, suggestionRule{
		match: func(it types.ScanItem) bool {
			return it.ID == "xcode-simulator-runtimes" && it.Size > 1<<30
		},
		generate: func(it types.ScanItem) *types.Suggestion {
			return &types.Suggestion{
				ItemID:      it.ID,
				Message:     fmt.Sprintf("Simulator runtimes: %s — keep only versions you test against", humanSize(it.Size)),
				Priority:    clamp(float64(it.Size)/(20<<30)*0.4 + 0.3),
				Rationale:   "Simulator runtimes are large and re-downloadable",
				SafetyLevel: types.SafetyModerate,
			}
		},
	})

	// Homebrew cleanup.
	e.rules = append(e.rules, suggestionRule{
		match: func(it types.ScanItem) bool {
			return it.ID == "homebrew-old-versions"
		},
		generate: func(it types.ScanItem) *types.Suggestion {
			return &types.Suggestion{
				ItemID:      it.ID,
				Message:     fmt.Sprintf("Homebrew has outdated downloads — run `brew cleanup` to reclaim space"),
				Priority:    0.50,
				Rationale:   "Outdated downloads no longer needed",
				SafetyLevel: types.SafetyModerate,
			}
		},
	})
}

// aggregateQuickWins creates aggregate suggestions for __pycache__ and node_modules.
func (e *SuggestionEngine) aggregateQuickWins(items []types.ScanItem) []types.Suggestion {
	type agg struct {
		count int
		total int64
	}
	groups := map[string]*agg{}

	for _, it := range items {
		var key string
		switch {
		case strings.HasPrefix(it.ID, "python-pycache-"):
			key = "__pycache__"
		case strings.HasPrefix(it.ID, "nodejs-node-modules-"):
			key = "node_modules"
		default:
			continue
		}
		if groups[key] == nil {
			groups[key] = &agg{}
		}
		groups[key].count++
		groups[key].total += it.Size
	}

	var out []types.Suggestion
	for name, g := range groups {
		if g.count < 2 {
			continue
		}
		out = append(out, types.Suggestion{
			ItemID:      "quickwin-" + name,
			Message:     fmt.Sprintf("%d %s dirs totaling %s — always safe to clean", g.count, name, humanSize(g.total)),
			Priority:    0.60,
			Rationale:   "Multiple regenerable directories found",
			SafetyLevel: types.SafetySafe,
		})
	}
	return out
}

func (e *SuggestionEngine) ageDays(it types.ScanItem) int {
	if it.LastAccess == nil {
		return 0
	}
	d := e.now.Sub(*it.LastAccess)
	if d < 0 {
		return 0
	}
	return int(d.Hours() / 24)
}

// calcPriority: size(40%) + safety(35%) + age(25%).
func (e *SuggestionEngine) calcPriority(it types.ScanItem) float64 {
	sizeWeight := math.Min(float64(it.Size)/(10<<30), 1.0) * 0.40
	safetyScore := 0.2
	switch it.SafetyLevel {
	case types.SafetySafe:
		safetyScore = 1.0
	case types.SafetyModerate:
		safetyScore = 0.6
	}
	safetyWeight := safetyScore * 0.35
	ageWeight := math.Min(float64(e.ageDays(it))/90.0, 1.0) * 0.25
	return clamp(sizeWeight + safetyWeight + ageWeight)
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func humanSize(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
