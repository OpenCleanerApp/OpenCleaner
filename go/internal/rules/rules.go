package rules

import (
	"context"

	"github.com/opencleaner/opencleaner/pkg/types"
)

type Rule struct {
	ID         string
	Name       string
	Path       string
	Category   types.Category
	Safety     types.SafetyLevel
	SafetyNote string
	Desc       string
	PresetSize *int64 // if set, engine uses this instead of walking the path for size
}

// Scanner is a dynamic scan source that discovers targets at runtime.
// Implementations live in internal/scanner/.
type Scanner interface {
	ID() string
	Name() string
	Category() types.Category
	Scan(ctx context.Context) ([]Rule, error)
}
