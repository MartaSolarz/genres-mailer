# --- etap budowania ---
FROM golang:1.25 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Statyczna binarka bez CGO; szablony, statyki i migracje są osadzone w binarce.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin

# Katalog danych z właścicielem = użytkownik nonroot obrazu distroless (uid 65532).
# Świeży wolumen nazwany zamontowany w tym miejscu odziedziczy te uprawnienia.
RUN mkdir -p /image-data/tmp /image-data/encrypted && chown -R 65532:65532 /image-data

# --- obraz finalny ---
FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/server /server
COPY --from=build /out/admin /admin
COPY --from=build --chown=65532:65532 /image-data /data

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/server"]
