package features

import (
	"path"
	"strings"
)

// Category is a coarse role of a file in a repository. Categories are the
// backbone of the dense features (fraction of files per category) and are
// also emitted as hashed tokens.
type Category uint8

const (
	CatSource Category = iota
	CatTest
	CatDocs
	CatCI
	CatBuild     // build system, dependency manifests, dockerfiles
	CatLock      // lock files
	CatConfig    // linters, editor, formatter configs
	CatStyle     // css/scss/less
	CatI18n      // translations
	CatAsset     // images, fonts, binaries
	CatSnapshot  // test snapshots / fixtures / golden files
	CatGenerated // vendored or generated
	CatChangelog
	CatLicense
	CatData // json/yaml/csv data files not covered elsewhere
	CatTemplate
	CatScript  // shell scripts, makefile-like helpers
	CatSchema  // sql migrations, proto, graphql, openapi
	CatVersion // version manifests (VERSION, version.go, __version__.py)
	numCategories
)

var categoryNames = [...]string{"source", "test", "docs", "ci", "build", "lock", "config", "style", "i18n", "asset",
	"snapshot", "generated", "changelog", "license", "data", "template", "script", "schema", "version"}

func (c Category) String() string { return categoryNames[c] }

var lockFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true, "bun.lock": true,
	"go.sum": true, "cargo.lock": true, "poetry.lock": true, "pipfile.lock": true, "composer.lock": true,
	"gemfile.lock": true, "pdm.lock": true, "uv.lock": true, "flake.lock": true, "packages.lock.json": true,
	"pubspec.lock": true, "mix.lock": true, "gradle.lockfile": true, "podfile.lock": true, "shrinkwrap.yaml": true,
	"npm-shrinkwrap.json": true, "requirements.lock": true, "deno.lock": true, "renv.lock": true,
}

var buildFiles = map[string]bool{
	"package.json": true, "go.mod": true, "cargo.toml": true, "pyproject.toml": true, "setup.py": true, "setup.cfg": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "settings.gradle": true, "settings.gradle.kts": true,
	"composer.json": true, "gemfile": true, "makefile": true, "gnumakefile": true, "cmakelists.txt": true, "dockerfile": true,
	"meson.build": true, "build.zig": true, "build.zig.zon": true, "mix.exs": true, "pubspec.yaml": true, "requirements.txt": true,
	"requirements-dev.txt": true, "pipfile": true, "package.swift": true, "podfile": true, "build.sbt": true, "bazel": true,
	"build": true, "build.bazel": true, "workspace": true, "workspace.bazel": true, "module.bazel": true, "justfile": true,
	"taskfile.yml": true, "taskfile.yaml": true, "magefile.go": true, "gulpfile.js": true, "gruntfile.js": true,
	"webpack.config.js": true, "webpack.config.ts": true, "vite.config.ts": true, "vite.config.js": true, "vite.config.mts": true,
	"rollup.config.js": true, "rollup.config.ts": true, "rollup.config.mjs": true, "tsup.config.ts": true, "esbuild.config.js": true,
	"tsconfig.json": true, "tsconfig.build.json": true, "babel.config.js": true, ".babelrc": true, "lerna.json": true,
	"nx.json": true, "turbo.json": true, "pnpm-workspace.yaml": true, "rush.json": true, "deno.json": true, "deno.jsonc": true,
	"conanfile.txt": true, "conanfile.py": true, "vcpkg.json": true, "configure.ac": true, "makefile.am": true,
	"manifest.in": true, "tox.ini": true, "noxfile.py": true, "cabal.project": true, "stack.yaml": true, "dune-project": true,
	"build.xml": true, "gradle.properties": true, "project.clj": true, "deps.edn": true, ".tool-versions": true,
	".nvmrc": true, ".node-version": true, ".python-version": true, ".ruby-version": true, ".go-version": true,
	"rust-toolchain": true, "rust-toolchain.toml": true, "netlify.toml": true, "vercel.json": true, "wrangler.toml": true,
	"goreleaser.yml": true, "goreleaser.yaml": true, ".goreleaser.yml": true, ".goreleaser.yaml": true,
	"dockerfile.dev": true, "docker-compose.yml": true, "docker-compose.yaml": true, "compose.yml": true, "compose.yaml": true,
	"nuxt.config.ts": true, "nuxt.config.js": true, "next.config.js": true, "next.config.mjs": true, "next.config.ts": true,
	"angular.json": true, "svelte.config.js": true, "astro.config.mjs": true, "tailwind.config.js": true, "tailwind.config.ts": true,
	"postcss.config.js": true, "postcss.config.cjs": true, "vitest.config.ts": true, "jest.config.js": true, "jest.config.ts": true,
	"karma.conf.js": true, "playwright.config.ts": true, "cypress.config.ts": true, "cypress.config.js": true,
	"flake.nix": true, "default.nix": true, "shell.nix": true, "renovate.json": true, "renovate.json5": true, ".renovaterc": true,
	".renovaterc.json": true, "dependabot.yml": true, "cargo.nix": true, "gemspec": true, "sonar-project.properties": true,
}

