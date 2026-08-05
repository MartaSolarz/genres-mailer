package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"

	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

var stdinReader = bufio.NewReader(os.Stdin)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "błąd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		usage()

		return errors.New("brak polecenia")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("wczytanie konfiguracji: %w", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("otwarcie bazy: %w", err)
	}

	defer func() { _ = st.Close() }()

	ctx := context.Background()

	switch args[0] {
	case "create-user":
		return createUser(ctx, st, args[1:])
	case "disable-user":
		return setDisabled(ctx, st, args[1:], true)
	case "enable-user":
		return setDisabled(ctx, st, args[1:], false)
	case "import-samples":
		return importSamples(ctx, st, args[1:])
	default:
		usage()

		return fmt.Errorf("nieznane polecenie: %s", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Użycie:
  admin create-user <username>       — utwórz użytkownika (hasło z prompta)
  admin disable-user <username>      — zablokuj użytkownika
  admin enable-user <username>       — odblokuj użytkownika
  admin import-samples <plik.csv>    — import próbek (CSV: sample_id,email)`)
}

func createUser(ctx context.Context, st *store.Store, args []string) error {
	if len(args) != 1 {
		return errors.New("użycie: create-user <username>")
	}

	username := strings.TrimSpace(args[0])
	if username == "" {
		return errors.New("nazwa użytkownika nie może być pusta")
	}

	password, err := promptPassword()
	if err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("haszowanie hasła: %w", err)
	}

	if err := st.CreateUser(ctx, username, string(hash)); err != nil {
		return fmt.Errorf("nie udało się utworzyć użytkownika %q: %w", username, err)
	}

	fmt.Printf("Utworzono użytkownika %q.\n", username)

	return nil
}

func setDisabled(ctx context.Context, st *store.Store, args []string, disabled bool) error {
	if len(args) != 1 {
		return errors.New("użycie: (disable|enable)-user <username>")
	}

	if err := st.SetUserDisabled(ctx, args[0], disabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("użytkownik %q nie istnieje", args[0])
		}

		return fmt.Errorf("zmiana statusu użytkownika %q: %w", args[0], err)
	}

	action := "odblokowano"
	if disabled {
		action = "zablokowano"
	}

	fmt.Printf("Użytkownika %q %s.\n", args[0], action)

	return nil
}

func importSamples(ctx context.Context, st *store.Store, args []string) error {
	if len(args) != 1 {
		return errors.New("użycie: import-samples <plik.csv>")
	}

	f, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("otwarcie pliku CSV: %w", err)
	}

	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	var added, updated, rejected, line int

	for {
		line++

		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return fmt.Errorf("odczyt CSV w wierszu %d: %w", line, err)
		}

		if line == 1 && isHeader(record) {
			continue
		}

		sampleID, email, ok := parseRow(record)
		if !ok {
			rejected++

			fmt.Fprintf(os.Stderr, "wiersz %d odrzucony: nieprawidłowy format lub adres e-mail\n", line)

			continue
		}

		inserted, err := st.UpsertSample(ctx, sampleID, email)
		if err != nil {
			return fmt.Errorf("zapis próbki w wierszu %d: %w", line, err)
		}

		if inserted {
			added++
		} else {
			updated++
		}
	}

	fmt.Printf("Import zakończony: dodano %d, zaktualizowano %d, odrzucono %d.\n", added, updated, rejected)

	return nil
}

func parseRow(record []string) (sampleID, email string, ok bool) {
	if len(record) < 2 {
		return "", "", false
	}

	sampleID = strings.TrimSpace(record[0])
	email = strings.TrimSpace(record[1])

	if sampleID == "" {
		return "", "", false
	}

	addr, err := mail.ParseAddress(email)
	if err != nil {
		return "", "", false
	}

	return sampleID, addr.Address, true
}

func isHeader(record []string) bool {
	if len(record) < 2 {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(record[0]), "sample_id")
}

func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Hasło: ")

	pw, err := readSecret()
	if err != nil {
		return "", err
	}

	fmt.Fprint(os.Stderr, "Powtórz hasło: ")

	pw2, err := readSecret()
	if err != nil {
		return "", err
	}

	if pw != pw2 {
		return "", errors.New("hasła nie są identyczne")
	}

	if len(pw) < 12 {
		return "", errors.New("hasło musi mieć co najmniej 12 znaków")
	}

	return pw, nil
}

func readSecret() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)

		fmt.Fprintln(os.Stderr)

		if err != nil {
			return "", fmt.Errorf("odczyt hasła: %w", err)
		}

		return string(b), nil
	}

	line, err := stdinReader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("odczyt hasła: %w", err)
	}

	return strings.TrimRight(line, "\r\n"), nil
}
