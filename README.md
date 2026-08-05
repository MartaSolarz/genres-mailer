# genres-mailer

Wewnętrzna aplikacja do bezpiecznej wysyłki zaszyfrowanych wyników badań
genetycznych. Genetyk loguje się, wybiera ID próbki (powiązane z adresem e-mail
odbiorcy), wgrywa PDF. Aplikacja szyfruje PDF (AES-256), a po zatwierdzeniu
wysyła **dwa osobne e-maile**: jeden z zaszyfrowanym załącznikiem, drugi z hasłem.

- Backend: Go (tylko biblioteka standardowa + minimum zależności, bez CGO)
- Baza: SQLite (`modernc.org/sqlite`), migracje przy starcie
- Szyfrowanie PDF: `pdfcpu` (AES-256); hasła kont: `bcrypt` (cost 12)
- Frontend: HTML + vanilla JS, osadzony w binarce; **zero zasobów z CDN**
- TLS/reverse proxy w produkcji: Caddy (aplikacja nie jest wystawiona wprost)

---

## Decyzje wdrożeniowe (odpowiedzi z UW → co ustawić)

Aplikacja jest przygotowana pod oba warianty każdej odpowiedzi. Poniżej mapa:

| Pytanie do UW | Odpowiedź | Co ustawiasz |
|---|---|---|
| **1. SSH + sudo** | tak | Dowolny wariant: Docker Compose (zalecany) lub systemd |
| **2. Porty 80/443, dostęp** | z internetu | `SITE_ADDRESS=domena` → Caddy uzyska cert **Let's Encrypt** automatycznie |
| | tylko UW/VPN | Caddyfile: `tls internal` (wewnętrzne CA) **lub** cert od DSK (`tls cert key`) |
| **3. Domena + TLS** | subdomena + Let's Encrypt | nic więcej — Caddy załatwia automatycznie |
| | cert centralny (DSK) | zamontuj cert/klucz do Caddy i odkomentuj `tls` w `deploy/Caddyfile` |
| **4. Relay SMTP UW** | bez auth | `SMTP_HOST`/`SMTP_PORT` = relay UW; `SMTP_USER`/`SMTP_PASS` puste |
| | z auth | dodatkowo `SMTP_USER`/`SMTP_PASS` → aplikacja użyje **STARTTLS + auth** |

Adres nadawcy (`SMTP_FROM`) ustaw na adres w domenie UW, aby poczta wychodziła
z zaufanej infrastruktury (pytanie 4 — najważniejsze). Aplikacja wysyła
pojedyncze wiadomości, więc obciążenie relaya jest znikome.

---

## 1. Konfiguracja (wspólna dla obu wariantów)

```bash
cp .env.example .env
chmod 600 .env

# Wygeneruj sekrety (32 bajty, base64):
openssl rand -base64 32   # wklej do APP_SECRET_KEY
openssl rand -base64 32   # wklej do SESSION_SECRET
```

Uzupełnij w `.env`: `SMTP_HOST`, `SMTP_PORT`, `SMTP_FROM` (oraz `SMTP_USER`/
`SMTP_PASS`, jeśli relay wymaga auth), `ORG_NAME`, a dla Dockera — `SITE_ADDRESS`.

---

## 2. Wariant A — Docker Compose (zalecany)

Wymaga: Docker + wtyczka `compose`. Aplikacja działa jako nonroot (distroless),
nie ma publikowanych portów — dostęp wyłącznie przez Caddy (TLS).

```bash
# Ustaw SITE_ADDRESS w .env, następnie:
make docker-up          # == docker compose up -d --build
```

Utwórz pierwszego użytkownika i zaimportuj próbki (spójne uprawnienia do wolumenu):

```bash
make docker-admin ARGS="create-user genetyk"          # zapyta o hasło
# import CSV: skopiuj plik do wolumenu 'data' albo zamontuj i wskaż /data/...
make docker-admin ARGS="import-samples /data/samples.csv"
```

Podgląd logów: `make docker-logs`. Zatrzymanie: `make docker-down`.

