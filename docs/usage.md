# Using git guess

## The command

```sh
git guess                       # what are my staged changes? (falls back to the working tree)
git guess -m "add login"        # feat(auth): add login
git guess HEAD~1                # a commit, or a range such as main..feature
git diff | git-guess            # any unified diff on stdin
git guess eval                  # how well does it do on this repository's own history?
```

When stdout is not a terminal, the output is exactly one line (the header), so it composes:

```sh
git commit -m "$(git guess -m 'add login')"
```

## The hook

```sh
git guess hook install
```

From now on, in this repository:

- `git commit -m "add login"` becomes `feat(auth): add login`
- a plain `git commit` opens your editor with `feat(auth): ` already on the first line, and the runner-up in a comment
- a subject that already has a type is left alone, and so are merge, squash, fixup, revert and amend messages
- when it is not sure, it asks once, in one keystroke:

```
? fix 41% or refactor 38%? [f/r, Enter keeps fix]
```

The hook never blocks a commit: if anything fails, your message goes through untouched. Skip it once with `GIT_GUESS_HOOK=0 git commit ...`, never be asked with `GIT_GUESS_ASK=0`, remove it with `git guess hook uninstall`. It respects `core.hooksPath`, so it lives happily next to husky or lefthook (add `git-guess hook run "$@"` to your existing `prepare-commit-msg`).

Prefer a wrapper to a hook? `git guess commit -m "add login"` runs `git commit -m "feat(auth): add login"` and passes every other argument through (`-a`, `-s`, `--amend`, `--no-verify`...).

## Gitmoji

Repositories that speak [gitmoji](https://gitmoji.dev) get the same guess, written the gitmoji way:

```sh
git guess --gitmoji -m "add login"        # ✨ (auth): add login
git guess --gitmoji=code -m "add login"   # :sparkles: (auth): add login
git config guess.gitmoji true             # from now on, every command and the hook (code for :shortcodes:)
git guess hook install --gitmoji          # installs the hook and sets that configuration
```

The type is still guessed by the classifier; the emoji is then narrowed down from the diff, and from the subject when there is one. Every file renamed gives 🚚, only deletions 🔥, a lone `.gitignore` 🙈, only `LICENSE` 📄, translation files 🌐, snapshots 📸, a `.d.ts` 🏷️, SQL migrations 🗃️, comment-only lines 💡, log lines 🔊/🔇. A dependency manifest (`package.json`, `go.mod`, `Cargo.toml`, `pyproject.toml`, `requirements.txt`, `composer.json`, `Gemfile`, `pubspec.yaml`, `build.gradle`, a `Dockerfile` base image, a GitHub Action `uses:`...) is read line by line: ➕ added, ➖ removed, ⬆️ upgraded, ⬇️ downgraded, 📌 pinned. A version bump with its changelog is 🔖. The subject adds "hotfix" 🚑️, "typo" ✏️, "a11y" ♿️, "wip" 🚧 and a few more. Otherwise the usual table applies: ✨ feat, 🐛 fix, 📝 docs, 🎨 style, ♻️ refactor, ⚡️ perf, ✅ test, 📦️ build, 👷 ci, 🔧 chore, ⏪️ revert.

It reads gitmoji too: a history written in gitmoji is learned from like a conventional one (`eval`, the local index and `check` all understand `✨ (auth): add login` and `:sparkles: add login`), and the hook leaves a subject that already starts with an emoji alone. The history also sets the house style: an emoji your repository (almost) never writes for a type is not proposed, so a project that says 💄 where the classifier says `style`, or 🐛 for every fix including security ones, gets its own habit. `--no-gitmoji` prints the conventional header regardless of the configuration.

How good is it? `scripts/eval_gitmoji.py` replays a repository's history and compares the emoji with the author's. On four public gitmoji repositories (gitmoji, gitmoji-cli, typebot, penpot) the emoji is the author's for 95% of the commits written by dependency bots and for 52% to 84% of the commits written by people; the rest is mostly intent the diff does not carry (🚸 or 🐛, 🎉 or ✨, 🚑️ or 🐛) plus the type errors of the classifier itself.


## Flags and exit codes

```
-m, --message <subject>   draft subject; prints "type(scope): subject"
-n, --top <n>             number of candidates shown (default 3)
-q, --quiet               print only the type
    --json                machine readable output
    --explain             show the closest past commits and the features that drove the decision
    --scope <name>        force the scope
    --no-scope            never add a scope
    --no-prior            do not learn from this repository's history
    --staged              only staged changes, no fallback to the working tree
    --unstaged            only unstaged changes
    --all                 everything since HEAD
    --min-confidence <p>  exit 3 when the confidence is below p (0..1)
    --gitmoji[=code]      gitmoji header, unicode or :shortcode: (see Gitmoji)
    --no-gitmoji          conventional header even when guess.gitmoji is set
    --no-color            disable colors
```

Exit codes: `0` ok, `1` error (no changes, not a diff, git failure), `3` confidence below `--min-confidence`.

`--json` gives `type`, `scope`, `confidence`, `header`, `candidates`, `source`, `files`, `added`, `removed`, `adapted_to_repo`, `history_commits`, with `--gitmoji` `emoji` and `emoji_code`, and with `--explain`, `nearest` and the driving features.

```json
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
  "adapted_to_repo": true,
  "history_commits": 1000
}
```

The scope is guessed from monorepo layouts (`packages/*`, `apps/*`, `crates/*`, `internal/*`...) and from the scopes already used in your log.