var configFiles = map[string]bool{
	".editorconfig": true, ".gitignore": true, ".gitattributes": true, ".gitmodules": true, ".dockerignore": true, ".npmignore": true,
	".prettierrc": true, ".prettierrc.json": true, ".prettierrc.js": true, ".prettierrc.cjs": true, ".prettierrc.yaml": true, ".prettierignore": true,
	".eslintrc": true, ".eslintrc.js": true, ".eslintrc.cjs": true, ".eslintrc.json": true, ".eslintrc.yml": true, ".eslintignore": true,
	"eslint.config.js": true, "eslint.config.mjs": true, "eslint.config.ts": true, "eslint.config.cjs": true, "biome.json": true, "biome.jsonc": true,
	".stylelintrc": true, ".stylelintrc.json": true, "stylelint.config.js": true, ".golangci.yml": true, ".golangci.yaml": true, ".golangci.toml": true,
	".rubocop.yml": true, ".flake8": true, ".pylintrc": true, "pylintrc": true, "mypy.ini": true, ".mypy.ini": true, "ruff.toml": true, ".ruff.toml": true,
	".pre-commit-config.yaml": true, ".commitlintrc": true, ".commitlintrc.json": true, ".commitlintrc.js": true, "commitlint.config.js": true,
	"commitlint.config.ts": true, ".czrc": true, ".cz.toml": true, ".huskyrc": true, ".lintstagedrc": true, ".lintstagedrc.json": true,
	"lint-staged.config.js": true, ".mailmap": true, "codeowners": true, ".codeowners": true, "funding.yml": true, ".markdownlint.json": true,
	".markdownlint.yaml": true, ".yamllint": true, ".yamllint.yml": true, ".hadolint.yaml": true, ".shellcheckrc": true, ".clang-format": true,
	".clang-tidy": true, ".rustfmt.toml": true, "rustfmt.toml": true, "clippy.toml": true, ".clippy.toml": true, ".php-cs-fixer.php": true,
	".php-cs-fixer.dist.php": true, "phpstan.neon": true, "phpstan.neon.dist": true, "psalm.xml": true, ".phpcs.xml": true, "phpcs.xml": true,
	"phpcs.xml.dist": true, ".swiftlint.yml": true, "detekt.yml": true, ".detekt.yml": true, "checkstyle.xml": true, ".editorconfig.json": true,
	".gitpod.yml": true, ".devcontainer.json": true, "devcontainer.json": true, ".codespellrc": true, ".cspell.json": true, "cspell.json": true,
	".typos.toml": true, "typos.toml": true, ".vscode": true, ".idea": true, ".env.example": true, ".env.sample": true, ".coveragerc": true,
	"codecov.yml": true, ".codecov.yml": true, "sonar-project.properties": true, ".nycrc": true, ".c8rc.json": true,
}

var changelogFiles = map[string]bool{"changelog.md": true, "changelog": true, "changes.md": true, "history.md": true, "changelog.rst": true,
	"changelog.txt": true, "release-notes.md": true, "releases.md": true, "news.md": true, "news.rst": true, "news": true, "changes": true, "changes.rst": true,
	"history.rst": true, "changelog.mdx": true, "release_notes.md": true}

