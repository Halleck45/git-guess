package gitmoji

import (
	"strings"
	"testing"

	"github.com/Halleck45/git-guess/internal/diff"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in                   string
		code, scope, message string
		ok                   bool
	}{
		{"✨ (auth): add login", ":sparkles:", "auth", "add login", true},
		{"✨ add login", ":sparkles:", "", "add login", true},
		{"✨add login", ":sparkles:", "", "add login", true},
		{":sparkles: (auth): add login", ":sparkles:", "auth", "add login", true},
		{":sparkles:add login", ":sparkles:", "", "add login", true},
		{"⚡️ lazy load", ":zap:", "", "lazy load", true},
		{"⚡ lazy load", ":zap:", "", "lazy load", true}, // no variation selector
		{"🧑‍💻 better dx", ":technologist:", "", "better dx", true},
		{"🔖 v1.2.0", ":bookmark:", "", "v1.2.0", true},
		{"feat(auth): add login", "", "", "", false},
		{":not_a_gitmoji: x", "", "", "", false},
		{"🍕 pizza", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, c := range cases {
		e, scope, msg, ok := Parse(c.in)
		if ok != c.ok || e.Code != c.code || scope != c.scope || msg != c.message {
			t.Errorf("%q: got (%q,%q,%q,%v) want (%q,%q,%q,%v)", c.in, e.Code, scope, msg, ok, c.code, c.scope, c.message, c.ok)
		}
	}
}

func TestFormat(t *testing.T) {
	e := get(":sparkles:")
	cases := []struct {
		scope, subject string
		code           bool
		want           string
	}{
		{"auth", "add login", false, "✨ (auth): add login"},
		{"", "add login", false, "✨ add login"},
		{"auth", "", false, "✨ (auth):"},
		{"", "", false, "✨"},
		{"auth", "add login", true, ":sparkles: (auth): add login"},
	}
	for _, c := range cases {
		if got := Format(e, c.scope, c.subject, c.code); got != c.want {
			t.Errorf("Format(%q,%q,%v) = %q want %q", c.scope, c.subject, c.code, got, c.want)
		}
	}
	// Round trip.
	if e2, s, m, ok := Parse(Format(e, "auth", "add login", false)); !ok || e2.Code != e.Code || s != "auth" || m != "add login" {
		t.Errorf("round trip failed: %v %q %q", ok, s, m)
	}
}

func TestEveryEmojiIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range All {
		if seen[e.Code] || seen[e.Unicode] {
			t.Errorf("duplicate %s", e.Code)
		}
		seen[e.Code], seen[e.Unicode] = true, true
		if e.typ != "" {
			if _, ok := baseOf[e.typ]; !ok {
				t.Errorf("%s maps to unknown type %q", e.Code, e.typ)
			}
		}
	}
	if len(All) != 75 {
		t.Errorf("expected the 75 official gitmojis, got %d", len(All))
	}
}

// d builds a diff from compact file specs: "status path|+added line|-removed line".
func d(t *testing.T, specs ...string) *diff.Diff {
	t.Helper()
	var b strings.Builder
	for _, spec := range specs {
		parts := strings.Split(spec, "|")
		status, p, _ := strings.Cut(parts[0], " ")
		old := p
		if status == "R" {
			old, p, _ = strings.Cut(p, "->")
		}
		b.WriteString("diff --git a/" + old + " b/" + p + "\n")
		switch status {
		case "A":
			b.WriteString("new file mode 100644\n--- /dev/null\n+++ b/" + p + "\n")
		case "D":
			b.WriteString("deleted file mode 100644\n--- a/" + p + "\n+++ /dev/null\n")
		case "R":
			b.WriteString("similarity index 90%\nrename from " + old + "\nrename to " + p + "\n--- a/" + old + "\n+++ b/" + p + "\n")
		default:
			b.WriteString("--- a/" + p + "\n+++ b/" + p + "\n")
		}
		if len(parts) > 1 {
			b.WriteString("@@ -1,1 +1,1 @@\n")
			for _, l := range parts[1:] {
				b.WriteString(l + "\n")
			}
		}
	}
	return diff.ParseString(b.String())
}

