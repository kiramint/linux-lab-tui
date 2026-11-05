.PHONY: build test run list clean tidy

BINARY=linux-lab

build: tidy
	go build -o $(BINARY) .

tidy:
	go mod tidy

test:
	go test ./...

run: build
	./$(BINARY)

list: build
	./$(BINARY) list

clean:
	rm -f $(BINARY)
