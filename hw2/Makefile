.PHONY: generate up down build

generate:
	mkdir -p api
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -generate types,chi-server,spec -package api api/openapi.yaml > api/api.gen.go

up:
	docker-compose up --build -d

down:
	docker-compose down

build: generate
	go build -o main ./cmd/app
