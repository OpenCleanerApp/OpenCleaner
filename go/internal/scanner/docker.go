package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/pkg/types"
)

type DockerScanner struct{}

func NewDockerScanner() *DockerScanner {
	return &DockerScanner{}
}

func (s *DockerScanner) ID() string               { return "docker" }
func (s *DockerScanner) Name() string              { return "Docker" }
func (s *DockerScanner) Category() types.Category  { return types.CategoryDeveloper }

// dockerDFRow represents one row from `docker system df --format json`.
type dockerDFRow struct {
	Type        string `json:"Type"`
	TotalCount  int    `json:"TotalCount"`
	Size        string `json:"Size"`
	Reclaimable string `json:"Reclaimable"`
}

func (s *DockerScanner) Scan(ctx context.Context) ([]rules.Rule, error) {
	// Check Docker Desktop data dir for macOS.
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, "Library", "Containers", "com.docker.docker", "Data")
	if _, err := os.Lstat(dataDir); err != nil {
		return nil, nil // Docker Desktop not installed
	}

	out, err := RunCommand(ctx, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil // docker binary not found
	}

	var found []rules.Rule
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row dockerDFRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}

		size := parseDockerSize(row.Reclaimable)
		if size <= 0 {
			size = parseDockerSize(row.Size)
		}
		if size <= 0 {
			continue
		}

		var id, name, note string
		var safety types.SafetyLevel

		switch strings.ToLower(row.Type) {
		case "images":
			id = "docker-images"
			name = "Docker images (reclaimable)"
			note = "Unused images; docker pull re-downloads as needed"
			safety = types.SafetyModerate
		case "containers":
			continue // don't suggest cleaning active containers
		case "local volumes":
			id = "docker-volumes"
			name = "Docker volumes (reclaimable)"
			note = "Dangling volumes may contain data; review before cleaning"
			safety = types.SafetyRisky
		case "build cache":
			id = "docker-build-cache"
			name = "Docker build cache"
			note = "Build layer cache; rebuilds are slower without it"
			safety = types.SafetySafe
		default:
			continue
		}

		sizePtr := new(int64)
		*sizePtr = size
		found = append(found, rules.Rule{
			ID:         id,
			Name:       name,
			Path:       dataDir,
			Category:   types.CategoryDeveloper,
			Safety:     safety,
			SafetyNote: note,
			Desc:       name,
			PresetSize: sizePtr,
		})
	}

	return found, nil
}

// parseDockerSize parses human-readable sizes like "2.5GB", "100MB", "1.2kB".
func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	// Strip reclaim percentage like "2.5GB (100%)" → "2.5GB"
	if idx := strings.Index(s, "("); idx > 0 {
		s = strings.TrimSpace(s[:idx])
	}

	multipliers := []struct {
		suffix string
		mult   float64
	}{
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"kB", 1e3},
		{"B", 1},
	}
	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			numStr := strings.TrimSpace(strings.TrimSuffix(s, m.suffix))
			var val float64
			if _, err := parseFloat(numStr); err == nil {
				val, _ = parseFloat(numStr)
				return int64(val * m.mult)
			}
		}
	}
	return 0
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
