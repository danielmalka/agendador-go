# -race needs cgo and a C compiler.
RACE := $(shell [ "$$(go env CGO_ENABLED)" = 1 ] && command -v gcc >/dev/null 2>&1 && echo -race)
MIN_COVER := 80

.PHONY: build test cover vet run

build:
	go build -o bin/scheduler ./cmd/scheduler

test:
	go test ./... $(RACE) -count=1

cover:
	go test ./... -count=1 -coverprofile=coverage.out
	@go tool cover -func=coverage.out | awk -v min=$(MIN_COVER) '/^total:/ { gsub("%","",$$3); printf "total coverage: %s%%\n", $$3; if ($$3+0 < min) { print "FAIL: below " min "%"; exit 1 } }'

vet:
	go vet ./...

run:
	go run ./cmd/scheduler -config configs/jobs.yml
