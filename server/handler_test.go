package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func createTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatalf("failed to create temp zip: %v", err)
	}
	defer tmpfile.Close()
	w := NewZipWriter(tmpfile)
	for name, content := range files {
		if err := w.AddFile(name, []byte(content)); err != nil {
			t.Fatalf("failed to add file to zip: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return tmpfile.Name()
}

type ZipWriter struct {
	w  *os.File
	zw *zip.Writer
}

func NewZipWriter(f *os.File) *ZipWriter {
	return &ZipWriter{w: f, zw: zip.NewWriter(f)}
}

func (z *ZipWriter) AddFile(name string, content []byte) error {
	f, err := z.zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return err
}

func (z *ZipWriter) Close() error {
	return z.zw.Close()
}

func TestHandler_RecompileAndInvoke(t *testing.T) {
	tmpDir := t.TempDir()
	binDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := NewHandler(ctx, tmpDir, binDir)

	// Create a minimal Go function as a zip
	mainGo := `package main
import (
	"fmt"
	"net/http"
	"os"
)
func main() {
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("BIND_PORT")), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
}`
	zipPath := createTestZip(t, map[string]string{"main.go": mainGo, "go.mod": `
	module testfunc

	go 1.24
	`})
	defer os.Remove(zipPath)
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	// Recompile
	req := httptest.NewRequest("PUT", "/_admin/testfunc/recompile", bytes.NewReader(zipData))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("recompile failed: %s", rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected status: %v", resp)
	}

	// Wait for function to start
	time.Sleep(2 * time.Second)

	// Invoke
	req2 := httptest.NewRequest("GET", "/testfunc/", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("invoke failed: %s", rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), "hello") {
		t.Fatalf("unexpected invoke response: %s", rec2.Body.String())
	}
}
