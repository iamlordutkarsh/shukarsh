# --- Build stage ---
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o shukarsh-server ./cmd/srv

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/shukarsh-server .
COPY --from=builder /app/srv/templates ./srv/templates
COPY --from=builder /app/srv/static ./srv/static
COPY --from=builder /app/db/migrations ./db/migrations

RUN mkdir -p /data/uploads

ENV ADMIN_PASSWORD=""
EXPOSE 8080

CMD ["./shukarsh-server", "--listen", ":8080"]
