package types

import "time"

type Category string

const (
	CategorySystem    Category = "system"
	CategoryDeveloper Category = "developer"
	CategoryApps      Category = "apps"
)

type SafetyLevel string

const (
	SafetySafe     SafetyLevel = "safe"
	SafetyModerate SafetyLevel = "moderate"
	SafetyRisky    SafetyLevel = "risky"
)

type ScanItem struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Size        int64       `json:"size"`
	Category    Category    `json:"category"`
	SafetyLevel SafetyLevel `json:"safety_level"`
	SafetyNote  string      `json:"safety_note,omitempty"`
	Description string      `json:"description,omitempty"`
	LastAccess  *time.Time  `json:"last_access,omitempty"`
}

type ScanResult struct {
	TotalSize       int64              `json:"total_size"`
	Items           []ScanItem         `json:"items"`
	ScanDurationMs  int64              `json:"scan_duration_ms"`
	CategorizedSize map[Category]int64 `json:"categorized_size"`
}

type CleanStrategy string

const (
	CleanStrategyTrash  CleanStrategy = "trash"
	CleanStrategyDelete CleanStrategy = "delete"
)

type CleanRequest struct {
	ItemIDs      []string      `json:"item_ids"`
	ExcludePaths []string      `json:"exclude_paths,omitempty"`
	Strategy     CleanStrategy `json:"strategy"`
	DryRun       bool          `json:"dry_run,omitempty"`
	Unsafe       bool          `json:"unsafe,omitempty"`
	Force        bool          `json:"force,omitempty"`
}

type CleanResult struct {
	CleanedSize  int64    `json:"cleaned_size"`
	CleanedCount int      `json:"cleaned_count"`
	FailedItems  []string `json:"failed_items"`
	AuditLogPath string   `json:"audit_log_path"`
	DryRun       bool     `json:"dry_run,omitempty"`
}

type DaemonStatus struct {
	OK         bool   `json:"ok"`
	Version    string `json:"version"`
	SocketPath string `json:"socket_path"`
}

type ProgressEvent struct {
	Type     string   `json:"type"`
	Current  string   `json:"current,omitempty"`
	Progress float64  `json:"progress,omitempty"`
	Message  string   `json:"message,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}