var licenseFiles = map[string]bool{"license": true, "license.md": true, "license.txt": true, "licence": true, "licence.md": true, "copying": true,
	"copying.md": true, "notice": true, "notice.md": true, "notice.txt": true, "unlicense": true, "license-mit": true, "license-apache": true,
	"code_of_conduct.md": true, "codeofconduct.md": true, "contributing.md": true, "contributors.md": true, "authors": true, "authors.md": true,
	"maintainers.md": true, "security.md": true, "support.md": true, "governance.md": true, "citation.cff": true}

var versionFiles = map[string]bool{"version": true, "version.txt": true, "version.go": true, "version.py": true, "__version__.py": true,
	"_version.py": true, "version.rb": true, "version.ts": true, "version.js": true, "version.json": true, ".version": true, "version.h": true,
	"version.cmake": true, "version.rs": true, "version.php": true, "version.java": true, "version.kt": true, "version.swift": true, "version.mjs": true}

var docExt = map[string]bool{"md": true, "mdx": true, "rst": true, "txt": true, "adoc": true, "asciidoc": true, "org": true, "tex": true, "man": true,
	"1": true, "5": true, "7": true, "8": true, "pod": true, "rdoc": true, "textile": true, "wiki": true, "markdown": true, "mkd": true, "livemd": true}

var styleExt = map[string]bool{"css": true, "scss": true, "sass": true, "less": true, "styl": true, "pcss": true, "postcss": true}

var assetExt = map[string]bool{"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "avif": true, "ico": true, "icns": true, "bmp": true,
	"tiff": true, "tif": true, "svg": true, "woff": true, "woff2": true, "ttf": true, "otf": true, "eot": true, "mp3": true, "mp4": true, "wav": true,
	"ogg": true, "webm": true, "mov": true, "pdf": true, "psd": true, "ai": true, "sketch": true, "fig": true, "zip": true, "gz": true, "tgz": true,
	"tar": true, "jar": true, "wasm": true, "so": true, "dll": true, "dylib": true, "exe": true, "bin": true, "dat": true, "pak": true, "glb": true,
	"gltf": true, "obj": true, "fbx": true, "blend": true, "hdr": true, "exr": true, "dds": true, "ktx": true, "flac": true, "aac": true, "m4a": true,
	"heic": true, "cur": true, "ani": true, "xcf": true, "pyc": true, "class": true, "o": true, "a": true, "lib": true, "pdb": true, "sqlite": true, "db": true}

var i18nExt = map[string]bool{"po": true, "pot": true, "mo": true, "xlf": true, "xliff": true, "arb": true, "resx": true, "strings": true, "stringsdict": true, "ftl": true}

var dataExt = map[string]bool{"json": true, "json5": true, "jsonc": true, "yaml": true, "yml": true, "toml": true, "csv": true, "tsv": true, "xml": true,
	"ini": true, "cfg": true, "conf": true, "properties": true, "plist": true, "ndjson": true, "jsonl": true, "env": true, "hcl": true, "tfvars": true}

var schemaExt = map[string]bool{"sql": true, "proto": true, "graphql": true, "gql": true, "prisma": true, "avsc": true, "thrift": true, "fbs": true,
	"capnp": true, "xsd": true, "wsdl": true, "openapi": true, "dbml": true, "cue": true}

var templateExt = map[string]bool{"html": true, "htm": true, "tmpl": true, "tpl": true, "hbs": true, "handlebars": true, "mustache": true, "ejs": true,
	"pug": true, "jade": true, "njk": true, "twig": true, "blade.php": true, "erb": true, "haml": true, "slim": true, "liquid": true, "j2": true,
	"jinja": true, "jinja2": true, "xhtml": true, "svelte": false, "vue": false, "gotmpl": true, "gohtml": true, "eex": true, "heex": true, "leex": true}

var scriptExt = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "bat": true, "cmd": true, "ps1": true, "psm1": true, "nu": true, "awk": true}

var snapshotExt = map[string]bool{"snap": true, "golden": true, "approved": true, "received": true, "expected": true, "out": true, "stdout": true,
	"stderr": true, "exp": true, "fixture": true, "vcr": true, "cassette": true}

