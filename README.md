# conventional

Guess the Conventional Commits type of a change from its diff. Pure Go, one binary, no network, about 10 ms per run.

```
$ git add .
$ conventional
✔ feat(auth)  ████████░░ 82%
  also: fix 11% · refactor 5%
  3 files, +48 −6, staged, tuned to this repo's history
```

You keep writing the subject. `conventional` writes the `feat(auth):` part, and it learns your repository's habits from `git log` before answering.

## Install

```sh
brew install Halleck45/tap/conventional
```

```sh
go install github.com/Halleck45/conventional/cmd/conventional@latest
```

Prebuilt binaries for Linux, macOS and Windows (amd64, arm64) are on the [releases page](https://github.com/Halleck45/conventional/releases).

## Usage

```sh
conventional                       # staged changes (falls back to the working tree)
conventional -m "add login"        # prints a complete header: feat(auth): add login
git diff | conventional            # any unified diff on stdin
conventional HEAD~1                # a commit, or a range such as main..feature
conventional eval                  # score it against your own history
```

When stdout is not a terminal, the output is exactly one line (the header), so it composes with anything.

### Commit with the type prefilled

```sh
conventional commit -m "add login"               # runs: git commit -m "feat(auth): add login"
git commit -m "$(conventional -m 'add login')"   # same thing, without the wrapper
```

`conventional commit` accepts the usual `git commit` arguments (`-a`, `--amend`, `--no-verify`...). Without `-m`, the editor opens with the header already on the first line.

### Or install the hook once and forget about it

```sh
conventional hook install
```

This writes a `prepare-commit-msg` hook (it respects `core.hooksPath`). From then on:

- `git commit -m "add login"` becomes `feat(auth): add login`
- interactive `git commit` opens the editor prefilled with `feat(auth): ` and a comment listing the runner-up
- a subject that already starts with a conventional type is left untouched
- merge, squash and amend messages are never rewritten

Skip it once with `CONVENTIONAL_HOOK=0 git commit -m "..."`. Remove it with `conventional hook uninstall`. The hook never blocks a commit: if the model or git fails, the message goes through as is.

### Check it on your own repository

```
$ conventional eval
replayed 248 commits of this repository
  top-1 75%   top-2 91%   (using this repo's history)

  chore      144/158  █████████░  guessed as docs 9, ci 2
  fix         11/23   █████░░░░░  guessed as chore 8, feat 2
  docs        17/18   █████████░  guessed as chore 1
  ...
```

`conventional eval` replays the last 200 conventional commits of the repository (`-n` changes that), guesses each one from its diff and compares with the type the author chose. Add `--with-message` to also feed it the subject line, as the hook does. The history prior is computed from commits older than the replayed window, so the score is what you would have seen at the time.

### Machine readable

```sh
$ conventional --json
{
  "type": "feat",
  "scope": "auth",
  "confidence": 0.82,
  "header": "feat(auth):",
  "candidates": [
    { "type": "feat", "p": 0.82 },
    { "type": "fix", "p": 0.11 },
    { "type": "refactor", "p": 0.05 }
  ],
  "source": "staged",
  "files": 3,
  "added": 48,
  "removed": 6,
  "adapted_to_repo": true
}
```

`--explain` adds the features that pushed the decision one way or the other:

```
$ conventional --explain
✔ test  █████████░ 91%
  ...
  why test rather than feat
    + directory tests                          +1.84
    + file name word test                      +1.21
    + added token assert                       +0.77
    − new identifier LoginService              -0.42
```

### Adapting to the repository

`conventional` reads the last 500 commit subjects of the current repository. When at least 30 of them follow Conventional Commits, it reweights its probabilities with the repository's own type distribution: a project that commits `chore` all day will get `chore` suggested more readily than one that never does. `--no-prior` disables this. Diffs read from stdin are never adapted.

The scope is guessed from monorepo layouts (`packages/*`, `apps/*`, `crates/*`, `internal/*`...) and from the scopes already used in the history. `--scope api` forces one, `--no-scope` removes it.

### Flags and exit codes

```
-m, --message <subject>   draft subject; prints "type(scope): subject"
-n, --top <n>             number of candidates shown (default 3)
-q, --quiet               print only the type
    --json                machine readable output
    --explain             show which features drove the decision
    --scope <name>        force the scope
    --no-scope            never add a scope
    --no-prior            do not adapt to this repository's history
    --staged              only staged changes, no fallback to the working tree
    --unstaged            only unstaged changes
    --all                 everything since HEAD
    --min-confidence <p>  exit 3 when the confidence is below p (0..1)
    --no-color            disable colors
```

Exit codes: `0` ok, `1` error (no changes, not a diff, git failure), `3` confidence below `--min-confidence`. The last one is handy in CI: `conventional --min-confidence 0.6 -q || echo "please pick the type yourself"`.

## How it works

1. A diff parser turns the patch into files, hunks and lines.
2. A featurizer produces about 90 dense structural features (file counts, added/removed ratios, languages, file kinds such as test, doc, config, lockfile, CI...) plus hashed n-grams of paths, code tokens, line shapes and hunk contexts (the function a hunk lives in). Feature hashing keeps the vocabulary unbounded and the binary small.
3. A multinomial logistic regression scores the 11 types. It was trained with scikit-learn on commits from about 240 open-source repositories that use Conventional Commits, across many languages and ecosystems, with every commit seen without its message and half of them a second time with it, so the model works in both situations. Probabilities are temperature-calibrated on held-out repositories so that 80% means roughly 80%.
4. Weights are pruned, quantized to int16 and embedded in the binary. The same Go featurizer is used for training (`go run ./cmd/featurize`) and inference, so there is no drift between the two.

## Accuracy

Evaluated on held-out repositories that were never seen during training (a split by repository, not by commit, so the numbers reflect what you get on a new project):

| Setting | Top-1 | Top-2 |
| --- | --- | --- |
| Diff only | 59% | 79% |
| Diff + repository history (the default inside a repository) | 63% | |
| Diff + subject line (`-m`) | 64% | 82% |
| Diff + history + subject line | 68% | |

Trained on 488,111 commits from 244 repositories (36 of them, 77,429 commits, held out for the numbers above). Probabilities are calibrated: when it says 80%, it is right about 85% of the time; below 40%, about a third.

The "easy" types are where it shines (docs 86% recall, test 77%, ci 66%), and the confusion is concentrated where humans disagree too: feat against fix, and chore against everything. Different projects label the same kind of change differently, which is exactly why it reads your history.

## Training your own

```sh
python3 scripts/collect.py          # probe and clone repositories, extract (diff, type) pairs into data/raw
go run ./cmd/featurize              # featurize with the exact code used at inference, into data/features
python3 scripts/train.py --final    # train, calibrate, quantize, export internal/model/model.bin
go build ./cmd/conventional         # the new model is embedded
```

`scripts/repos.txt` is the seed list; `scripts/collect.py` keeps the repositories whose recent history is mostly conventional.

## Limitations

- feat, fix and refactor are inherently ambiguous from a diff alone. A three-line change can be any of them; only the author knows the intent. The subject line (`-m`) helps a lot.
- revert is rarely detectable without the message.
- It is a suggestion, not a linter. Use commitlint or similar if you need enforcement.

## License

MIT, see [LICENSE](LICENSE).
