package funcs

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type (
	Func struct {
		binfile string
		proc    *os.Process
		proxy   *httputil.ReverseProxy
	}
)

func (f *Func) Name() string {
	bin := filepath.Base(f.binfile)
	return strings.TrimSuffix(bin, filepath.Ext(bin))
}

func (f *Func) Bin() string {
	return f.binfile
}

func Compile(zipfile string, srcdir string, bindir string, funcname string) (*Func, error) {
	// Open the zip archive
	zr, err := zip.OpenReader(zipfile)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	// Ensure srcdir exists
	if err := os.MkdirAll(srcdir, 0755); err != nil {
		return nil, fmt.Errorf("create srcdir: %w", err)
	}

	// Extract files
	absSrc, _ := filepath.Abs(srcdir)
	for _, f := range zr.File {
		// Protect against ZipSlip
		destPath := filepath.Join(srcdir, f.Name)
		destPathClean, err := filepath.Abs(filepath.Clean(destPath))
		if err != nil {
			return nil, fmt.Errorf("failed to get abs path: %w", err)
		}
		if destPathClean != absSrc && !strings.HasPrefix(destPathClean, absSrc+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(destPathClean, 0755); err != nil {
				return nil, fmt.Errorf("makedir: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPathClean), 0755); err != nil {
			return nil, fmt.Errorf("mkdir for file: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zipped file: %w", err)
		}
		outFile, err := os.OpenFile(destPathClean, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("create file: %w", err)
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return nil, fmt.Errorf("copy file contents: %w", err)
		}
		outFile.Close()
		rc.Close()
	}

	// Ensure bindir exists
	if err := os.MkdirAll(bindir, 0755); err != nil {
		return nil, fmt.Errorf("create bindir: %w", err)
	}

	// Run go build from srcdir
	outPath := filepath.Join(bindir, funcname+".out")
	cmd := exec.Command("go", "build", "-o", outPath, ".")
	cmd.Dir = srcdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go build failed: %w: %s", err, string(out))
	}

	// Return an empty Func; the caller can set up runtime/proxy as needed
	return &Func{
		binfile: outPath,
	}, nil
}

func (f *Func) Run(ctx context.Context) error {
	if f.binfile == "" {
		return errors.New("no binary to run")
	}

	// Find a free random port by listening on :0
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	addr := ln.Addr().String()
	// extract port
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		ln.Close()
		return fmt.Errorf("split host port: %w", err)
	}
	// close the listener to free the port for the process
	ln.Close()

	// Prepare command with env vars
	env := os.Environ()
	env = append(env, "BIND_ADDR=127.0.0.1")
	env = append(env, fmt.Sprintf("BIND_PORT=%s", portStr))

	cmd := exec.CommandContext(ctx, f.binfile)
	cmd.Env = env
	// redirect stdout/stderr to parent for debugging (optional)
	cmd.Stdout = os.Stdout
	buf := bytes.Buffer{}
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// Save process handle
	f.proc = cmd.Process

	// Reap process in background and capture exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		f.proc = nil
	}()

	// Wait for the process to start accepting connections
	targetHost := fmt.Sprintf("127.0.0.1:%s", portStr)
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", targetHost, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		lastErr = err
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return fmt.Errorf("context canceled while waiting for backend: %w", ctx.Err())
		case err, closed := <-done:
			if !closed || err != nil {
				return fmt.Errorf("process exited before listening: %v, (closed: %v)", err, closed)
			}
		default:
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			return fmt.Errorf("backend did not start in time: %w", lastErr)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Build proxy to the running process
	target := &url.URL{Scheme: "http", Host: targetHost}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// Ensure the director preserves the original request path and query
	origDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		origDirector(r)
		// keep Host header of target
		r.Host = target.Host
	}

	f.proxy = proxy

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (f *Func) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.proxy.ServeHTTP(w, r)
}
