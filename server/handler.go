package server

import (
	"context"
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
	h.m.HandleFunc("PUT /_admin/{func_name}/recompile", h.recompile)
	h.m.HandleFunc("/{func_name}/", h.invoke)
	h.m.HandleFunc("/{func_name}", h.invoke)
	h.m.HandleFunc("/_health/check", h.healthCheck)
	return h
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

	// Compile
	fn, err := funcs.Compile(zipFile.Name(), filepath.Join(h.srcDir, funcName), filepath.Join(h.binDir, funcName), funcName)
	if err != nil {
		http.Error(w, "compile error: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Start function in background
	if oldCtx, _ := h.funcsCtx.Load(funcName); oldCtx != nil {
		oldCtx.(maestro.Context).Shutdown()
		err := maestro.SyncShutdown(oldCtx.(maestro.Context), maestro.TimeoutAfter(time.Minute))
		if err != nil {
			// TODO: handle errors here, and figure out what to if the process keeps running after being killed
			http.Error(w, "failed to shutdown old function", http.StatusInternalServerError)
		}
	}
	h.funcs.Store(funcName, fn)
	h.ctx.Spawn(h.runFunc(funcName, fn))

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
