package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/andrebq/maestro"
)

func Run(ctx context.Context, addr string, port uint, baseDir string) error {
	srcDir := filepath.Join(baseDir, "tmp")
	binDir := filepath.Join(baseDir, "bin")
	h := NewHandler(ctx, srcDir, binDir)
	srv := &http.Server{
		Addr:    net.JoinHostPort(addr, strconv.FormatUint(uint64(port), 10)),
		Handler: h,
	}
	mctx := maestro.New(ctx)
	mctx.Spawn(func(ctx maestro.Context) error {
		defer mctx.Shutdown()
		slog.Info("Starting server", "address", srv.Addr, "sourceDir", srcDir, "binDir", binDir)
		return srv.ListenAndServe()
	})

	<-mctx.Done()
	slog.Info("Shutting down server", "address", srv.Addr)
	return srv.Shutdown(context.TODO())
}
