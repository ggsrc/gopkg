TEST_DIRS := $(shell go list -f '{{.Dir}}' -m | xargs -I {} go list {}/...)

lint:
	@echo "--> Running linter"
	@golangci-lint run

lint-fix:
	@echo "--> Running linter auto fix"
	@golangci-lint run --fix

gen-readme:
	@echo "--> Running readmeai"
	@readmeai


build:
	@go list -f '{{.Dir}}' -m | xargs -I {} go build -v {}/...


test:
	@go list -f '{{.Dir}}' -m | xargs -I {} go test -v {}/...

codecov:
	export ENV=test && \
		go test ${TEST_DIRS} -coverprofile=coverage.txt -covermode=atomic -p 1