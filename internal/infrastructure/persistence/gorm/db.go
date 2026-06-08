package gormrepo

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/yu-be-shi/character-api/internal/config"
)

// Open は設定済み *gorm.DB を返す。
// マイグレーションはこの API では行わない（スキーマは character-db の Atlas が管理する）。
func Open(cfg config.DBConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		// ドライバ固有のエラーを gorm.ErrDuplicatedKey / ErrForeignKeyViolated 等へ変換し、
		// リポジトリがドメインエラー（ErrDuplicate 等）へマッピングできるようにする。
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gormrepo: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("gormrepo: sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	slog.Info("database connected", "driver", "postgres")
	return db, nil
}