// Classify returns the category of a path. Order matters: the first matching
// rule wins, and rules are ordered from most to least specific.
func Classify(p string) Category {
	lp := strings.ToLower(p)
	base := path.Base(lp)
	ext := extOf(base)
	dirs := strings.Split(path.Dir(lp), "/")
	if lp == "." || lp == "" {
		return CatSource
	}
	// --- CI ---
	if strings.HasPrefix(lp, ".github/workflows/") || strings.HasPrefix(lp, ".github/actions/") || strings.HasPrefix(lp, ".circleci/") ||
		strings.HasPrefix(lp, ".gitlab/") || base == ".gitlab-ci.yml" || base == ".travis.yml" || base == "appveyor.yml" ||
		base == ".appveyor.yml" || base == "jenkinsfile" || base == "azure-pipelines.yml" || strings.HasPrefix(lp, ".buildkite/") ||
		base == "bitbucket-pipelines.yml" || base == ".drone.yml" || strings.HasPrefix(lp, ".woodpecker") || base == "cloudbuild.yaml" ||
		base == "cloudbuild.yml" || strings.HasPrefix(lp, ".teamcity/") || base == "codemagic.yaml" || base == ".cirrus.yml" ||
		strings.HasPrefix(lp, ".azure-pipelines/") || strings.HasPrefix(lp, ".ci/") || strings.HasPrefix(lp, "ci/") ||
		strings.HasPrefix(lp, ".github/") && (ext == "yml" || ext == "yaml") || base == ".mergify.yml" || base == ".mergify.yaml" ||
		strings.HasPrefix(lp, ".semaphore/") || base == "wercker.yml" || base == ".scrutinizer.yml" || strings.HasPrefix(lp, ".forgejo/workflows/") ||
		strings.HasPrefix(lp, ".gitea/workflows/") {
		return CatCI
	}
	// --- version manifests ---
	if versionFiles[base] {
		return CatVersion
	}
	// --- changelog / license ---
	if changelogFiles[base] || strings.HasPrefix(lp, ".changeset/") || strings.HasPrefix(lp, "changelog.d/") || strings.HasPrefix(lp, "changelogs/") ||
		strings.HasPrefix(lp, "changes/") || strings.HasPrefix(lp, "news.d/") || strings.HasPrefix(lp, ".changes/") || strings.HasPrefix(lp, "changelog/") ||
		strings.HasPrefix(lp, "release-notes/") || strings.HasPrefix(lp, "releasenotes/") || strings.HasPrefix(lp, "doc/release/upcoming_changes/") ||
		strings.HasPrefix(lp, "doc/source/whatsnew/") || strings.HasPrefix(lp, "doc/whats_new/") || strings.HasPrefix(lp, "docs/changelog/") ||
		strings.HasPrefix(lp, "unreleased/") || strings.HasPrefix(lp, "towncrier/") || strings.HasPrefix(lp, "doc/changes/") ||
		strings.Contains(lp, "/changelog.") || strings.Contains(lp, "/upcoming_changes/") || strings.Contains(lp, "/whatsnew/") {
		return CatChangelog
	}
	if licenseFiles[base] || strings.HasPrefix(lp, "licenses/") || strings.HasPrefix(lp, "license/") {
		return CatLicense
	}
	// --- lock ---
	if lockFiles[base] {
		return CatLock
	}
	// --- generated / vendored ---
	if hasDir(dirs, "vendor", "node_modules", "third_party", "thirdparty", "third-party", "generated", "gen", "__generated__", "_generated",
		"autogen", "auto-generated", ".yarn", "dist", "build", "out", "target") || strings.Contains(base, ".generated.") ||
		strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, ".pb.cc") || strings.HasSuffix(base, ".pb.h") || strings.HasSuffix(base, "_pb2.py") ||
		strings.HasSuffix(base, "_pb2_grpc.py") || strings.HasSuffix(base, ".g.dart") || strings.HasSuffix(base, ".freezed.dart") ||
		strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") || strings.HasSuffix(base, ".bundle.js") ||
		strings.HasSuffix(base, "_generated.go") || strings.HasSuffix(base, ".gen.go") || strings.HasSuffix(base, ".gen.ts") ||
		strings.HasSuffix(base, "_string.go") || strings.HasSuffix(base, ".d.ts.map") || strings.HasSuffix(base, ".js.map") ||
		strings.HasSuffix(base, "zz_generated") || strings.HasPrefix(base, "zz_generated") || strings.HasSuffix(base, ".swagger.json") ||
		strings.HasSuffix(base, "_grpc.pb.go") || strings.HasSuffix(base, ".pb.swift") || strings.HasSuffix(base, ".mock.go") ||
		strings.HasSuffix(base, "_mock.go") || hasDir(dirs, "mocks") && ext == "go" {
		return CatGenerated
	}
	// --- snapshots & fixtures ---
	if hasDir(dirs, "__snapshots__", "snapshots", "__fixtures__", "fixtures", "testdata", "test_data", "golden", "goldens", "__mocks__",
		"cassettes", "vcr_cassettes", "expected", "baselines", "__image_snapshots__", "recordings") || snapshotExt[ext] ||
		strings.Contains(base, ".snap.") || strings.HasSuffix(base, ".golden") {
		return CatSnapshot
	}
	// --- tests ---
	if isTestPath(lp, base, dirs, ext) {
		return CatTest
	}
	// --- docs ---
	if hasDir(dirs, "docs", "doc", "documentation", "website", "site", "wiki", "manual", "guide", "guides", "handbook", "man", "docsrc", "docsite",
		"examples", "example", "samples", "sample", "demo", "demos", "tutorials", "tutorial", "adr", "adrs", "rfcs", "rfc", "design-docs", "proposals",
		"blog", "book", ".github/issue_template", "issue_template", "pull_request_template") || docExt[ext] || strings.HasPrefix(base, "readme") ||
		base == "docs.go" || base == "doc.go" || strings.HasPrefix(lp, ".github/issue_template") || strings.HasPrefix(lp, ".github/pull_request_template") ||
		strings.HasPrefix(lp, ".github/discussion_template") || base == "mkdocs.yml" || base == "mkdocs.yaml" || base == "docusaurus.config.js" ||
		base == "docusaurus.config.ts" || base == ".readthedocs.yaml" || base == ".readthedocs.yml" || base == "conf.py" && hasDir(dirs, "doc", "docs") ||
		base == "typedoc.json" || base == "book.toml" || base == "_config.yml" || base == "jsdoc.json" || base == ".vitepress" || hasDir(dirs, ".vitepress") {
		return CatDocs
	}
	// --- i18n ---
	if hasDir(dirs, "i18n", "locales", "locale", "translations", "translation", "lang", "langs", "l10n", "intl", "messages", "_locales", "localization", "localizations") ||
		i18nExt[ext] || base == "en.json" || base == "en-us.json" || base == "fr.json" || base == "de.json" || base == "zh-cn.json" || base == "ja.json" {
		return CatI18n
	}
	// --- build ---
	if buildFiles[base] || strings.HasPrefix(base, "dockerfile") || strings.HasSuffix(base, ".dockerfile") || ext == "gradle" ||
		strings.HasSuffix(base, ".csproj") || strings.HasSuffix(base, ".fsproj") || strings.HasSuffix(base, ".vbproj") || strings.HasSuffix(base, ".sln") ||
		strings.HasSuffix(base, ".props") || strings.HasSuffix(base, ".targets") || strings.HasSuffix(base, ".nuspec") || strings.HasSuffix(base, ".gemspec") ||
		strings.HasSuffix(base, ".cabal") || strings.HasSuffix(base, ".podspec") || strings.HasSuffix(base, ".pbxproj") || strings.HasSuffix(base, ".xcconfig") ||
		strings.HasSuffix(base, ".xcscheme") || ext == "cmake" || ext == "mk" || ext == "bzl" || ext == "nix" || ext == "bazel" || ext == "ninja" ||
		strings.HasPrefix(base, "requirements") && ext == "txt" || strings.HasPrefix(base, "webpack.") || strings.HasPrefix(base, "vite.config") ||
		strings.HasPrefix(base, "rollup.config") || strings.HasPrefix(base, "tsconfig.") || strings.HasPrefix(base, "jest.config") ||
		strings.HasPrefix(base, "vitest.config") || strings.HasPrefix(base, "tsup.config") || strings.HasPrefix(base, "esbuild.") ||
		strings.HasPrefix(base, "docker-compose") || strings.HasPrefix(base, "babel.config") || strings.HasPrefix(base, "gradle-wrapper") ||
		hasDir(dirs, "gradle", "cmake", ".cargo", "scripts/build", "build-tools", "buildtools", "tools/build", ".docker", "docker", "deploy", "deployment", "helm", "charts", "k8s", "kubernetes", "terraform", "infra", "infrastructure", "packaging", "pkg/packaging", "release", "releases", "installer", "dist-scripts") ||
		base == "manifest.json" && hasDir(dirs, "chrome", "extension") || ext == "nuspec" || ext == "spec" && strings.Contains(lp, "rpm") ||
		ext == "ebuild" || base == "pkgbuild" || base == "snapcraft.yaml" || base == "flatpak.json" || strings.HasSuffix(base, ".flatpak.yml") ||
		base == "electron-builder.yml" || base == "electron-builder.json" || base == "tauri.conf.json" || base == "app.json" || base == "eas.json" ||
		base == "fastfile" || base == "appfile" || base == "matchfile" || base == "gymfile" || strings.HasPrefix(lp, "fastlane/") || base == "info.plist" ||
		base == "androidmanifest.xml" || base == "proguard-rules.pro" || base == "build.rs" || base == "setup.cfg" || base == "poetry.toml" || base == "mise.toml" ||
		base == ".mise.toml" || base == "deno.json" || base == "wails.json" || base == "sam.yaml" || base == "template.yaml" && hasDir(dirs, "sam") {
		return CatBuild
	}
	// --- config ---
	if configFiles[base] || hasDir(dirs, ".vscode", ".idea", ".devcontainer", ".husky", ".config", ".github", ".gitlab") || strings.HasPrefix(base, ".eslintrc") ||
		strings.HasPrefix(base, ".prettierrc") || strings.HasPrefix(base, ".stylelintrc") || strings.HasPrefix(base, ".babelrc") || strings.HasPrefix(base, ".lintstagedrc") ||
		strings.HasPrefix(base, ".commitlintrc") || strings.HasPrefix(base, ".releaserc") || strings.HasPrefix(base, ".npmrc") || strings.HasPrefix(base, ".yarnrc") ||
		base == ".gitignore" || strings.HasSuffix(base, "ignore") && strings.HasPrefix(base, ".") || base == "lefthook.yml" || base == ".lefthook.yml" ||
		strings.HasPrefix(base, ".env") || base == "sonar-project.properties" || base == "release-please-config.json" || base == ".release-please-manifest.json" ||
		strings.HasPrefix(base, ".versionrc") || base == "cliff.toml" || base == ".mocharc.js" || base == ".mocharc.json" || base == ".mocharc.yml" ||
		strings.HasPrefix(base, ".swcrc") || base == "knip.json" || base == ".knip.json" || base == "lefthook.yaml" || base == "dprint.json" ||
		strings.HasPrefix(base, ".oxlintrc") || base == "oxlint.json" || base == ".editorconfig" {
		return CatConfig
	}
	// --- style ---
	if styleExt[ext] {
		return CatStyle
	}
	// --- assets ---
	if assetExt[ext] || hasDir(dirs, "assets", "static", "public", "images", "img", "icons", "fonts", "media", "resources/images", "res/drawable", "res/mipmap") && !isCode(ext) {
		return CatAsset
	}
	// --- schema ---
	if schemaExt[ext] || hasDir(dirs, "migrations", "migration", "db/migrate", "schema", "schemas", "proto", "protos", "openapi", "swagger") && !isCode(ext) ||
		hasDir(dirs, "migrations", "migration", "db/migrate", "alembic/versions") || strings.HasSuffix(base, ".graphqls") || base == "schema.graphql" || base == "openapi.yaml" || base == "openapi.json" || base == "swagger.yaml" || base == "swagger.json" {
		return CatSchema
	}
	// --- script ---
	if scriptExt[ext] || hasDir(dirs, "scripts", "script", "bin", "tools", "tool", "hack", "utils/scripts", "dev", ".dev", "devtools") && !isCode(ext) {
		return CatScript
	}
	// --- template ---
	if templateExt[ext] || strings.HasSuffix(base, ".blade.php") || strings.HasSuffix(base, ".html.erb") || strings.HasSuffix(base, ".html.twig") ||
		hasDir(dirs, "templates", "template", "views", "layouts", "partials", "_includes", "_layouts") && (ext == "html" || ext == "htm" || ext == "xml" || ext == "" || templateExt[ext]) {
		return CatTemplate
	}
	// --- data ---
	if dataExt[ext] && !isCode(ext) {
		return CatData
	}
	if hasDir(dirs, "tools", "scripts", "hack", "dev", "devtools", "tooling") {
		return CatScript
	}
	if hasDir(dirs, "examples", "example", "samples", "sample", "demo", "demos") {
		return CatDocs
	}
	return CatSource
}

