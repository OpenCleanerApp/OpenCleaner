package transport

import (
	"fmt"
	"os"
)

// DefaultSocketPath returns a short, per-user unix socket path.
//
// We intentionally keep this in /tmp to avoid overlong sun_path issues on macOS.
func DefaultSocketPath() string {
	return fmt.Sprintf("/tmp/opencleaner.%d.sock", os.Getuid())
}
