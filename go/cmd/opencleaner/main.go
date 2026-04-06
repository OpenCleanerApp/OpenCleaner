package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/opencleaner/opencleaner/internal/daemon"
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
		// Usage: clean <id1,id2,...> [--dry-run] [--execute --yes] [--unsafe] [--force] [--strategy=trash|delete] [--exclude=/path]
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "clean requires comma-separated item ids")
			os.Exit(2)
		}
		rawIDs := strings.Split(args[1], ",")
		ids := make([]string, 0, len(rawIDs))
		for _, id := range rawIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			fmt.Fprintln(os.Stderr, "clean requires at least one item id")
			os.Exit(2)
		}
		req := types.CleanRequest{ItemIDs: ids, Strategy: types.CleanStrategyTrash, DryRun: true}
		confirmed := false
		for _, a := range args[2:] {
			switch {
			case a == "--dry-run":
				req.DryRun = true
			case a == "--execute" || a == "--no-dry-run":
				req.DryRun = false
			case a == "--yes" || a == "-y":
				confirmed = true
			case a == "--unsafe":
				req.Unsafe = true
			case a == "--force":
				req.Force = true
			case strings.HasPrefix(a, "--strategy="):
				req.Strategy = types.CleanStrategy(strings.TrimPrefix(a, "--strategy="))
			case strings.HasPrefix(a, "--exclude="):
				p := strings.TrimPrefix(a, "--exclude=")
				if p != "" {
					req.ExcludePaths = append(req.ExcludePaths, p)
				}
			}
		}
		if !req.DryRun && !confirmed {
			fmt.Fprintln(os.Stderr, "refusing to clean without confirmation; re-run with --execute --yes (or omit --execute for a dry-run)")
			os.Exit(2)
		}
		var res types.CleanResult
		doJSON(client, http.MethodPost, baseURL+"/api/v1/clean", req, &res)
		print(jsonOut, res)
	case "undo":
		var res types.UndoResult
		doJSON(client, http.MethodPost, baseURL+"/api/v1/undo", map[string]any{}, &res)
		print(jsonOut, res)
	case "daemon":
		if len(args) < 2 {
			usage()
			os.Exit(2)
		}
		sub := args[1]
		switch sub {
		case "install":
			binaryPath := "/usr/local/bin/opencleanerd"
			for _, a := range args[2:] {
				if strings.HasPrefix(a, "--binary-path=") {
					binaryPath = strings.TrimPrefix(a, "--binary-path=")
				}
			}
			if err := daemon.InstallPlistWithSocket(binaryPath, socketPath); err != nil {
				fatal(err)
			}
			print(jsonOut, map[string]any{"ok": true})
		case "uninstall":
			if err := daemon.UninstallPlist(); err != nil {
				fatal(err)
			}
			print(jsonOut, map[string]any{"ok": true})
		case "restart":
			if err := daemon.Restart(); err != nil {
				fatal(err)
			}
			print(jsonOut, map[string]any{"ok": true})
		default:
			usage()
			os.Exit(2)
		}
	case "schedule":
		if len(args) < 2 {
			usage()
			os.Exit(2)
		}
		sub := args[1]
		switch sub {
		case "status":
			var st map[string]any
			doJSON(client, http.MethodGet, baseURL+"/api/v1/scheduler", nil, &st)
			print(jsonOut, st)
		case "set":
			// schedule set --interval=daily --time=03:00 [--day=1] [--notify]
			cfg := map[string]any{"enabled": true, "interval": "daily", "time": "03:00"}
			for _, a := range args[2:] {
				switch {
				case strings.HasPrefix(a, "--interval="):
					cfg["interval"] = strings.TrimPrefix(a, "--interval=")
				case strings.HasPrefix(a, "--time="):
					cfg["time"] = strings.TrimPrefix(a, "--time=")
				case strings.HasPrefix(a, "--day="):
					var day int
					fmt.Sscanf(strings.TrimPrefix(a, "--day="), "%d", &day)
					cfg["day"] = day
				case a == "--notify":
					cfg["notify"] = true
				}
			}
			var st map[string]any
			doJSON(client, http.MethodPut, baseURL+"/api/v1/scheduler", cfg, &st)
			print(jsonOut, st)
		case "disable":
			var st map[string]any
			doJSON(client, http.MethodDelete, baseURL+"/api/v1/scheduler", nil, &st)
			print(jsonOut, st)
		default:
			usage()
			os.Exit(2)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func doJSON(client *http.Client, method, url string, body any, out any) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(b, &apiErr); err == nil {
			if msg := strings.TrimSpace(apiErr.Error); msg != "" {
				fatal(fmt.Errorf("%s", msg))
			}
		}
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
		if len(t.Suggestions) > 0 {
			fmt.Println("\nSuggestions:")
			for _, s := range t.Suggestions {
				icon := "💡"
				switch {
				case s.Priority >= 0.8:
					icon = "⚡"
				case s.Priority >= 0.5:
					icon = "🧹"
				}
				fmt.Printf("  %s %s [priority: %.2f]\n", icon, s.Message, s.Priority)
			}
		}
		if len(t.Warnings) > 0 {
			fmt.Println("\nWarnings:")
			for _, w := range t.Warnings {
				fmt.Printf("  ⚠️  %s\n", w)
			}
		}
	case types.CleanResult:
		fmt.Printf("cleaned=%d items, %d bytes; failed=%d; audit=%s\n", t.CleanedCount, t.CleanedSize, len(t.FailedItems), t.AuditLogPath)
		if len(t.FailedItems) > 0 {
			fmt.Printf("failed IDs: %s\n", strings.Join(t.FailedItems, ","))
		}
	case types.UndoResult:
		fmt.Printf("restored=%d items, %d bytes; failed=%d\n", t.RestoredCount, t.RestoredSize, len(t.FailedItems))
		if len(t.FailedItems) > 0 {
			fmt.Printf("failed paths: %s\n", strings.Join(t.FailedItems, ","))
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
	fmt.Println("  opencleaner [--socket=...] [--json] clean <id1,id2> [--dry-run] [--execute --yes] [--unsafe] [--force] [--strategy=trash|delete] [--exclude=/path]")
	fmt.Println("  opencleaner [--socket=...] [--json] undo")
	fmt.Println("  opencleaner [--socket=...] [--json] schedule status")
	fmt.Println("  opencleaner [--socket=...] [--json] schedule set --interval=daily|weekly|monthly --time=HH:MM [--day=0-6] [--notify]")
	fmt.Println("  opencleaner [--socket=...] [--json] schedule disable")
	fmt.Println("  opencleaner daemon install [--binary-path=/usr/local/bin/opencleanerd] [--socket=...]")
	fmt.Println("  opencleaner daemon uninstall")
	fmt.Println("  opencleaner daemon restart")
	fmt.Println("  opencleaner version")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
