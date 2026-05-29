BINARIES := dispatcher gatherer researcher planner designer coder committer

.PHONY: build clean $(BINARIES)

build: $(BINARIES)

$(BINARIES):
	CGO_ENABLED=0 go build -o bin/$@ ./cmd/$@/

clean:
	rm -rf bin/

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

check: fmt vet test
