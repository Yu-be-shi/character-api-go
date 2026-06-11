package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Port           string `env:"PORT"             envDefault:"8080"`
	LogLevel       string `env:"LOG_LEVEL"        envDefault:"info"`
	InternalAPIKey string `env:"INTERNAL_API_KEY,required"`

	DB DBConfig

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`

	// RedisAddr は冪等性キー用 Redis のアドレス（例: redis:6379）。空なら冪等性機能は無効。
	RedisAddr string `env:"REDIS_ADDR" envDefault:""`

	// RateLimitRPS は /api/v1 への IP あたりの秒間リクエスト上限。0 で無効（既定）。
	RateLimitRPS float64 `env:"RATE_LIMIT_RPS" envDefault:"0"`

	// 予約パターンの TTL 回収設定。確定（confirm）されなかった作成（pending）を、
	// ReservationTTL より古くなったらバックグラウンドスイーパーが物理回収する。
	// ReservationSweepInterval が 0 ならスイーパーを起動しない。
	ReservationTTL           time.Duration `env:"RESERVATION_TTL"            envDefault:"1h"`
	ReservationSweepInterval time.Duration `env:"RESERVATION_SWEEP_INTERVAL" envDefault:"10m"`
}

type DBConfig struct {
	DSN string `env:"DB_DSN,required"`
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}
	return c, nil
}
