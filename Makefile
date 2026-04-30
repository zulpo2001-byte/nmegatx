APP=nme-v9

.PHONY: dev build run-worker tidy seed migrate

dev:
	go run ./cmd/server

run-worker:
	go run ./cmd/worker

build:
	go build -o nme-server ./cmd/server
	go build -o nme-worker ./cmd/worker
	go build -o nme-seed ./cmd/seed
	go build -o nme-migrate ./cmd/migrate

seed:
	go run ./cmd/seed

migrate:
	go run ./cmd/migrate

tidy:
	go mod tidy
