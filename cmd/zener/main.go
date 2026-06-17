// Zener - a tiny anonymous file dropbox.
// Copyright (C) 2026 Tobias von Dewitz <tobias@vondewitz.org>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	zener "github.com/tob/zener"
	"github.com/tob/zener/internal/config"
	httpapi "github.com/tob/zener/internal/http"
	s3store "github.com/tob/zener/internal/s3"
	"github.com/tob/zener/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Info("configuration loaded", "config", cfg.Redacted())

	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	objects, err := s3store.New(ctx, s3store.Config{
		Endpoint:     cfg.S3.Endpoint,
		Region:       cfg.S3.Region,
		Bucket:       cfg.S3.Bucket,
		AccessKey:    cfg.S3.AccessKey,
		SecretKey:    cfg.S3.SecretKey,
		UsePathStyle: cfg.S3.UsePathStyle,
	})
	if err != nil {
		return err
	}

	frontend, err := zener.FrontendFS()
	if err != nil {
		return err
	}
	handler, err := httpapi.New(httpapi.Dependencies{
		Store:     db,
		BlobStore: objects,
		Config: httpapi.Config{
			BaseURL:           cfg.BaseURL,
			SessionSecret:     cfg.SessionSecret,
			AdminUsername:     cfg.AdminUsername,
			AdminPassword:     cfg.AdminPassword,
			MaxFileSize:       cfg.MaxFileSize,
			AllowedExtensions: cfg.AllowedExtensions,
			S3Prefix:          cfg.S3.Prefix,
			SecureCookies:     true,
			TrustedProxyHops:  cfg.TrustedProxyHops,
			E2EIntake: httpapi.E2EConfig{
				Enabled:   cfg.E2EIntake.Enabled,
				Required:  cfg.E2EIntake.Required,
				Algorithm: cfg.E2EIntake.Algorithm,
			},
		},
		Logger:   logger,
		StaticFS: httpapi.FS(frontend),
	})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
