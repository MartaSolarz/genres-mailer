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

## Rozwój

```bash
make run                 # lokalnie (wczytuje .env)
make check               # lint + testy + govulncheck
make docker-up           # pełny stack z Caddy
```

Testy i statyczna analiza działają też w CI (`.github/workflows/ci.yml`).
