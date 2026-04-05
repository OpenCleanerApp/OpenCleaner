package rules

import (
	"path/filepath"

	"github.com/opencleaner/opencleaner/pkg/types"
)

// BuiltinRules is a conservative MVP set. Do NOT add new targets without validating against the PRD and
// clmm-clean-my-mac-cli reference paths.
func BuiltinRules(home string) []Rule {
	p := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }

	return []Rule{
		{
			ID:         "xcode-derived-data",
			Name:       "Xcode DerivedData",
			Path:       p("Library", "Developer", "Xcode", "DerivedData"),
			Category:   types.CategoryDeveloper,
			Safety:     types.SafetyModerate,
			SafetyNote: "Xcode will rebuild DerivedData; first build may be slower.",
		},
		{
			ID:         "xcode-archives",
			Name:       "Xcode Archives",
			Path:       p("Library", "Developer", "Xcode", "Archives"),
			Category:   types.CategoryDeveloper,
			Safety:     types.SafetyModerate,
			SafetyNote: "Deleting archives removes old builds; ensure you don’t need them.",
		},
		{
			ID:         "npm-cache",
			Name:       "npm Cache",
			Path:       p(".npm", "_cacache"),
			Category:   types.CategoryDeveloper,
			Safety:     types.SafetySafe,
			SafetyNote: "Safe; npm will re-download packages.",
		},
		{
			ID:         "yarn-cache",
			Name:       "Yarn Cache",
			Path:       p("Library", "Caches", "Yarn"),
			Category:   types.CategoryDeveloper,
			Safety:     types.SafetySafe,
			SafetyNote: "Safe; Yarn will re-download packages.",
		},
	}
}
