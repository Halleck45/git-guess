.PHONY: build test lint collect featurize train release-snapshot

build:
	go build -o conventional ./cmd/conventional

test:
	go test ./...

lint:
	go vet ./...

collect:
	python3 scripts/collect.py

featurize:
	go run ./cmd/featurize -max-per-repo 3000 -max-bot-per-repo 100

train:
	python3 scripts/train.py --alpha 1e-5 --epochs 25 --final

release-snapshot:
	goreleaser release --snapshot --clean
