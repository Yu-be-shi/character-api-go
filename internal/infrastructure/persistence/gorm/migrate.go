package gormrepo

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed migrations/postgres
var migrationsFS embed.FS

// schemaMigration はマイグレーション適用記録。
// AutoMigrate でテーブルを作るため DDL はダイアレクト非依存。
type schemaMigration struct {
	Version   int64     `gorm:"primaryKey;autoIncrement:false"`
	Name      string    `gorm:"size:255;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

// runMigrations は migrations/postgres/*.up.sql を昇順で読み込み、
// 未適用のものだけ実行して記録する。
func runMigrations(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return fmt.Errorf("migrate: create tracking table: %w", err)
	}

	var recorded []schemaMigration
	if err := db.Find(&recorded).Error; err != nil {
		return fmt.Errorf("migrate: query applied: %w", err)
	}
	applied := make(map[int64]bool, len(recorded))
	for _, m := range recorded {
		applied[m.Version] = true
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("migrate: read migrations/postgres: %w", err)
	}

	var upFiles []fs.DirEntry
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e)
		}
	}
	sort.Slice(upFiles, func(i, j int) bool { return upFiles[i].Name() < upFiles[j].Name() })

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("migrate: get sql.DB: %w", err)
	}

	for _, f := range upFiles {
		version, err := parseMigrationVersion(f.Name())
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}

		content, err := fs.ReadFile(migrationsFS, "migrations/postgres/"+f.Name())
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", f.Name(), err)
		}

		if _, err := sqlDB.Exec(string(content)); err != nil {
			return fmt.Errorf("migrate: exec %s: %w", f.Name(), err)
		}

		record := schemaMigration{Version: version, Name: f.Name(), AppliedAt: time.Now().UTC()}
		if err := db.Create(&record).Error; err != nil {
			return fmt.Errorf("migrate: record %s: %w", f.Name(), err)
		}

		slog.Info("migration applied", "version", version, "name", f.Name())
	}

	return nil
}

func parseMigrationVersion(filename string) (int64, error) {
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("migrate: invalid filename %q (want NNNN_description.up.sql)", filename)
	}
	var v int64
	if _, err := fmt.Sscan(parts[0], &v); err != nil {
		return 0, fmt.Errorf("migrate: parse version from %q: %w", filename, err)
	}
	return v, nil
}
