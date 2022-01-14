VERSION=$(shell git describe --tags --candidates=1 --dirty 2>/dev/null || echo "dev")
FLAGS=-s -w -X main.Version=$(VERSION)
SRC=$(shell find . -type f -name '*.go' -not -path "./vendor/*")

ecs-run-task: $(SRC)
	go build -o ecs-run-task -ldflags="$(FLAGS)" -v .

.PHONY: test
test:
	gofmt -s -l -w $(SRC)
	go vet -v ./...
	go test -race -v ./...

.PHONY: clean
clean:
	rm -f ecs-run-task

.PHONY: release
release:
	go install github.com/mitchellh/gox@latest
	gox -ldflags="$(FLAGS)" -output="build/{{.Dir}}-{{.OS}}-{{.Arch}}" -os="darwin linux windows" -arch="amd64 arm64" .
