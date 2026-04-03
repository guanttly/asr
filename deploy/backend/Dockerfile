FROM golang:1.25 AS builder

ARG APP_NAME

WORKDIR /app

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /app/backend
RUN go mod download

COPY backend/ /app/backend/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/${APP_NAME} ./cmd/${APP_NAME}

FROM debian:bookworm-slim

ARG APP_NAME

WORKDIR /app

COPY --from=builder /out/${APP_NAME} /app/${APP_NAME}
COPY backend/configs /app/configs
COPY backend/migrations /app/migrations

EXPOSE 10010 10011 10012 10013

CMD ["sh", "-c", "./${APP_NAME}"]