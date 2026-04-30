FROM golang:1.24-alpine AS builder
ENV CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/nme-server  ./cmd/server  && \
    go build -o /out/nme-worker  ./cmd/worker  && \
    go build -o /out/nme-migrate ./cmd/migrate && \
    go build -o /out/nme-seed    ./cmd/seed

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/ ./
COPY frontend/   ./frontend/
COPY migrations/ ./migrations/
COPY .env.example .env.example
EXPOSE 8080
CMD ["./nme-server"]
