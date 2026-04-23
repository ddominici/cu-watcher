package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DB struct {
		ConnectionString string `yaml:"connectionString"`
		WindowsAuth      bool   `yaml:"windowsAuth"`
	} `yaml:"db"`

	Scraper struct {
		UserAgent              string `yaml:"userAgent"`
		TimeoutSeconds         int    `yaml:"timeoutSeconds"`
		MaxConcurrency         int    `yaml:"maxConcurrency"`
		DelayBetweenRequestsMs int    `yaml:"delayBetweenRequestsMs"`
		FollowKBLinks          bool   `yaml:"followKbLinks"`
		MaxKBToFetch           int    `yaml:"maxKbToFetch"`
		Since                  string `yaml:"since"`
	} `yaml:"scraper"`

	Sources []struct {
		Key          string `yaml:"key"`
		MajorVersion int    `yaml:"majorVersion"`
		URL          string `yaml:"url"`
	} `yaml:"sources"`

	Logging struct {
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		MaxSizeMB  int    `yaml:"maxSizeMB"`
		MaxBackups int    `yaml:"maxBackups"`
		MaxAgeDays int    `yaml:"maxAgeDays"`
	} `yaml:"logging"`

	Email struct {
		Enabled  bool     `yaml:"enabled"`
		From     string   `yaml:"from"`
		To       []string `yaml:"to"`
		SMTPHost string   `yaml:"smtpHost"`
		SMTPPort int      `yaml:"smtpPort"`
		Username string   `yaml:"username"`
		Password string   `yaml:"password"`
		UseTLS   bool     `yaml:"useTLS"`
	} `yaml:"email"`
}

// Load reads the config file at path, first loading a .env file from the same
// directory (if present), then expanding ${VAR} placeholders in the YAML using
// environment variables.
func Load(path string) (*Config, error) {
	loadDotEnv(filepath.Join(filepath.Dir(path), ".env"))

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand ${VAR} / $VAR placeholders before YAML parsing so that sensitive
	// values can live in .env rather than in the config file itself.
	expanded := os.ExpandEnv(string(b))

	cfg := &Config{}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml", ".json":
		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("config must be .yaml/.yml/.json")
	}

	// defaults
	if cfg.Scraper.TimeoutSeconds == 0 {
		cfg.Scraper.TimeoutSeconds = 60
	}
	if cfg.Scraper.MaxConcurrency == 0 {
		cfg.Scraper.MaxConcurrency = 4
	}
	if cfg.Scraper.UserAgent == "" {
		cfg.Scraper.UserAgent = "SqlUpdatesScraper/1.0"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.File == "" {
		cfg.Logging.File = "logs/scraper.log"
	}
	if cfg.Logging.MaxSizeMB == 0 {
		cfg.Logging.MaxSizeMB = 50
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = 10
	}
	if cfg.Logging.MaxAgeDays == 0 {
		cfg.Logging.MaxAgeDays = 14
	}

	return cfg, nil
}

// loadDotEnv reads KEY=VALUE pairs from path and sets them as environment
// variables. Existing variables are never overwritten (the real environment
// always takes precedence). The file is optional; missing files are silently
// ignored.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		// Don't override variables already set in the real environment.
		if os.Getenv(key) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		// Strip matching surrounding quotes.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		os.Setenv(key, value)
	}
}
