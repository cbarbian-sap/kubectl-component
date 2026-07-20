BINARY := kubectl-component
BIN_DIR := bin

.PHONY: build
build:
	go build -o $(BIN_DIR)/$(BINARY) ./cmd/$(BINARY)

.PHONY: install
install:
	go install ./cmd/$(BINARY)

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
