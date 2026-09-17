# FAQ

**Does it send my code anywhere?** No. There is no network code in the binary.

**Does it slow down `git commit`?** About 30 ms once the history is indexed, a second or two on the very first run in a repository.

**My repository does not use Conventional Commits yet.** Then there is no history to learn from and you get the global model (59% top-1, 79% top-2). Accuracy climbs as your history grows; `git guess eval` shows where you stand.

**Windows?** Yes, the hook runs under Git for Windows' shell. The interactive question needs a terminal and is skipped in editors and CI.

**Can I disable the question, the scope, the local learning?** `GIT_GUESS_ASK=0`, `--no-scope`, `--no-prior`.

**Exit codes?** `0` ok, `1` error (no changes, not a diff, git failure), `3` confidence below `--min-confidence`. `git guess --min-confidence 0.6 -q || echo "pick it yourself"`.

**Machine readable?** `--json` gives `type`, `scope`, `confidence`, `header`, `candidates`, `source`, `files`, `added`, `removed`, `adapted_to_repo`, `history_commits` and, with `--explain`, `nearest` and the driving features.

**Gitmoji?** `--gitmoji` writes `✨ (auth): add login` instead of `feat(auth): add login`, `git config guess.gitmoji true` makes it the default for the hook too, and a history written in gitmoji is learned from like a conventional one. Details in [Using git guess](usage.md#gitmoji).



## Compared to other tools

| | git guess | commitizen / cz | commitlint | AI commit writers |
| --- | --- | --- | --- | --- |
| picks the type for you | from the diff | you pick from a menu | no, checks syntax only | from the diff, via an LLM |
| learns your repository | yes | no | no | no |
| offline, no API key | yes | yes | yes | no |
| latency | ~30 ms | interactive | ~200 ms (node) | seconds, costs money |
| lints existing commits | semantically | no | syntactically | no |
| labels pull requests | yes | no | no | no |

They compose: commitizen for the interactive flow, commitlint for enforcement, `git guess` to fill in the type.

## Changelog

Releases are on the [releases page](https://github.com/Halleck45/git-guess/releases).
