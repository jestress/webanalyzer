lint:
	# run golangci-lint
	golangci-lint run --timeout 5m

test:
	# run all unit tests with coverage
	go test -v -cover ./...
