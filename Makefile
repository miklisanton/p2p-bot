.PHONY: deps build network vet
all: vet build
build: network
	docker compose up --build -d
network: deps
	docker network create app-network || true
deps:
	docker build -t dependencies -f ./dependencies.Dockerfile .

vet:
	go vet ./...
