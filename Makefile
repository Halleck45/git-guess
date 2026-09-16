.PHONY: build test lint collect featurize train train-meta release-snapshot

build:
	go build -o git-guess ./cmd/git-guess

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

train-meta:
	python3 scripts/experiments/exp_local2.py 1000 15
	python3 scripts/train_meta.py --final

release-snapshot:
	goreleaser release --snapshot --clean