**Certyfikat TLS** — patrz komentarze w `deploy/Caddyfile`:
- publiczna domena → nic nie rób (Let's Encrypt automatycznie),
- cert od DSK → zamontuj pliki i odkomentuj `tls /etc/caddy/tls/cert.pem /etc/caddy/tls/key.pem`,
- tylko sieć wewnętrzna → odkomentuj `tls internal`.

Restart VM: usługi mają `restart: unless-stopped` — wstaną automatycznie.

---

## 3. Wariant B — systemd (bez Dockera)

```bash
# 1. Zbuduj statyczne binarki
make build
sudo install -m 0755 bin/server /usr/local/bin/genres-mailer-server
sudo install -m 0755 bin/admin  /usr/local/bin/genres-mailer-admin

# 2. Użytkownik systemowy i katalog danych
sudo useradd --system --home /var/lib/genres-mailer --shell /usr/sbin/nologin genres-mailer
sudo install -d -o genres-mailer -g genres-mailer -m 0700 /var/lib/genres-mailer

# 3. Plik środowiska (sekrety) — ustaw DB_PATH/DATA_DIR na katalog danych
sudo install -d -m 0755 /etc/genres-mailer
sudo cp .env /etc/genres-mailer/env
sudo sed -i 's#^DB_PATH=.*#DB_PATH=/var/lib/genres-mailer/app.db#; \
             s#^DATA_DIR=.*#DATA_DIR=/var/lib/genres-mailer#; \
             s#^LISTEN_ADDR=.*#LISTEN_ADDR=127.0.0.1:8080#' /etc/genres-mailer/env
sudo chmod 600 /etc/genres-mailer/env

# 4. Usługa
sudo cp deploy/genres-mailer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now genres-mailer

# 5. Pierwszy użytkownik (jako użytkownik usługi)
sudo -u genres-mailer env $(sudo grep -v '^#' /etc/genres-mailer/env | xargs) \
     genres-mailer-admin create-user genetyk
```

Przed aplikacją postaw Caddy (na tym samym hoście) z `reverse_proxy 127.0.0.1:8080`
i sekcją `tls` odpowiednią do wariantu (patrz `deploy/Caddyfile`). Restart VM:
`systemctl enable` zapewnia automatyczny start.

> Uwaga: unit używa stałego użytkownika `genres-mailer` (spójne uprawnienia do
> bazy + proste zarządzanie kontami). W pliku `.service` jest zakomentowany
> wariant `DynamicUser=yes` dla wdrożeń, gdzie konta tworzy się tylko przez Dockera.

---

## 4. Zarządzanie kontami i próbkami

CLI `admin` (Docker: `make docker-admin ARGS="..."`, systemd: `genres-mailer-admin ...`):

| Polecenie | Działanie |
|---|---|
| `create-user <login>` | utwórz konto (hasło z prompta, min. 12 znaków) |
| `set-password <login>` | zmień hasło |
| `disable-user <login>` / `enable-user <login>` | zablokuj / odblokuj |
| `import-samples <plik.csv>` | import próbek (CSV: `sample_id,email`), upsert po `sample_id` |

CSV z próbkami — nagłówek opcjonalny:

```csv
sample_id,email
PROBKA-001,jan.kowalski@example.org
```

---

## 5. Aktualizacja

- **Docker:** `git pull && make docker-up` (przebuduje obraz i wymieni kontener; wolumen danych zostaje).
- **systemd:** `git pull && make build && sudo install -m0755 bin/server /usr/local/bin/genres-mailer-server && sudo systemctl restart genres-mailer`.

Migracje bazy uruchamiają się automatycznie przy starcie (idempotentne).

## 6. Kopia zapasowa

Wystarczy katalog danych (baza + zaszyfrowane pliki):
- Docker: wolumen `data`
- systemd: `/var/lib/genres-mailer`

Zaszyfrowane PDF-y i hasła są usuwane/zerowane po `FILE_RETENTION_HOURS` (domyślnie 72h);
wpisy w historii pozostają.

---

## Bezpieczeństwo — uwagi

- Sekrety (`.env` / `/etc/genres-mailer/env`) trzymaj z uprawnieniami `600`.
  Rotacja `APP_SECRET_KEY` uniemożliwi odczyt haseł zapisanych wcześniej — rób ją
  tylko gdy nie ma aktywnych, jeszcze niewysłanych dokumentów.
- **Relay bez auth wysyła mail z hasłem tekstem jawnym.** Załącznik jest
  bezpieczny (AES-256), ale przy relayu bez STARTTLS podsłuch na trasie mógłby
  przechwycić hasło. Jeśli relay UW wspiera auth/STARTTLS — użyj go
  (`SMTP_USER`/`SMTP_PASS`). W sieci zamkniętej UW ryzyko jest ograniczone.
- Aplikacja nasłuchuje tylko lokalnie/wewnętrznie; ruch z internetu obsługuje
  wyłącznie Caddy (TLS + HSTS). Nagłówki bezpieczeństwa (CSP `default-src 'self'`,
  X-Frame-Options, nosniff, Referrer-Policy, no-store) ustawia sama aplikacja.

## Rozwój

```bash
make run                 # lokalnie (wczytuje .env)
make check               # lint + testy + govulncheck
make docker-up           # pełny stack z Caddy
```

Testy i statyczna analiza działają też w CI (`.github/workflows/ci.yml`).