func isTestPath(lp, base string, dirs []string, ext string) bool {
	if hasDir(dirs, "test", "tests", "__tests__", "spec", "specs", "e2e", "integration", "integration-tests", "integration_tests", "unittest", "unittests",
		"unit_tests", "unit-tests", "testing", "tests-e2e", "test-e2e", "cypress", "playwright", "__test__", "t", "testsuite", "test_suite", "acceptance",
		"functional", "regression", "benchmarks", "benchmark", "bench", "benches", "testutil", "testutils", "test-utils", "test_utils", "testhelpers", "test-helpers",
		"__integration__", "__e2e__", "smoke", "tests_integration", "test-integration", "e2e-tests", "test/e2e", "testkit", "testcases", "test_cases", "features") {
		return true
	}
	name := base
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	full := strings.TrimSuffix(base, "."+ext)
	switch {
	case strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "_spec") || strings.HasSuffix(name, "-test") || strings.HasSuffix(name, "-spec"),
		strings.HasSuffix(full, ".test") || strings.HasSuffix(full, ".spec") || strings.HasSuffix(full, "_test") || strings.HasSuffix(full, "_spec") ||
			strings.HasSuffix(full, ".e2e") || strings.HasSuffix(full, ".e2e-spec") || strings.HasSuffix(full, ".integration") || strings.HasSuffix(full, ".bench") ||
			strings.HasSuffix(full, ".cy") || strings.HasSuffix(full, ".test-d") || strings.HasSuffix(full, ".spec-d") || strings.HasSuffix(full, ".stories"),
		strings.HasPrefix(name, "test_") || strings.HasPrefix(name, "test-") || strings.HasPrefix(name, "spec_"),
		strings.HasSuffix(name, "test") && (ext == "java" || ext == "kt" || ext == "cs" || ext == "scala" || ext == "php" || ext == "swift" || ext == "groovy" || ext == "dart" || ext == "exs"),
		strings.HasSuffix(name, "tests") && (ext == "java" || ext == "kt" || ext == "cs" || ext == "scala" || ext == "php" || ext == "swift" || ext == "cpp" || ext == "cc" || ext == "py" || ext == "rs"),
		strings.HasSuffix(name, "testcase") || strings.HasSuffix(name, "spec") && (ext == "rb" || ext == "js" || ext == "ts" || ext == "php" || ext == "scala" || ext == "exs"),
		base == "conftest.py" || base == "pytest.ini" || base == "phpunit.xml" || base == "phpunit.xml.dist" || base == ".rspec" || base == "codeception.yml",
		strings.HasSuffix(full, "_test") && ext == "go", strings.Contains(base, ".test.") || strings.Contains(base, ".spec."):
		return true
	}
	return false
}

