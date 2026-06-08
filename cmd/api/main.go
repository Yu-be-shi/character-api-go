// @title           Character API
// @version         1.0
// @description     キャラクター管理 REST API
// @host            localhost:8080
// @BasePath        /
//
// @securityDefinitions.apikey  InternalAPIKey
// @in                          header
// @name                        X-Internal-API-Key
// @description                 内部サービス間認証キー

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yu-be-shi/character-api/internal/config"
	gormrepo "github.com/yu-be-shi/character-api/internal/infrastructure/persistence/gorm"
	httpiface "github.com/yu-be-shi/character-api/internal/interfaces/http"
	charUsecase "github.com/yu-be-shi/character-api/internal/usecase/character"
	raceUsecase "github.com/yu-be-shi/character-api/internal/usecase/race"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogger(cfg.LogLevel)

	db, err := gormrepo.Open(cfg.DB)
	if err != nil {
		return err
	}

	raceRepo := gormrepo.NewRaceRepository(db)
	charRepo := gormrepo.NewCharacterRepository(db)

	raceSvc := raceUsecase.NewService(raceRepo)
	charSvc := charUsecase.NewService(charRepo, raceRepo, nil)

	e := httpiface.New(cfg, charSvc, raceSvc)

	srvErr := make(chan error, 1)
	go func() {
		addr := ":" + cfg.Port
		slog.Info("http server starting", "addr", addr)
		if err := e.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
		close(srvErr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
		slog.Info("shutdown signal received")
	case err := <-srvErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("shutdown complete")
	return nil
}

func setupLogger(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}
