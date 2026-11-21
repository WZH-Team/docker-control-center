# Docker Control Center 构建
.PHONY: build web test clean

build: web
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dcc .

web:
	pnpm --dir web install
	pnpm --dir web build

test:
	go vet ./...
	go test ./...

clean:
	rm -rf dcc dist web/node_modules
