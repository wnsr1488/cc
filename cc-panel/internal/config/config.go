package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	JWTSecret      string
	TokenTTL       time.Duration
	EncryptionKey  string
	AdminUsername  string
	AdminPassword  string
	SSHTimeout            time.Duration
	IP2RegionV4XDB        string
	IP2RegionV6XDB        string
	GeoCIDRSyncInterval   time.Duration
	MonitorCollectInterval time.Duration
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		EncryptionKey:  os.Getenv("APP_ENCRYPTION_KEY"),
		AdminUsername:  getenv("ADMIN_USERNAME", "admin"),
		AdminPassword:  os.Getenv("ADMIN_PASSWORD"),
		TokenTTL:       time.Duration(getenvInt("TOKEN_TTL_MINUTES", 60)) * time.Minute,
		SSHTimeout:          time.Duration(getenvInt("SSH_TIMEOUT_SECONDS", 10)) * time.Second,
		IP2RegionV4XDB:      os.Getenv("IP2REGION_V4_XDB"),
		IP2RegionV6XDB:      os.Getenv("IP2REGION_V6_XDB"),
		GeoCIDRSyncInterval:    time.Duration(getenvInt("GEO_CIDR_SYNC_INTERVAL_HOURS", 24)) * time.Hour,
		MonitorCollectInterval: time.Duration(getenvInt("MONITOR_COLLECT_INTERVAL_MINUTES", 5)) * time.Minute,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if len(cfg.EncryptionKey) != 32 {
		return Config{}, fmt.Errorf("APP_ENCRYPTION_KEY must be exactly 32 characters")
	}
	if cfg.AdminPassword == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD is required for initial admin bootstrap")
	}

	return cfg, nil
}

func LoadDatabaseURL() (string, error) {
	if err := loadDotEnv(".env"); err != nil {
		return "", err
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", fmt.Errorf("DATABASE_URL is required")
	}
	return databaseURL, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" && os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s from .env: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
