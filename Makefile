lint:
	gofumpt -l -w .
	golangci-lint run