func hasDir(dirs []string, names ...string) bool {
	for _, d := range dirs {
		for _, n := range names {
			if d == n {
				return true
			}
		}
	}
	// also match multi-segment names like "db/migrate"
	joined := strings.Join(dirs, "/")
	for _, n := range names {
		if strings.Contains(n, "/") && (strings.HasPrefix(joined, n) || strings.Contains(joined, "/"+n+"/") || strings.HasSuffix(joined, "/"+n)) {
			return true
		}
	}
	return false
}

var codeExt = map[string]bool{"go": true, "rs": true, "py": true, "js": true, "ts": true, "tsx": true, "jsx": true, "mjs": true, "cjs": true, "mts": true, "cts": true,
	"java": true, "kt": true, "kts": true, "scala": true, "rb": true, "php": true, "c": true, "h": true, "cc": true, "cpp": true, "cxx": true, "hpp": true, "hh": true, "hxx": true,
	"cs": true, "fs": true, "swift": true, "m": true, "mm": true, "dart": true, "ex": true, "exs": true, "erl": true, "hs": true, "ml": true, "mli": true, "clj": true, "cljs": true,
	"lua": true, "pl": true, "pm": true, "r": true, "jl": true, "zig": true, "nim": true, "v": true, "sv": true, "vhd": true, "vue": true, "svelte": true, "astro": true,
	"elm": true, "purs": true, "re": true, "res": true, "groovy": true, "gradle": true, "sol": true, "move": true, "cu": true, "cuh": true, "cl": true, "glsl": true,
	"hlsl": true, "wgsl": true, "metal": true, "vert": true, "frag": true, "asm": true, "s": true, "d": true, "pas": true, "f90": true, "f": true, "for": true,
	"tf": true, "cr": true, "gd": true, "gdscript": true, "rkt": true, "scm": true, "lisp": true, "el": true, "vim": true, "fnl": true, "tcl": true, "ps1": true, "sh": true,
	"bash": true, "zsh": true, "fish": true, "pyx": true, "pxd": true, "pyi": true, "ipynb": true, "sql": true, "proto": true, "graphql": true, "prisma": true, "css": true,
	"scss": true, "less": true, "html": true}

