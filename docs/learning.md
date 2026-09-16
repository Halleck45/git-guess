# How it learns your repository

## Local learning

The first time it runs in a repository, `git guess` indexes the last 1,000 conventional commits (a second or two, once; the index lives in `.git/git-guess/` and is refreshed incrementally). Every guess then weighs the global model against three local pieces of evidence:

- the types of the past diffs most similar to yours,
- what this repository tends to call what the global model guesses (a project that says `chore` for CI changes gets `chore`),
- the types of past commits that touched the same files.

You can watch it work:

```
$ git guess eval
replayed 250 commits of this repository, each scored with only the commits before it
  top-1 79%   top-2 90%   (global model alone: 78%)

  chore      147/159  █████████░  guessed as fix 6, docs 5
  fix         18/24   ████████░░  guessed as chore 3, feat 3
  docs        16/18   █████████░  guessed as chore 2
  ...
```

`eval` replays your history in order, guessing each commit from the ones before it, and compares with what the author chose. Run it before installing the hook: it tells you exactly what to expect on your project.

`--explain` shows the closest past commits and the features that drove a decision; `--no-prior` turns the local learning off. Diffs read from stdin are never adapted.



## Accuracy

Measured on 36 repositories the model had never seen (77,000 commits, split by repository, not by commit), scoring each commit with only the commits before it:

| Setting | Top-1 | Top-2 |
| --- | --- | --- |
| Diff only, no history (stdin, brand-new repository) | 59% | 79% |
| Diff + your repository's history (the default) | **70%** | **88%** |

Probabilities are calibrated: above 90% confidence it is right 96% of the time; below 50% about 44%, which is why it asks instead of guessing silently.

The remaining errors are almost all `feat` against `fix` against `refactor`, and `chore` against everything. That is not a modelling gap: the same three-line patch is a fix for one author and a refactor for another, and the diff does not carry intent. The easy types are where it shines (docs 86% recall, test 77%, ci 66% before local learning, higher after). Different projects label the same change differently, which is exactly why it reads your history.
