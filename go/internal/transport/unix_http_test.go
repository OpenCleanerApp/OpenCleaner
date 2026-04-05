package transport

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func shortTempDir(t *testing.T) string {
	// Unix socket paths are length-limited; t.TempDir() can be too long on macOS.
	if d, err := os.MkdirTemp("/tmp", "opencleaner-"); err == nil {
		t.Cleanup(func() { _ = os.RemoveAll(d) })
		return d
	}
	d := t.TempDir()
	return d
}

func TestListenUnixSocketRefusesNonSocket(t *testing.T) {
	tmp := shortTempDir(t)
	sock := filepath.Join(tmp, "opencleaner.sock")

	if err := os.WriteFile(sock, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ListenUnixSocket(sock)
	if err == nil {
		t.Fatalf("expected error")
	}

	if _, err2 := os.Lstat(sock); err2 != nil {
		t.Fatalf("expected file to remain, got: %v", err2)
	}
}

func TestListenUnixSocketDetectsAlreadyRunning(t *testing.T) {
	tmp := shortTempDir(t)
	sock := filepath.Join(tmp, "opencleaner.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, err = ListenUnixSocket(sock)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestListenUnixSocketCreatesSocket0600(t *testing.T) {
	tmp := shortTempDir(t)
	sock := filepath.Join(tmp, "opencleaner.sock")

	ln, err := ListenUnixSocket(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	defer os.Remove(sock)

	fi, err := os.Lstat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("expected socket file")
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("expected perms 0600, got %o", fi.Mode().Perm())
	}
}
