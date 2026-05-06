package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Env            string
	Port           int
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	ReservationTTL time.Duration
	VATRate        decimal.Decimal
	Currency       string
}

// Load reads environment variables and returns a validated Config.
func Load() (*Config, error) {
	port, err := strconv.Atoi(getEnv("APP_PORT", "8080"))
	if err != nil {
		return nil, fmt.Errorf("invalid APP_PORT: %w", err)
	}

	ttlSeconds, err := strconv.Atoi(getEnv("RESERVATION_TTL_S", "900"))
	if err != nil {
		return nil, fmt.Errorf("invalid RESERVATION_TTL_S: %w", err)
	}

	vatRate, err := decimal.NewFromString(getEnv("VAT_RATE", "0.05"))
	if err != nil {
		return nil, fmt.Errorf("invalid VAT_RATE: %w", err)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	return &Config{
		Env:            getEnv("APP_ENV", "development"),
		Port:           port,
		DatabaseURL:    dbURL,
		RedisURL:       redisURL,
		JWTSecret:      getEnv("JWT_SECRET", ""),
		ReservationTTL: time.Duration(ttlSeconds) * time.Second,
		VATRate:        vatRate,
		Currency:       getEnv("CURRENCY", "AED"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
