.PHONY: dev build test docker
dev:
	npm run build && go run .
build:
	npm run build && go build -o portal .
test:
	go test ./... && npm run build
docker:
	docker build -t azeroth-portal .
