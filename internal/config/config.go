package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Port           string `env:"PORT"             envDefault:"8080"`
	LogLevel       string `env:"LOG_LEVEL"        envDefault:"info"`
	InternalAPIKey string `env:"INTERNAL_API_KEY,required"`

	DB DBConfig

	CORSOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
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
