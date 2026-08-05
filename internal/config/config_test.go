package config

import (
	"testing"
)

func validEnv() map[string]string {
	return map[string]string{
		"LISTEN_ADDR":          "127.0.0.1:8080",
		"DB_PATH":              "./data/app.db",
		"DATA_DIR":             "./data",
		"APP_SECRET_KEY":       "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"SESSION_SECRET":       "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=",
		"SMTP_HOST":            "localhost",
		"SMTP_PORT":            "1025",
		"SMTP_FROM":            "wyniki@example.org",
		"FILE_RETENTION_HOURS": "72",
		"MAX_UPLOAD_MB":        "25",
	}
}

func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadValid(t *testing.T) {
	setEnv(t, validEnv())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("oczekiwano poprawnego wczytania, otrzymano błąd: %v", err)
	}
	if len(cfg.AppSecretKey) != 32 {
		t.Errorf("APP_SECRET_KEY: oczekiwano 32 bajtów, jest %d", len(cfg.AppSecretKey))
	}
	if cfg.MaxUploadBytes() != 25*1024*1024 {
		t.Errorf("MaxUploadBytes: nieprawidłowa wartość %d", cfg.MaxUploadBytes())
	}
	if cfg.FileRetention.Hours() != 72 {
		t.Errorf("FileRetention: oczekiwano 72h, jest %v", cfg.FileRetention)
	}
}

func TestLoadFailFast(t *testing.T) {
	cases := map[string]func(m map[string]string){
		"brak DB_PATH":             func(m map[string]string) { delete(m, "DB_PATH") },
		"brak DATA_DIR":            func(m map[string]string) { delete(m, "DATA_DIR") },
		"brak APP_SECRET_KEY":      func(m map[string]string) { delete(m, "APP_SECRET_KEY") },
		"APP_SECRET_KEY zła dł.":   func(m map[string]string) { m["APP_SECRET_KEY"] = "c2hvcnQ=" },
		"APP_SECRET_KEY nie b64":   func(m map[string]string) { m["APP_SECRET_KEY"] = "!!!nie-base64!!!" },
		"brak SESSION_SECRET":      func(m map[string]string) { delete(m, "SESSION_SECRET") },
		"SESSION_SECRET za krótki": func(m map[string]string) { m["SESSION_SECRET"] = "krotki" },
		"brak SMTP_HOST":           func(m map[string]string) { delete(m, "SMTP_HOST") },
		"brak SMTP_FROM":           func(m map[string]string) { delete(m, "SMTP_FROM") },
		"SMTP_FROM bez @":          func(m map[string]string) { m["SMTP_FROM"] = "niepoprawny" },
		"SMTP_PORT nieprawidłowy":  func(m map[string]string) { m["SMTP_PORT"] = "abc" },
		"MAX_UPLOAD_MB ujemny":     func(m map[string]string) { m["MAX_UPLOAD_MB"] = "-1" },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			env := validEnv()
			mutate(env)
			full := validEnv()
			for k := range full {
				if _, ok := env[k]; !ok {
					t.Setenv(k, "")
				}
			}
			setEnv(t, env)
			if _, err := Load(); err == nil {
				t.Errorf("oczekiwano błędu walidacji dla przypadku %q, ale go nie było", name)
			}
		})
	}
}
