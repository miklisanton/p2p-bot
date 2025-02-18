.PHONY: deps build network vet
build: network
	docker compose up --build -d
network: deps
	docker network create app-network || true
deps: vet
	docker build -t dependencies -f ./dependencies.Dockerfile .
vet:
	go vet ./...