func isCode(ext string) bool { return codeExt[ext] }

// extOf returns the lower-cased extension without dot. Handles "foo.test.ts"
// => "ts", ".gitignore" => "" and "Makefile" => "".
func extOf(base string) string {
	i := strings.LastIndexByte(base, '.')
	if i <= 0 || i == len(base)-1 {
		return ""
	}
	e := base[i+1:]
	if len(e) > 12 {
		return ""
	}
	return e
}

// Lang is a coarse language family used as a hashed token and for language
// diversity stats. It is derived from the extension only.
func Lang(ext string) string {
	switch ext {
	case "js", "mjs", "cjs", "jsx":
		return "js"
	case "ts", "mts", "cts", "tsx":
		return "ts"
	case "vue", "svelte", "astro":
		return "sfc"
	case "py", "pyi", "pyx", "pxd", "ipynb":
		return "py"
	case "go":
		return "go"
	case "rs":
		return "rust"
	case "java", "kt", "kts", "scala", "groovy":
		return "jvm"
	case "rb", "erb":
		return "ruby"
	case "php":
		return "php"
	case "c", "h", "cc", "cpp", "cxx", "hpp", "hh", "hxx", "m", "mm", "cu", "cuh":
		return "c"
	case "cs", "fs", "vb":
		return "dotnet"
	case "swift":
		return "swift"
	case "dart":
		return "dart"
	case "ex", "exs", "erl", "heex", "eex":
		return "beam"
	case "hs", "ml", "mli", "elm", "purs", "clj", "cljs", "rkt", "scm", "lisp", "el", "fnl":
		return "fp"
	case "lua", "vim":
		return "lua"
	case "sh", "bash", "zsh", "fish", "ps1", "bat", "cmd", "nu":
		return "shell"
	case "css", "scss", "sass", "less", "styl":
		return "css"
	case "html", "htm", "xhtml":
		return "html"
	case "md", "mdx", "rst", "adoc", "txt":
		return "doc"
	case "json", "json5", "jsonc", "yaml", "yml", "toml", "ini", "cfg", "conf", "xml", "properties", "hcl", "tf", "tfvars", "cue", "nix":
		return "config"
	case "sql", "proto", "graphql", "gql", "prisma":
		return "schema"
	case "zig", "nim", "v", "d", "cr", "jl", "r", "pl", "pm", "sol", "gd", "tcl", "f90", "asm", "s":
		return "other"
	case "":
		return "none"
	default:
		return "unknown"
	}
}
