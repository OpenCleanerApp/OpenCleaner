package engine

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/opencleaner/opencleaner/internal/audit"
	"github.com/opencleaner/opencleaner/internal/rules"
	"github.com/opencleaner/opencleaner/internal/stream"
	"github.com/opencleaner/opencleaner/pkg/types"
)

func BenchmarkScan100Files(b *testing.B)   { benchmarkScanN(b, 100) }
func BenchmarkScan10000Files(b *testing.B) { benchmarkScanN(b, 10_000) }
func BenchmarkScan100000Files(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping large benchmark in -short")
	}
	benchmarkScanN(b, 100_000)
}

func benchmarkScanN(b *testing.B, n int) {
	home := b.TempDir()
	b.Setenv("HOME", home)

	root := filepath.Join(home, "Library", "Caches", "bench-scan")
	if err := os.MkdirAll(root, 0o700); err != nil {
		b.Fatal(err)
	}

	payload := []byte("x")
	// Create files in a small fanout to avoid huge single-directory slowdowns.
	for i := 0; i < n; i++ {
		d := filepath.Join(root, "d", string(rune('a'+(i%26))))
		if err := os.MkdirAll(d, 0o700); err != nil {
			b.Fatal(err)
		}
		p := filepath.Join(d, "f"+strconv.Itoa(i))
		if err := os.WriteFile(p, payload, 0o600); err != nil {
			b.Fatal(err)
		}
	}

	eng := New([]rules.Rule{{
		ID:         "bench",
		Name:       "Bench",
		Path:       root,
		Category:   types.CategorySystem,
		Safety:     types.SafetySafe,
		SafetyNote: "bench",
		Desc:       "bench",
	}}, stream.NewBroker(), audit.NewLogger(filepath.Join(home, "audit.log")))

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := eng.Scan(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