func TestPick(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		msg  string
		diff *diff.Diff
		want string
	}{
		{"feat default", "feat", "add login", d(t, "M src/auth.go|+func Login() {}"), ":sparkles:"},
		{"fix default", "fix", "", d(t, "M src/auth.go|-a|+b"), ":bug:"},
		{"docs", "docs", "", d(t, "M README.md|+hello"), ":memo:"},
		{"style", "style", "", d(t, "M src/a.go|-x|+ x"), ":art:"},
		{"style css", "style", "", d(t, "M web/app.css|+.a{}"), ":lipstick:"},
		{"refactor", "refactor", "", d(t, "M src/a.go|-x|+y"), ":recycle:"},
		{"perf", "perf", "", d(t, "M src/a.go|-x|+y"), ":zap:"},
		{"test", "test", "", d(t, "M src/a_test.go|+func TestX(t *testing.T) {}"), ":white_check_mark:"},
		{"ci", "ci", "", d(t, "M .github/workflows/ci.yml|+  - run: go test"), ":construction_worker:"},
		{"chore", "chore", "", d(t, "M .editorconfig|+indent_style = tab"), ":wrench:"},
		{"revert", "revert", "", d(t, "M src/a.go|-x|+y"), ":rewind:"},
		{"wip", "feat", "wip: login", d(t, "M src/a.go|+x"), ":construction:"},
		{"gitignore", "chore", "", d(t, "M .gitignore|+dist/"), ":see_no_evil:"},
		{"license", "docs", "", d(t, "M LICENSE|-2024|+2025"), ":page_facing_up:"},
		{"contributors", "docs", "", d(t, "M AUTHORS.md|+- Jane"), ":busts_in_silhouette:"},
		{"rename", "refactor", "", d(t, "R src/old.go->src/new.go|-package old|+package new"), ":truck:"},
		{"rename with rewrite", "refactor", "", d(t, "R src/old.go->src/new.go|-a|-b|-c|-d|-e|-f|+1|+2|+3|+4|+5|+6"), ":recycle:"},
		{"delete files", "refactor", "", d(t, "D src/legacy.go|-package legacy|-func Old() {}"), ":fire:"},
		{"delete files chore", "chore", "", d(t, "D scripts/old.sh|-echo old"), ":fire:"},
		{"dead code", "refactor", "remove unused helpers", d(t, "M src/a.go|-func unused() {}"), ":coffin:"},
		{"fix pure removal stays bug", "fix", "", d(t, "M src/a.go|-wrong line"), ":bug:"},
		{"i18n files", "feat", "", d(t, "M locales/fr.json|+\"hello\": \"bonjour\""), ":globe_with_meridians:"},
		{"i18n message", "feat", "add japanese translation", d(t, "M src/a.go|+x"), ":globe_with_meridians:"},
		{"assets", "chore", "", d(t, "A public/logo.png"), ":bento:"},
		{"logs added", "feat", "", d(t, "M src/a.go|+\tlog.Printf(\"x=%v\", x)|+\tlogger.Info(\"y\")"), ":loud_sound:"},
		{"logs removed", "refactor", "", d(t, "M src/a.js|-  console.log(x)"), ":mute:"},
		{"types", "feat", "", d(t, "M src/index.d.ts|+export type X = 1"), ":label:"},
		{"database", "feat", "", d(t, "A db/migrations/0002_users.sql|+CREATE TABLE users ()"), ":card_file_box:"},
		{"css feat", "feat", "", d(t, "M web/style.css|+.a{}"), ":lipstick:"},
		{"a11y", "feat", "improve a11y of the modal", d(t, "M src/a.tsx|+aria-label"), ":wheelchair:"},
		{"analytics", "feat", "add tracking on checkout", d(t, "M src/a.ts|+track()"), ":chart_with_upwards_trend:"},
		{"feature flag", "feat", "put search behind a feature flag", d(t, "M src/a.ts|+flag"), ":triangular_flag_on_post:"},
		{"authz", "feat", "check admin permissions", d(t, "M src/a.ts|+can()"), ":passport_control:"},
		{"security", "fix", "prevent XSS in comments", d(t, "M src/a.ts|-x|+escape(x)"), ":lock:"},
		{"hotfix", "fix", "hotfix crash on start", d(t, "M src/a.ts|-x|+y"), ":ambulance:"},
		{"typo fix", "fix", "fix typo in error message", d(t, "M src/a.ts|-recieve|+receive"), ":pencil2:"},
		{"typo docs", "docs", "typos", d(t, "M README.md|-teh|+the"), ":pencil2:"},
		{"ci fix", "fix", "", d(t, "M .github/workflows/ci.yml|-x|+y"), ":green_heart:"},
		{"ci fix message", "ci", "fix flaky release job", d(t, "M .github/workflows/release.yml|-x|+y"), ":green_heart:"},
		{"catch", "fix", "handle errors from the API", d(t, "M src/a.ts|+try {"), ":goal_net:"},
		{"lint", "style", "fix lint warnings", d(t, "M src/a.go|-x|+y"), ":rotating_light:"},
		{"comments", "docs", "", d(t, "M src/a.go|+// Login authenticates the user.|+// It returns an error when the password is wrong."), ":bulb:"},
		{"comments with code stay memo", "docs", "", d(t, "M src/a.go|+// Login authenticates.|+func Login() {}"), ":memo:"},
		{"snapshots", "test", "", d(t, "M src/__snapshots__/a.snap|+exports[`x`] = 1"), ":camera_flash:"},
		{"mocks", "test", "", d(t, "M src/__mocks__/api.ts|+export const get = jest.fn()"), ":clown_face:"},
		{"failing test", "test", "add failing test for #12", d(t, "M src/a_test.go|+x"), ":test_tube:"},
		{"deprecate", "refactor", "deprecate the old client", d(t, "M src/a.go|+// Deprecated: use New"), ":wastebasket:"},
		{"architecture", "refactor", "new architecture for plugins", d(t, "M src/a.go|-x|+y"), ":building_construction:"},
		{"dep add npm", "build", "", d(t, `M package.json|+    "zod": "^3.22.0",`), ":heavy_plus_sign:"},
		{"dep add with lock", "chore", "", d(t, `M package.json|+    "zod": "^3.22.0",`, "M package-lock.json|+  x|-  y"), ":heavy_plus_sign:"},
		{"dep remove", "build", "", d(t, `M package.json|-    "lodash": "^4.17.21",`), ":heavy_minus_sign:"},
		{"dep upgrade npm", "build", "", d(t, `M package.json|-    "react": "^18.2.0",|+    "react": "^19.0.0",`), ":arrow_up:"},
		{"dep downgrade", "build", "", d(t, `M package.json|-    "react": "^19.0.0",|+    "react": "^18.2.0",`), ":arrow_down:"},
		{"dep pin", "build", "", d(t, `M package.json|-    "react": "^18.2.0",|+    "react": "18.2.0",`), ":pushpin:"},
		{"dep upgrade go", "build", "", d(t, "M go.mod|-\tgithub.com/x/y v1.2.0|+\tgithub.com/x/y v1.3.0", "M go.sum|+a|-b"), ":arrow_up:"},
		{"dep upgrade cargo", "build", "", d(t, `M Cargo.toml|-serde = "1.0.190"|+serde = "1.0.200"`, "M Cargo.lock|+a|-b"), ":arrow_up:"},
		{"dep upgrade cargo table", "build", "", d(t, `M Cargo.toml|-tokio = { version = "1.30", features = ["full"] }|+tokio = { version = "1.40", features = ["full"] }`), ":arrow_up:"},
		{"dep add pyproject", "build", "", d(t, `M pyproject.toml|+    "httpx>=0.27",`), ":heavy_plus_sign:"},
		{"dep upgrade requirements", "build", "", d(t, "M requirements.txt|-requests==2.31.0|+requests==2.32.0"), ":arrow_up:"},
		{"dep upgrade composer", "build", "", d(t, `M composer.json|-        "symfony/console": "^6.4",|+        "symfony/console": "^7.0",`), ":arrow_up:"},
		{"dep upgrade gemfile", "build", "", d(t, `M Gemfile|-gem "rails", "~> 7.0"|+gem "rails", "~> 7.1"`), ":arrow_up:"},
		{"dep upgrade pubspec", "build", "", d(t, "M pubspec.yaml|-  http: ^1.1.0|+  http: ^1.2.0"), ":arrow_up:"},
		{"dep upgrade gradle", "build", "", d(t, `M build.gradle.kts|-    implementation("com.squareup.okhttp3:okhttp:4.11.0")|+    implementation("com.squareup.okhttp3:okhttp:4.12.0")`), ":arrow_up:"},
		{"dep mixed", "build", "", d(t, `M package.json|+    "zod": "^3.22.0",|-    "react": "^18.2.0",|+    "react": "^19.0.0",`), ":package:"},
		{"build script change is not a dep", "build", "", d(t, `M package.json|-    "build": "tsc",|+    "build": "tsup",`), ":package:"},
		{"build makefile", "build", "", d(t, "M Makefile|+lint:\n\tgolangci-lint run"), ":package:"},
		{"version bump", "chore", "", d(t, `M package.json|-  "version": "1.2.0",|+  "version": "1.3.0",`, "M CHANGELOG.md|+## 1.3.0"), ":bookmark:"},
		{"version file", "chore", "", d(t, "M VERSION|-1.2.0|+1.3.0"), ":bookmark:"},
		{"release message", "chore", "release 1.3.0", d(t, "M src/a.go|-x|+y"), ":bookmark:"},
		{"scripts", "chore", "", d(t, "M scripts/deploy.sh|+set -e"), ":hammer:"},
		{"chore ci files", "chore", "", d(t, "M .github/workflows/ci.yml|+x"), ":construction_worker:"},
		{"breaking", "feat", "", d(t, "M src/a.go|-x|+y"), ":boom:"},
		{"empty diff", "feat", "", &diff.Diff{}, ":sparkles:"},
		{"nil diff", "fix", "", nil, ":bug:"},
	}
	for _, c := range cases {
		breaking := c.name == "breaking"
		got := Pick(c.typ, breaking, c.diff, c.msg)
		if got.Code != c.want {
			t.Errorf("%s: got %s %s want %s", c.name, got.Unicode, got.Code, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.0", "1.3.0", 1}, {"^1.2.0", "^1.3.0", 1}, {"1.3.0", "1.2.9", -1}, {"1.2", "1.2.1", 1}, {"1.2.0", "1.2.0", 0},
		{"v0.9.0", "v0.10.0", 1}, {"~> 7.0", "~> 7.1", 1}, {"", "1.0", 0}, {"2.0.0-rc.1", "2.0.0", 1}, {"v0.0.0-20230101000000-abc", "v0.0.0-20240101000000-def", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d want %d", c.a, c.b, got, c.want)
		}
	}
}
