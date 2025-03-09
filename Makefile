run: fmt
	go run . config.yaml

build: fmt
	go build

fmt:
	go fmt *.go
	go fmt SimpleQueueMailing/*.go
