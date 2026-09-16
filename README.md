<p align="center">
  <img src="docs/demo.svg" width="680" alt="git guess in a terminal: it prints feat(auth) 82%, then git commit -m 'add login' becomes feat(auth): add login">
</p>

<h1 align="center">git guess</h1>

<p align="center"><b>feat or fix? Let git guess.</b><br>
It reads the diff, learns how your repository names things, and writes the Conventional Commits type for you.<br>
One binary. No API key. Nothing leaves your machine. About 30 ms.</p>

<p align="center">
  <a href="https://github.com/Halleck45/git-guess/actions/workflows/ci.yml"><img src="https://github.com/Halleck45/git-guess/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Halleck45/git-guess/releases"><img src="https://img.shields.io/github/v/release/Halleck45/git-guess?color=6f42c1" alt="Release"></a>
  <a href="https://github.com/marketplace/actions/git-guess"><img src="https://img.shields.io/badge/GitHub%20Action-git--guess-6f42c1?logo=githubactions&logoColor=white" alt="GitHub Action"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT"></a>
</p>

---

Every team that adopts [Conventional Commits](https://www.conventionalcommits.org) hits the same wall: the format is easy, the *type* is not. Is this a `fix` or a `refactor`? Does a dependency bump count as `build` or `chore`? Everyone answers differently, linters only check the syntax, and the changelog ends up lying.

`git guess` answers from the one thing that never lies: the diff. It was trained on 488,000 commits from 244 open-source projects, and it keeps learning from the last 1,000 commits of *your* repository, so it picks up your conventions instead of imposing its own.

## Install

```sh
brew install Halleck45/tap/git-guess
```

```sh
curl -fsSL https://raw.githubusercontent.com/Halleck45/git-guess/main/install.sh | sh
```

```sh
go install github.com/Halleck45/git-guess/cmd/git-guess@latest
```

Prebuilt binaries for Linux, macOS and Windows (amd64, arm64) are on the [releases page](https://github.com/Halleck45/git-guess/releases). The binary is called `git-guess`, which is why `git guess` just works.

## Sixty seconds

```sh
git guess                       # what are my staged changes?
git guess -m "add login"        # feat(auth): add login
git guess HEAD~1                # a commit, or a range such as main..feature
git guess eval                  # how well does it do on this repository's own history?
```

Then forget about it:

```sh
git guess hook install
```

From now on `git commit -m "add login"` becomes `feat(auth): add login`, a plain `git commit` opens your editor with the type prefilled, and when it is not sure it asks once, in one keystroke: `? fix 41% or refactor 38%? [f/r]`. The hook never blocks a commit.

## In CI

```yaml
- uses: Halleck45/git-guess@v1     # labels every pull request from its diff, lints its commits
```

## How good is it

On 36 repositories it had never seen, guessing each commit from the diff and the commits before it: **70% top-1, 88% top-2**. Above 90% confidence it is right 96% of the time. Run `git guess eval` in your own repository to see what to expect there.

The remaining errors are almost all feat against fix against refactor: the same three-line patch is a fix for one author and a refactor for another, and the diff does not carry intent. That is why it asks when unsure, and why it learns from your history rather than imposing a convention.

## Learn more

- [Using git guess](docs/usage.md): every command, flag, exit code, the hook, JSON output
- [In CI](docs/ci.md): the GitHub Action, labeling, `git guess check`
- [How it learns your repository](docs/learning.md): the local index, `eval`, the numbers in full
- [How it works](docs/how-it-works.md): the pipeline, and how to train your own model
- [FAQ](docs/faq.md): privacy, speed, Windows, and how it compares to commitizen, commitlint and AI commit writers

## License

MIT, see [LICENSE](LICENSE).
