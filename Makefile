.PHONY: deps build network vet

all: vet build
dev: vet builddev
prod: vet volume build

builddev: network
	docker compose  -f dev-docker-compose.yaml up --build -d
build: network
	docker compose up --build -d
network: deps
	docker network create app-network || true
deps:
	docker build -t dependencies -f ./dependencies.Dockerfile .

vet:
	go vet ./...

volume: down
	docker volume rm $$(basename $$PWD)_db_data || true
down:
	docker compose down
