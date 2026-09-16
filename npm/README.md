# git guess

feat or fix? Let git guess. It reads the diff and writes the Conventional Commits type, the way your repository would.

```sh
npx git-guess                 # type for the staged changes
npm install -g git-guess      # then `git guess` works, like any git subcommand
git guess hook install        # git commit -m "add login" becomes feat(auth): add login
```

This package downloads the prebuilt binary for your platform from the
[GitHub release](https://github.com/Halleck45/git-guess/releases) on first use and
caches it under `~/.cache/git-guess`. Nothing else leaves your machine.

Documentation and source: https://github.com/Halleck45/git-guess
