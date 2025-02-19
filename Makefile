.PHONY: deps build network vet

all: vet build
prod: vet volume build

build: network
	docker compose up --build -d
network: deps
	docker network create app-network || true
deps:
	docker build -t dependencies -f ./dependencies.Dockerfile .

vet:
	go vet ./...

volume: down
	docker volume rm p2phub-backend_db_data
down:
	docker compose down
