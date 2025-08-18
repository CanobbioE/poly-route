PHONY:
GO111MODULE=on

default:

fmt:
	@gofmt -s -w $$(go list -f "{{.Dir}}" ./...)

gci:
	@gci write -s standard -s default -s "prefix(github.com/CanobbioE/poly-route)" -s blank -s dot ./internal ./example .

lint-all:
	@golangci-lint run --timeout 2m0s ./...

lint-diff:
	@golangci-lint run --new-from-rev=$$(git merge-base HEAD main) --timeout 2m0s ./...

start-mock:
	go run ./example/

generate-proto: _proto fmt gci

_proto:
	@buf lint
	@buf format --write
	@buf generate

install-tools:
	@echo Installing tools
	@go install github.com/bufbuild/buf/cmd/buf@latest
	@go install github.com/bufbuild/buf/cmd/protoc-gen-buf-lint@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/daixiang0/gci@latest
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo Installation completed

build:
	docker build -t canobbioe/poly-route:latest -f Dockerfile .

publish:
	- docker rmi canobbioe/poly-route
	docker build -t canobbioe/poly-route:latest -f Dockerfile .
	docker push canobbioe/poly-route

test:
	- docker compose down
	- docker rmi canobbioe/poly-route
	- docker rmi canobbioe/example-servers
	- docker rmi canobbioe/example-client
	docker compose build
	docker compose up


