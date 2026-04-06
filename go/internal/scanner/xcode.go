package scanner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type XcodeScanner struct {
	home string
}

func NewXcodeScanner(home string) *XcodeScanner {
	return &XcodeScanner{home: home}
}

func (s *XcodeScanner) ID() string               { return "xcode" }
func (s *XcodeScanner) Name() string              { return "Xcode (deep)" }
func (s *XcodeScanner) Category() types.Category  { return types.CategoryDeveloper }

func (s *XcodeScanner) Scan(ctx context.Context) ([]rules.Rule, error) {
	var found []rules.Rule
	devDir := filepath.Join(s.home, "Library", "Developer")

	// Known large targets not covered by builtin rules.
	targets := []struct {
		id, name, relPath, note string
		safety                  types.SafetyLevel
	}{
		{
			"xcode-simulator-runtimes",
			"Simulator runtimes",
			filepath.Join("CoreSimulator", "Profiles", "Runtimes"),
			"Xcode re-downloads runtimes on demand; 5-10 GB each",
			types.SafetyModerate,
		},
		{
			"xcode-simulator-devices",
			"Simulator devices",
			filepath.Join("CoreSimulator", "Devices"),
			"Recreated by Xcode; contains app data for simulators",
			types.SafetyModerate,
		},
		{
			"xcode-watchos-device-support",
			"watchOS DeviceSupport",
			filepath.Join("Xcode", "watchOS DeviceSupport"),
			"Re-downloaded on first debug session with watch",
			types.SafetyModerate,
		},
		{
			"xcode-tvos-device-support",
			"tvOS DeviceSupport",
			filepath.Join("Xcode", "tvOS DeviceSupport"),
			"Re-downloaded on first debug session with Apple TV",
			types.SafetyModerate,
		},
		{
			"xcode-previews",
			"SwiftUI Previews cache",
			filepath.Join("Xcode", "UserData", "Previews"),
			"Rebuilt automatically by Xcode on next preview",
			types.SafetySafe,
		},
	}

	for _, t := range targets {
		p := filepath.Join(devDir, t.relPath)
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		found = append(found, rules.Rule{
			ID:         t.id,
			Name:       t.name,
			Path:       p,
			Category:   types.CategoryDeveloper,
			Safety:     t.safety,
			SafetyNote: t.note,
			Desc:       t.note,
		})
	}

	// Per-version iOS DeviceSupport directories — each can be 2-5 GB.
	iosDS := filepath.Join(devDir, "Xcode", "iOS DeviceSupport")
	entries, err := os.ReadDir(iosDS)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(iosDS, e.Name())
			found = append(found, rules.Rule{
				ID:         "xcode-ios-device-support-" + pathHash(p),
				Name:       "iOS DeviceSupport " + e.Name(),
				Path:       p,
				Category:   types.CategoryDeveloper,
				Safety:     types.SafetyModerate,
				SafetyNote: "Re-downloaded on first debug for this iOS version",
				Desc:       "Debug symbols for iOS " + e.Name(),
			})
		}
	}

	return found, nil
}
