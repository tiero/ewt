.PHONY: fmt run

## fmt: Go Format
fmt:
	@echo "Gofmt..."
	@if [ -n "$(gofmt -l .)" ]; then echo "Go code is not formatted"; exit 1; fi

## run: run ewtd
run: 
	@echo "Run..."
	go run ./cmd/ewtd/main.go