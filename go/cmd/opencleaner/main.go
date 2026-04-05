package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/opencleaner/opencleaner/internal/transport"
	"github.com/opencleaner/opencleaner/pkg/types"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	socketPath := transport.DefaultSocketPath()
	jsonOut := false

	// global flags (very small parser): --socket=..., --json
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "--socket=") {
			socketPath = strings.TrimPrefix(a, "--socket=")
			continue
		}
		if a == "--json" {
			jsonOut = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	cmd := args[0]
	client := transport.NewUnixSocketHTTPClient(socketPath)
	baseURL := "http://unix"

	switch cmd {
	case "version":
		fmt.Println(version)
	case "status":
		var st types.DaemonStatus
		doJSON(client, http.MethodGet, baseURL+"/api/v1/status", nil, &st)
		print(jsonOut, st)
	case "scan":
		var res types.ScanResult
		doJSON(client, http.MethodPost, baseURL+"/api/v1/scan", map[string]any{}, &res)
		print(jsonOut, res)
	case "clean":
		// Usage: clean <id1,id2,...> [--dry-run] [--unsafe] [--force] [--strategy=trash|delete]
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "clean requires comma-separated item ids")
			os.Exit(2)
		}
		ids := strings.Split(args[1], ",")
		req := types.CleanRequest{ItemIDs: ids, Strategy: types.CleanStrategyTrash}
		for _, a := range args[2:] {
			switch {
			case a == "--dry-run":
				req.DryRun = true
			case a == "--unsafe":
				req.Unsafe = true
			case a == "--force":
				req.Force = true
			case strings.HasPrefix(a, "--strategy="):
				req.Strategy = types.CleanStrategy(strings.TrimPrefix(a, "--strategy="))
			}
		}
		var res types.CleanResult
		doJSON(client, http.MethodPost, baseURL+"/api/v1/clean", req, &res)
		print(jsonOut, res)
	default:
		usage()
		os.Exit(2)
	}
}

func doJSON(client *http.Client, method, url string, body any, out any) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rdr)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		fatal(fmt.Errorf("%s", strings.TrimSpace(string(b))))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			fatal(err)
		}
	}
}

func print(jsonOut bool, v any) {
	if jsonOut {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return
	}
	switch t := v.(type) {
	case types.DaemonStatus:
		fmt.Printf("ok=%v version=%s socket=%s\n", t.OK, t.Version, t.SocketPath)
	case types.ScanResult:
		fmt.Printf("items=%d total=%d bytes\n", len(t.Items), t.TotalSize)
		for _, it := range t.Items {
			fmt.Printf("- %s (%s): %d bytes [%s]\n", it.ID, it.Path, it.Size, it.SafetyLevel)
		}
	case types.CleanResult:
		fmt.Printf("cleaned=%d items, %d bytes; failed=%d; audit=%s\n", t.CleanedCount, t.CleanedSize, len(t.FailedItems), t.AuditLogPath)
		if len(t.FailedItems) > 0 {
			fmt.Printf("failed IDs: %s\n", strings.Join(t.FailedItems, ","))
		}
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
}

func usage() {
	fmt.Println("opencleaner (MVP)")
	fmt.Println("Usage:")
	fmt.Printf("  opencleaner [--socket=%s] [--json] status\n", transport.DefaultSocketPath())
	fmt.Println("  opencleaner [--socket=...] [--json] scan")
	fmt.Println("  opencleaner [--socket=...] [--json] clean <id1,id2> [--dry-run] [--unsafe] [--force] [--strategy=trash|delete]")
	fmt.Println("  opencleaner version")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
