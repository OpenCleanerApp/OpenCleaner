package rules

import "github.com/opencleaner/opencleaner/pkg/types"

type Rule struct {
	ID         string
	Name       string
	Path       string
	Category   types.Category
	Safety     types.SafetyLevel
	SafetyNote string
	Desc       string
}
