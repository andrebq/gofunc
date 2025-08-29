package server

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"io"
	"os"

	"github.com/andrebq/gofunc/funcs"
	"github.com/andrebq/maestro"
)

type (
	handler struct {
		m *http.ServeMux

		ctx maestro.Context

		funcs    sync.Map
		funcsCtx sync.Map

		srcDir, binDir string
	}
)

func NewHandler(ctx context.Context, tmpDir, binDir string) *handler {
	h := &handler{
		m:      http.NewServeMux(),
		srcDir: tmpDir,
		binDir: binDir,
		ctx:    maestro.New(ctx),
	}
	h.loadFuncs()
	h.m.HandleFunc("PUT /_admin/{func_name}/recompile", h.recompile)
	h.m.HandleFunc("/{func_name}/", h.invoke)
	h.m.HandleFunc("/{func_name}", h.invoke)
	h.m.HandleFunc("/_health/check", h.healthCheck)
	return h
}

func (h *handler) loadFuncs() {
	funcList, err := funcs.LoadFuncs(h.binDir)
	if err != nil {
		slog.Error("Unable to preload functions from bindir", "error", err, "binDir", h.binDir)
		return
	}
	for _, fn := range funcList {
		h.registerFunc(fn)
	}
}

func (h *handler) registerFunc(fn *funcs.Func) error {
	if oldCtx, _ := h.funcsCtx.Load(fn.Name()); oldCtx != nil {
		oldCtx.(maestro.Context).Shutdown()
		err := maestro.SyncShutdown(oldCtx.(maestro.Context), maestro.TimeoutAfter(time.Minute))
		if err != nil {
			return err
		}
	}
	slog.Info("Registering function", "name", fn.Name(), "binfile", fn.Bin())
	h.funcs.Store(fn.Name(), fn)
	h.ctx.Spawn(h.runFunc(fn.Name(), fn))
	return nil
}

func (h *handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *handler) recompile(w http.ResponseWriter, r *http.Request) {
	// Get function name from path
	funcName := r.PathValue("func_name")
	if funcName == "" {
		http.Error(w, "missing func_name", http.StatusBadRequest)
		return
	}

	slog.Info("Recompiling function", "name", funcName, "addr", r.RemoteAddr, "forwarding", r.Header.Get("X-Forwarded-For"))
	zipFile, err := os.CreateTemp("", "gofaas-upload-*.zip")
	if err != nil {
		http.Error(w, "failed to create temp zipfile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		zipFile.Close()
		os.Remove(zipFile.Name())
	}()
	// Copy body to zip file
	if _, err := io.Copy(zipFile, r.Body); err != nil {
		http.Error(w, "failed to read zip from body: "+err.Error(), http.StatusBadRequest)
		return
	}
	zipFile.Close()
	slog.Info("Uploaded zip file", "path", zipFile.Name())

	// Compile
	start := time.Now()
	fn, err := funcs.Compile(zipFile.Name(), filepath.Join(h.srcDir, funcName), filepath.Join(h.binDir, funcName), funcName)
	if err != nil {
		http.Error(w, "compile error: "+err.Error(), http.StatusBadRequest)
		return
	}
	slog.Info("Compiled function", "name", funcName, "binfile", fn.Bin(), "duration", time.Since(start))
	err = h.registerFunc(fn)
	if err != nil {
		slog.Error("Failed to register function", "error", err)
		http.Error(w, "failed to register function", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","funcName":"` + funcName + `"}`))
}

func (h *handler) runFunc(name string, fn *funcs.Func) func(ctx maestro.Context) error {
	return func(ctx maestro.Context) error {
		h.funcsCtx.Store(name, ctx)
		defer func() {
			h.funcsCtx.Delete(name)
			h.funcs.Delete(name)
		}()
		err := fn.Run(ctx)
		if err != nil {
			println("[ERROR] ", err.Error())
		}
		return err
	}
}

func (h *handler) invoke(w http.ResponseWriter, r *http.Request) {
	funcName := r.PathValue("func_name")
	if funcName == "" {
		http.Error(w, "missing func_name", http.StatusBadRequest)
		return
	}
	fnVal, ok := h.funcs.Load(funcName)
	if !ok {
		http.Error(w, "function not found", http.StatusNotFound)
		return
	}

	fn, ok := fnVal.(*funcs.Func)
	if !ok || fn == nil {
		http.Error(w, "invalid function entry", http.StatusInternalServerError)
		return
	}

	fn.ServeHTTP(w, r)
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.m.ServeHTTP(w, r)
}
