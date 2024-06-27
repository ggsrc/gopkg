NAME=gopkg
TEST_DIRS := $(shell go list -f '{{.Dir}}' -m | xargs -I {} go list {}/...)
REDIS_DOCKER_NAME=$(NAME)-redis
REDIS_PORT=6379
POSTGRES_DOCKER_NAME=$(NAME)-postgres
POSTGRES_PASSWORD=my-secret
POSTGRES_DB=$(NAME)_test_db
POSTGRES_PORT=5432
APPNAME=$(NAME)_test

lint:
	@echo "--> Running linter"
	@go list -f '{{.Dir}}' -m | xargs -I {} golangci-lint run {}/... --verbose

lint-fix:
	@echo "--> Running linter auto fix"
	@go list -f '{{.Dir}}' -m | xargs -I {} golangci-lint run {}/... --fix

build:
	@go list -f '{{.Dir}}' -m | xargs -I {} go build -v {}/...


test:
	@export ENV=test && \
		go test github.com/ggsrc/gopkg/...

codecov:
	export ENV=test && \
		go test ${TEST_DIRS} -coverprofile=coverage.txt -covermode=atomic -p 1


docker-redis-start:
	docker run -d --name $(REDIS_DOCKER_NAME) -p $(REDIS_PORT):6379 redis

docker-redis-stop:
	docker stop $(REDIS_DOCKER_NAME)
	docker rm $(REDIS_DOCKER_NAME)

docker-postgres-start:
	docker run -d --name $(POSTGRES_DOCKER_NAME) -e POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) -e POSTGRES_DB=$(POSTGRES_DB) -p $(POSTGRES_PORT):5432 postgres:14.5

docker-postgres-stop:
	docker stop $(POSTGRES_DOCKER_NAME)
	docker rm $(POSTGRES_DOCKER_NAME)

proto:
	protoc --go_out=. \
			-I=. \
			--go_opt=paths=source_relative \
			--go-grpc_out=require_unimplemented_servers=false:. \
			--go-grpc_opt=paths=source_relative \
			interceptor/grpc/errors/*.proto