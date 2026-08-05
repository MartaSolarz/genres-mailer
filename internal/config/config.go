package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string
	DBPath     string
	DataDir    string

	AppSecretKey  []byte
	SessionSecret []byte

	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	FileRetention time.Duration
	MaxUploadMB   int64
}

func Load() (*Config, error) {
	c := &Config{}

	c.ListenAddr = getEnvDefault("LISTEN_ADDR", "127.0.0.1:8080")

	c.DBPath = os.Getenv("DB_PATH")
	if c.DBPath == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej DB_PATH")
	}

	c.DataDir = os.Getenv("DATA_DIR")
	if c.DataDir == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej DATA_DIR")
	}

	key, err := decodeBase64Key("APP_SECRET_KEY", 32)
	if err != nil {
		return nil, err
	}

	c.AppSecretKey = key

	sessRaw := os.Getenv("SESSION_SECRET")
	if sessRaw == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej SESSION_SECRET")
	}

	sess, derr := base64.StdEncoding.DecodeString(sessRaw)
	if derr != nil {
		sess = []byte(sessRaw)
	}

	if len(sess) < 16 {
		return nil, fmt.Errorf("SESSION_SECRET musi mieć co najmniej 16 bajtów")
	}

	c.SessionSecret = sess

	c.SMTPHost = os.Getenv("SMTP_HOST")
	if c.SMTPHost == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej SMTP_HOST")
	}

	portStr := getEnvDefault("SMTP_PORT", "25")

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("SMTP_PORT ma nieprawidłową wartość: %q", portStr)
	}

	c.SMTPPort = port

	c.SMTPUser = os.Getenv("SMTP_USER")
	c.SMTPPass = os.Getenv("SMTP_PASS")

	c.SMTPFrom = os.Getenv("SMTP_FROM")
	if c.SMTPFrom == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej SMTP_FROM")
	}

	if !strings.Contains(c.SMTPFrom, "@") {
		return nil, fmt.Errorf("SMTP_FROM nie wygląda na adres e-mail: %q", c.SMTPFrom)
	}

	retHours, err := getEnvIntDefault("FILE_RETENTION_HOURS", 72)
	if err != nil {
		return nil, err
	}

	if retHours <= 0 {
		return nil, fmt.Errorf("FILE_RETENTION_HOURS musi być dodatnie")
	}

	c.FileRetention = time.Duration(retHours) * time.Hour

	maxUpload, err := getEnvIntDefault("MAX_UPLOAD_MB", 25)
	if err != nil {
		return nil, err
	}

	if maxUpload <= 0 {
		return nil, fmt.Errorf("MAX_UPLOAD_MB musi być dodatnie")
	}

	c.MaxUploadMB = int64(maxUpload)

	return c, nil
}

func (c *Config) MaxUploadBytes() int64 {
	return c.MaxUploadMB * 1024 * 1024
}

func (c *Config) SessionTTL() time.Duration {
	return 8 * time.Hour
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func getEnvIntDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s ma nieprawidłową wartość liczbową: %q", key, v)
	}

	return n, nil
}

func decodeBase64Key(key string, wantLen int) ([]byte, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return nil, fmt.Errorf("brak wymaganej zmiennej %s", key)
	}

	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s nie jest poprawnym base64: %w", key, err)
	}

	if len(b) != wantLen {
		return nil, fmt.Errorf("%s musi mieć %d bajtów po zdekodowaniu (ma %d)", key, wantLen, len(b))
	}

	return b, nil
}
