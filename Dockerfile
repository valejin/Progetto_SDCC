# --- FASE 1: La Build ---
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /gossip-app .

# --- FASE 2: L'Immagine Finale ---
FROM alpine:latest
COPY --from=builder /gossip-app /gossip-app
ENTRYPOINT ["/gossip-app"]