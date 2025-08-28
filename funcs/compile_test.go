package funcs

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrebq/maestro"
)

func writeZip(t *testing.T, zipPath string, files map[string]string) {
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(w, bytes.NewBufferString(content)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompileRunProxy(t *testing.T) {
	tmp := t.TempDir()

	srcDir := filepath.Join(tmp, "srcfiles")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// source files for the tiny HTTP binary
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
        fmt.Fprintln(w, "hello")
    })
    http.ListenAndServe(addr+":"+port, nil)
}
`,
	}

	// create zip of source files
	zipPath := filepath.Join(tmp, "src.zip")
	writeZip(t, zipPath, files)

	// prepare extraction and bin dirs
	extractDir := filepath.Join(tmp, "extracted")
	binDir := filepath.Join(tmp, "bin")

	fobj, err := Compile(zipPath, extractDir, binDir, "myfunc")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	mctx := maestro.New(context.Background())
	mctx.Spawn(func(ctx maestro.Context) error {
		err := fobj.Run(ctx)
		if !errors.Is(err, context.Canceled) {
			log.Fatal("Run error", err)
		}
		return nil
	})

	// wait for the function to be ready
	time.Sleep(time.Second)

	// expose the Func via an httptest server (it uses the reverse proxy)
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
	if body != "hello\n" {
		t.Fatalf("unexpected body: %q", body)
	}

	mctx.Shutdown()
	mctx.WaitChildren(maestro.TimeoutAfter(time.Minute))
}
