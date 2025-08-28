package uploader

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrebq/gofunc/server"
)

// Test that CreateZip respects ignore patterns and includes files correctly.
func TestCreateZip_RespectsIgnore(t *testing.T) {
	dir := t.TempDir()

	// Create files and dirs
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0600)
	os.WriteFile(filepath.Join(dir, "b.log"), []byte("b"), 0600)
	os.MkdirAll(filepath.Join(dir, "sub"), 0700)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0600)

	// Write ignore: ignore *.log and sub/
	os.WriteFile(filepath.Join(dir, ".gofaasignore"), []byte("*.log\nsub/\n"), 0600)

	tmp := filepath.Join(t.TempDir(), "out.zip")
	patterns := LoadIgnoreFile(filepath.Join(dir, ".gofaasignore"))
	if err := CreateZip(tmp, dir, patterns); err != nil {
		t.Fatalf("CreateZip failed: %v", err)
	}

	// Open zip and check entries
	zr, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()

	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}

	if !contains(names, "a.txt") {
		t.Fatalf("expected a.txt in zip, got %v", names)
	}
	if contains(names, "b.log") {
		t.Fatalf("did not expect b.log in zip")
	}
	if contains(names, "sub/c.txt") || contains(names, "sub/") {
		t.Fatalf("did not expect sub entries in zip")
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// TestUpload_EndToEnd starts a server using server.NewHandler and uses Upload
// to PUT a zip; then it verifies the uploaded function can be invoked.
func TestUpload_EndToEnd(t *testing.T) {
	// prepare a minimal go module to upload
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module testfunc\n\ngo 1.24\n"), 0600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainGo := `package main
import (
	"fmt"
	"net/http"
	"os"
)
func main() {
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("BIND_PORT")), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("uploaded"))
	}))
}`
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(mainGo), 0600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	// start server with handler
	tmpDir := t.TempDir()
	binDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := server.NewHandler(ctx, tmpDir, binDir)
	ts := httptest.NewServer(h)
	defer ts.Close()

	// perform upload
	if err := Upload(context.Background(), ts.URL, "testfunc", src); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// wait a bit for compile/start
	time.Sleep(time.Second)

	// invoke the uploaded function
	resp, err := http.Get(ts.URL + "/testfunc/")
	if err != nil {
		t.Fatalf("invoke request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
}
