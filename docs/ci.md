# git guess in CI

## The GitHub Action

```yaml
# .github/workflows/git-guess.yml
on:
  pull_request:
permissions:
  contents: read
  pull-requests: write
jobs:
  guess:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: Halleck45/git-guess@v1
```

That is enough to get every pull request labeled `enhancement`, `bug`, `documentation`, and so on, from its diff. Options:

| input | default | what it does |
| --- | --- | --- |
| `label` | `true` | add a label with the guessed type |
| `label-map` | `feat=enhancement,fix=bug,docs=documentation` | rename types to your labels; other types keep their name |
| `label-prefix` | `""` | prefix for the remaining types, e.g. `type: ` |
| `min-confidence` | `0.5` | do not label below this confidence |
| `comment` | `false` | keep one sticky comment with the guess and its alternatives |
| `check-commits` | `true` | annotate commits without a type, or with a type the diff disputes |
| `strict` | `false` | fail the job on disputed types too |

Outputs `type`, `scope`, `confidence` and `header` are available to later steps, for example to prefix the PR title or pick a release channel.

Behind the action is `git guess check`, a semantic linter you can run anywhere:

```
$ git guess check main..HEAD
  ✔ 3f2a1c0 fix(router): keep query on redirect
  ✘ 9b8e7d6 add retry option
      no type; suggestion: feat: add retry option
  ? 1c2d3e4 docs: handle null token
      declared docs but the diff looks like fix 91%

3 commits checked, 1 without type, 1 disputed
```

Unlike commitlint, it reads the diff. Exit code 1 when a commit has no type, `--strict` to fail on disputed ones too, `--github` for workflow annotations, `--json` for everything else. Without a range it checks the current branch against its upstream. Gitmoji subjects count as typed, and suggestions follow `--gitmoji` or `guess.gitmoji`.
