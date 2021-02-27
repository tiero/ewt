.PHONY: fmt run proto

## fmt: Go Format
fmt:
	@echo "Gofmt..."
	@if [ -n "$(gofmt -l .)" ]; then echo "Go code is not formatted"; exit 1; fi

## help: prints this help message
help:
	@echo "Usage: \n"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## run: run ewtd
run: 
	@echo "Run..."
	go run ./cmd/ewtd/main.go


## proto: generate protobuf admin stubs
proto:
	chmod u+x ./scripts/compile-proto-go
	./scripts/compile-proto-go