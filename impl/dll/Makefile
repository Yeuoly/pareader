.PHONY: test tui dll dll-plain

GARBLE ?= garble
WINDOWS_CC ?= x86_64-w64-mingw32-gcc
DLL_CGO_CFLAGS := -Os -ffunction-sections -fdata-sections -flto -DHID_API_NO_EXPORT_DEFINE
DLL_CGO_LDFLAGS := -flto
DLL_LDFLAGS := -s -w -buildid= -extldflags=-Wl,--gc-sections

test:
	go test ./...

tui:
	mkdir -p build
	go build -o build/pareader ./cmd/reader

dll:
	mkdir -p build
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=$(WINDOWS_CC) \
		CGO_CFLAGS='$(DLL_CGO_CFLAGS)' CGO_LDFLAGS='$(DLL_CGO_LDFLAGS)' \
		$(GARBLE) -tiny build \
		-trimpath \
		-ldflags='$(DLL_LDFLAGS)' \
		-buildmode=c-shared \
		-o build/aimeio.dll \
		./cmd/aimeio
	$(RM) .h

dll-plain:
	mkdir -p build
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=$(WINDOWS_CC) \
		CGO_CFLAGS='$(DLL_CGO_CFLAGS)' CGO_LDFLAGS='$(DLL_CGO_LDFLAGS)' \
		go build \
		-trimpath \
		-gcflags='all=-l' \
		-ldflags='$(DLL_LDFLAGS)' \
		-buildmode=c-shared \
		-o build/aimeio.dll \
		./cmd/aimeio
