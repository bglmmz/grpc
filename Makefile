all: vet test testrace testappengine

build: deps
	go build github.com/bglmmz/grpc/...

clean:
	go clean -i github.com/bglmmz/grpc/...

deps:
	go get -d -v github.com/bglmmz/grpc/...

proto:
	@ if ! which protoc > /dev/null; then \
		echo "error: protoc not installed" >&2; \
		exit 1; \
	fi
	go generate github.com/bglmmz/grpc/...

test: testdeps
	go test -cpu 1,4 -timeout 5m github.com/bglmmz/grpc/...

testappengine: testappenginedeps
	goapp test -cpu 1,4 -timeout 5m github.com/bglmmz/grpc/...

testappenginedeps:
	goapp get -d -v -t -tags 'appengine appenginevm' github.com/bglmmz/grpc/...

testdeps:
	go get -d -v -t github.com/bglmmz/grpc/...

testrace: testdeps
	go test -race -cpu 1,4 -timeout 7m github.com/bglmmz/grpc/...

updatedeps:
	go get -d -v -u -f github.com/bglmmz/grpc/...

updatetestdeps:
	go get -d -v -t -u -f github.com/bglmmz/grpc/...

vet: vetdeps
	./vet.sh

vetdeps:
	./vet.sh -install

.PHONY: \
	all \
	build \
	clean \
	deps \
	proto \
	test \
	testappengine \
	testappenginedeps \
	testdeps \
	testrace \
	updatedeps \
	updatetestdeps \
	vet \
	vetdeps
