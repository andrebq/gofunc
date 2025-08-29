package funcs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompileLoadFuncsAndRun(t *testing.T) {
	tmp := t.TempDir()

	// Prepare source files for a simple HTTP binary
	files := map[string]string{
		"go.mod": "module example.com/testmod\n\n",
		"main.go": `package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    addr := os.Getenv("BIND_ADDR")
    port := os.Getenv("BIND_PORT")
    if addr == "" { addr = "127.0.0.1" }
    if port == "" { port = "8080" }
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "fromloadfuncs")
    })
    http.ListenAndServe(addr+":"+port, nil)
}
`,
	}

	// Create zip of source files
	zipPath := filepath.Join(tmp, "src.zip")
	writeZip(t, zipPath, files)

	// Prepare extraction and bin dirs
	extractDir := filepath.Join(tmp, "extracted")
	binDir := filepath.Join(tmp, "bin")

	// Compile the function (do not run it yet)
	_, err := Compile(zipPath, extractDir, binDir, "myfunc")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Load the function(s) from binDir
	funcs, err := LoadFuncs(binDir)
	if err != nil {
		t.Fatalf("LoadFuncs failed: %v", err)
	}
	if len(funcs) != 1 {
		t.Fatalf("expected 1 func, got %d", len(funcs))
	}

	// Run the loaded function
	fobj := funcs[0]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fobj.Run(ctx)
	}()

	// Wait for the function to be ready
	time.Sleep(time.Second)

	// Expose the Func via an httptest server (it uses the reverse proxy)
	ts := httptest.NewServer(fobj)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("http get failed: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status: %d body=%q", resp.StatusCode, body)
	}
	if body != "fromloadfuncs\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestLoadFuncs_NoOutFiles(t *testing.T) {
	tmp := t.TempDir()
	// create a non-.out file
	f := filepath.Join(tmp, "notanout.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	funcs, err := LoadFuncs(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Fatalf("expected 0 funcs, got %d", len(funcs))
	}
}

func TestLoadFuncs_NonExecutableOut(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "foo.out")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// not executable
	if err := os.Chmod(f, 0644); err != nil {
		t.Fatal(err)
	}
	funcs, err := LoadFuncs(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Fatalf("expected 0 funcs, got %d", len(funcs))
	}
}

func TestLoadFuncs_ExecutableOut(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "bar.out")
	if err := os.WriteFile(f, []byte("data"), 0755); err != nil {
		t.Fatal(err)
	}
	// make sure it's executable
	if err := os.Chmod(f, 0755); err != nil {
		t.Fatal(err)
	}
	funcs, err := LoadFuncs(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 1 {
		t.Fatalf("expected 1 func, got %d", len(funcs))
	}
	if funcs[0].binfile != f {
		t.Errorf("expected binfile %q, got %q", f, funcs[0].binfile)
	}
}
