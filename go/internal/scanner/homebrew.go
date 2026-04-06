package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type HomebrewScanner struct {
	home string
}

func NewHomebrewScanner(home string) *HomebrewScanner {
	return &HomebrewScanner{home: home}
}

func (s *HomebrewScanner) ID() string               { return "homebrew" }
func (s *HomebrewScanner) Name() string              { return "Homebrew" }
func (s *HomebrewScanner) Category() types.Category  { return types.CategoryDeveloper }

func (s *HomebrewScanner) Scan(ctx context.Context) ([]rules.Rule, error) {
	var found []rules.Rule

	// Known cache paths — homebrew-cache reuses builtin ID for dedup.
	knownPaths := []struct {
		id, name, path, note string
		safety               types.SafetyLevel
	}{
		{
			"homebrew-cache",
			"Homebrew cache",
			filepath.Join(s.home, "Library", "Caches", "Homebrew"),
			"Download cache; brew re-downloads as needed",
			types.SafetySafe,
		},
		{
			"homebrew-logs",
			"Homebrew logs",
			filepath.Join(s.home, "Library", "Logs", "Homebrew"),
			"Build logs; safe to remove",
			types.SafetySafe,
		},
	}

	for _, kp := range knownPaths {
		if _, err := os.Lstat(kp.path); err != nil {
			continue
		}
		found = append(found, rules.Rule{
			ID:         kp.id,
			Name:       kp.name,
			Path:       kp.path,
			Category:   types.CategoryDeveloper,
			Safety:     kp.safety,
			SafetyNote: kp.note,
			Desc:       kp.note,
		})
	}

	// Use brew cleanup --dry-run to find outdated downloads.
	out, err := RunCommand(ctx, "brew", "cleanup", "--dry-run")
	if err != nil || out == "" {
		return found, nil // brew not installed or no output
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	cleanableCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "==>") {
			cleanableCount++
		}
	}

	if cleanableCount > 0 {
		// Create a single rule representing old versions.
		// We point to the Homebrew cache dir for existence/size — actual
		// cleanup would use `brew cleanup` (deferred to Phase 3 CleanCmd).
		cachePath := filepath.Join(s.home, "Library", "Caches", "Homebrew")
		if _, err := os.Lstat(cachePath); err == nil {
			found = append(found, rules.Rule{
				ID:         "homebrew-old-versions",
				Name:       "Homebrew outdated downloads",
				Path:       cachePath,
				Category:   types.CategoryDeveloper,
				Safety:     types.SafetyModerate,
				SafetyNote: "Outdated formula/cask downloads; run `brew cleanup` to remove",
				Desc:       formatCount(cleanableCount, "outdated item"),
			})
		}
	}

	return found, nil
}

func formatCount(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
