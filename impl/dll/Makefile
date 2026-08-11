.PHONY: test tui dll

test:
	go test ./...

tui:
	mkdir -p build
	go build -o build/pareader ./cmd/reader

dll:
	mkdir -p build
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build \
		-buildmode=c-shared \
		-o build/aimeio.dll \
		./cmd/aimeio
