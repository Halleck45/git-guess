// Package gitmoji maps Conventional Commits types to gitmoji intentions
// (https://gitmoji.dev) and back.
//
// The classifier only knows the eleven conventional types. Gitmoji is finer:
// a "fix" can be a bug, a hotfix, a security patch or a fixed CI build. Pick
// starts from the predicted type and narrows it down with signals that are
// unambiguous in the diff (every file renamed, only dependency manifests
// changed, only snapshots...) or explicit in the draft subject ("typo",
// "hotfix", "a11y"). When nothing narrows it down, the base emoji of the
// type wins, which is what the community tables use.
package gitmoji

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Halleck45/git-guess/internal/diff"
	"github.com/Halleck45/git-guess/internal/features"
)

// Emoji is one gitmoji.
type Emoji struct {
	Unicode     string
	Code        string // :shortcode:
	Description string
	typ         string // conventional type it stands for, "" when none
}

// Type returns the conventional type the emoji stands for, or "" when the
// intention has no conventional equivalent (work in progress, experiments...).
func (e Emoji) Type() string { return e.typ }

// IsZero reports whether e is the zero value.
func (e Emoji) IsZero() bool { return e.Code == "" }

// All is the official list (gitmojis.json), in the upstream order. The
// conventional type of each entry is a judgment call: it is what the emoji
// most often means in a repository that also uses Conventional Commits, and
// it only serves to learn from a gitmoji history and to lint it.
var All = []Emoji{
	{"🎨", ":art:", "Improve structure / format of the code.", "style"},
	{"⚡️", ":zap:", "Improve performance.", "perf"},
	{"🔥", ":fire:", "Remove code or files.", "refactor"},
	{"🐛", ":bug:", "Fix a bug.", "fix"},
	{"🚑️", ":ambulance:", "Critical hotfix.", "fix"},
	{"✨", ":sparkles:", "Introduce new features.", "feat"},
	{"📝", ":memo:", "Add or update documentation.", "docs"},
	{"🚀", ":rocket:", "Deploy stuff.", "chore"},
	{"💄", ":lipstick:", "Add or update the UI and style files.", "style"},
	{"🎉", ":tada:", "Begin a project.", "feat"},
	{"✅", ":white_check_mark:", "Add, update, or pass tests.", "test"},
	{"🔒️", ":lock:", "Fix security or privacy issues.", "fix"},
	{"🔐", ":closed_lock_with_key:", "Add or update secrets.", "chore"},
	{"🔖", ":bookmark:", "Release / Version tags.", "chore"},
	{"🚨", ":rotating_light:", "Fix compiler / linter warnings.", "style"},
	{"🚧", ":construction:", "Work in progress.", ""},
	{"💚", ":green_heart:", "Fix CI Build.", "ci"},
	{"⬇️", ":arrow_down:", "Downgrade dependencies.", "build"},
	{"⬆️", ":arrow_up:", "Upgrade dependencies.", "build"},
	{"📌", ":pushpin:", "Pin dependencies to specific versions.", "build"},
	{"👷", ":construction_worker:", "Add or update CI build system.", "ci"},
	{"📈", ":chart_with_upwards_trend:", "Add or update analytics or track code.", "feat"},
	{"♻️", ":recycle:", "Refactor code.", "refactor"},
	{"➕", ":heavy_plus_sign:", "Add a dependency.", "build"},
	{"➖", ":heavy_minus_sign:", "Remove a dependency.", "build"},
	{"🔧", ":wrench:", "Add or update configuration files.", "chore"},
	{"🔨", ":hammer:", "Add or update development scripts.", "chore"},
	{"🌐", ":globe_with_meridians:", "Internationalization and localization.", "feat"},
	{"✏️", ":pencil2:", "Fix typos.", "docs"},
	{"💩", ":poop:", "Write bad code that needs to be improved.", ""},
	{"⏪️", ":rewind:", "Revert changes.", "revert"},
	{"🔀", ":twisted_rightwards_arrows:", "Merge branches.", ""},
	{"📦️", ":package:", "Add or update compiled files or packages.", "build"},
	{"👽️", ":alien:", "Update code due to external API changes.", "fix"},
	{"🚚", ":truck:", "Move or rename resources (e.g.: files, paths, routes).", "refactor"},
	{"📄", ":page_facing_up:", "Add or update license.", "docs"},
	{"💥", ":boom:", "Introduce breaking changes.", "feat"},
	{"🍱", ":bento:", "Add or update assets.", "chore"},
	{"♿️", ":wheelchair:", "Improve accessibility.", "feat"},
	{"💡", ":bulb:", "Add or update comments in source code.", "docs"},
	{"🍻", ":beers:", "Write code drunkenly.", ""},
	{"💬", ":speech_balloon:", "Add or update text and literals.", ""},
	{"🗃️", ":card_file_box:", "Perform database related changes.", ""},
	{"🔊", ":loud_sound:", "Add or update logs.", ""},
	{"🔇", ":mute:", "Remove logs.", ""},
	{"👥", ":busts_in_silhouette:", "Add or update contributor(s).", "docs"},
	{"🚸", ":children_crossing:", "Improve user experience / usability.", "feat"},
	{"🏗️", ":building_construction:", "Make architectural changes.", "refactor"},
	{"📱", ":iphone:", "Work on responsive design.", "feat"},
	{"🤡", ":clown_face:", "Mock things.", "test"},
	{"🥚", ":egg:", "Add or update an easter egg.", "feat"},
	{"🙈", ":see_no_evil:", "Add or update a .gitignore file.", "chore"},
	{"📸", ":camera_flash:", "Add or update snapshots.", "test"},
	{"⚗️", ":alembic:", "Perform experiments.", ""},
	{"🔍️", ":mag:", "Improve SEO.", "feat"},
	{"🏷️", ":label:", "Add or update types.", "refactor"},
	{"🌱", ":seedling:", "Add or update seed files.", "chore"},
	{"🚩", ":triangular_flag_on_post:", "Add, update, or remove feature flags.", "feat"},
	{"🥅", ":goal_net:", "Catch errors.", "fix"},
	{"💫", ":dizzy:", "Add or update animations and transitions.", "feat"},
	{"🗑️", ":wastebasket:", "Deprecate code that needs to be cleaned up.", "refactor"},
	{"🛂", ":passport_control:", "Work on code related to authorization, roles and permissions.", "feat"},
	{"🩹", ":adhesive_bandage:", "Simple fix for a non-critical issue.", "fix"},
	{"🧐", ":monocle_face:", "Data exploration/inspection.", ""},
	{"⚰️", ":coffin:", "Remove dead code.", "refactor"},
	{"🧪", ":test_tube:", "Add a failing test.", "test"},
	{"👔", ":necktie:", "Add or update business logic.", "feat"},
	{"🩺", ":stethoscope:", "Add or update healthcheck.", "feat"},
	{"🧱", ":bricks:", "Infrastructure related changes.", "chore"},
	{"🧑‍💻", ":technologist:", "Improve developer experience.", "chore"},
	{"💸", ":money_with_wings:", "Add sponsorships or money related infrastructure.", "chore"},
	{"🧵", ":thread:", "Add or update code related to multithreading or concurrency.", ""},
	{"🦺", ":safety_vest:", "Add or update code related to validation.", "feat"},
	{"✈️", ":airplane:", "Improve offline support.", "feat"},
	{"🦖", ":t-rex:", "Code that adds backwards compatibility.", ""},
}

// baseOf is the emoji of each conventional type when nothing more specific
// applies. It follows the usual conventional-to-gitmoji tables.
var baseOf = map[string]string{
	"feat": ":sparkles:", "fix": ":bug:", "docs": ":memo:", "style": ":art:", "refactor": ":recycle:", "perf": ":zap:",
	"test": ":white_check_mark:", "build": ":package:", "ci": ":construction_worker:", "chore": ":wrench:", "revert": ":rewind:",
}

var byCode = func() map[string]Emoji {
	m := make(map[string]Emoji, len(All))
	for _, e := range All {
		m[e.Code] = e
	}
	return m
}()

// ByCode returns the emoji of a :shortcode:.
func ByCode(code string) (Emoji, bool) {
	e, ok := byCode[code]
	return e, ok
}

// ForType returns the base emoji of a conventional type.
func ForType(typ string) Emoji {
	if c, ok := baseOf[typ]; ok {
		return byCode[c]
	}
	return byCode[":wrench:"]
}

func get(code string) Emoji { return byCode[code] }

const vs16 = "️"

// Parse recognizes a gitmoji subject: "<emoji> [(scope)][:] <message>", the
// emoji in unicode (with or without the variation selector) or as a
// :shortcode:. It returns the emoji, the scope and the message.
func Parse(subject string) (e Emoji, scope, message string, ok bool) {
	s := strings.TrimSpace(subject)
	rest := ""
	if strings.HasPrefix(s, ":") {
		end := strings.IndexByte(s[1:], ':')
		if end < 0 {
			return Emoji{}, "", "", false
		}
		e, ok = byCode[s[:end+2]]
		if !ok {
			return Emoji{}, "", "", false
		}
		rest = s[end+2:]
	} else {
		best := 0
		for _, cand := range All {
			for _, u := range []string{cand.Unicode, strings.ReplaceAll(cand.Unicode, vs16, "")} {
				if len(u) > best && strings.HasPrefix(s, u) {
					e, best = cand, len(u)
				}
			}
		}
		if best == 0 {
			return Emoji{}, "", "", false
		}
		rest = s[best:]
	}
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, "(") {
		if end := strings.IndexByte(rest, ')'); end > 0 {
			scope = strings.TrimSpace(rest[1:end])
			rest = strings.TrimLeft(rest[end+1:], " \t")
		}
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	return e, scope, rest, true
}

// HasPrefix reports whether a subject starts with a gitmoji, whatever it means.
func HasPrefix(subject string) bool {
	_, _, _, ok := Parse(subject)
	return ok
}

// Format writes a gitmoji header: "✨ (auth): add login", "✨ add login",
// or with shortcode ":sparkles: (auth): add login". Without a subject the
// header ends after the scope colon (or the emoji), ready to be completed.
func Format(e Emoji, scope, subject string, shortcode bool) string {
	h := e.Unicode
	if shortcode {
		h = e.Code
	}
	if scope != "" {
		h += " (" + scope + "):"
	}
	if subject != "" {
		h += " " + subject
	}
	return h
}

var (
	reWIP        = regexp.MustCompile(`^\s*\[?wip\b`)
	reTypo       = regexp.MustCompile(`\btypos?\b`)
	reHotfix     = regexp.MustCompile(`\bhot-?fix\b`)
	reCIFix      = regexp.MustCompile(`\b(fix\w*|broken|fail\w*|green|flaky|unbreak)\b`)
	reA11y       = regexp.MustCompile(`\b(a11y|accessib\w*|aria|screen ?readers?)\b`)
	reI18n       = regexp.MustCompile(`\b(i18n|l10n|translat\w*|locali[sz]\w*)\b`)
	reAnalytics  = regexp.MustCompile(`\b(analytics|tracking|telemetry)\b`)
	reFlag       = regexp.MustCompile(`\bfeature[ -]?(flags?|toggles?)\b`)
	reAnimation  = regexp.MustCompile(`\b(animat\w*|transitions?)\b`)
	reResponsive = regexp.MustCompile(`\b(responsive|mobile layout|breakpoints?)\b`)
	reSEO        = regexp.MustCompile(`\bseo\b`)
	reAuthz      = regexp.MustCompile(`\b(authori[sz]\w*|permissions?|rbac|acl|roles?)\b`)
	reValidation = regexp.MustCompile(`\bvalidat\w*\b`)
	reDeprecate  = regexp.MustCompile(`\bdeprecat\w*\b`)
	reDeadCode   = regexp.MustCompile(`\b(dead code|unused)\b`)
	reArch       = regexp.MustCompile(`\barchitect\w*\b`)
	reLint       = regexp.MustCompile(`\b(lint\w*|warnings?|eslint|clippy|golangci|rubocop|flake8|pylint|staticcheck|go vet)\b`)
	reRelease    = regexp.MustCompile(`^\s*(release|bump (the )?version|prepare (the )?release|v?\d+\.\d+\.\d+)`)
	reFailing    = regexp.MustCompile(`\b(failing|red) tests?\b`)
	reBump       = regexp.MustCompile(`^\s*(bump|upgrade|update)\b`)
	reDowngrade  = regexp.MustCompile(`^\s*(downgrade|rollback|roll back)\b`)
	rePin        = regexp.MustCompile(`^\s*pin\b`)
	reUsesDep    = regexp.MustCompile(`^\s*-?\s*uses:\s*([^@\s]+)@(\S+)`)
	reFromDep    = regexp.MustCompile(`^\s*FROM\s+(?:--platform=\S+\s+)?([^:\s@]+)[:@](\S+)`)
	reLogLine    = regexp.MustCompile(`(?i)(console\.(log|debug|info|warn|error|trace)\(|\b(log|logger|logging|logrus|zap|slog|tracing)\w*[.:]{1,2}(debug|info|infof|warn|warnf|warning|error|errorf|trace|fatal|print\w*)\b|fmt\.Print|println!|eprintln!|dbg!|\bprint\(|System\.(out|err)\.print|error_log\(|var_dump\(|\bdd\(|\bdump\(|\bconsole\.dir\()`)
	reComment    = regexp.MustCompile(`^\s*(//|#|/\*|\*|--|;|<!--|-->|"""|''')`)
)

// Adapt bends a pick to the habits of a repository, given how many times
// each :shortcode: appears in its history. An emoji the repository (almost)
// never writes for that type gives way to the one it writes most: 💄 rather
// than 🎨 in a project that never formats code, 🐛 rather than 🔒 in one
// that never singles out security fixes. Without a gitmoji history the pick
// is unchanged.
func Adapt(e Emoji, typ string, used map[string]int) Emoji {
	if len(used) == 0 || e.typ != typ {
		return e // 🚧, 💥 and the like are not about the type
	}
	favorite, n := Emoji{}, 0
	for _, cand := range All {
		if cand.typ == typ && used[cand.Code] > n {
			favorite, n = cand, used[cand.Code]
		}
	}
	switch {
	case n < 3:
		return e // not enough history for this type
	case used[e.Code]*10 >= n:
		return e // a habit of this repository too
	case structural[e.Code] && used[e.Code] > 0:
		return e // the files leave no doubt, and the repository does write it
	}
	return favorite
}

// structural lists the intentions read from the files themselves rather
// than from the subject or a hunch: a repository that writes them at all
// gets them even when rare.
var structural = map[string]bool{":bookmark:": true, ":heavy_plus_sign:": true, ":heavy_minus_sign:": true, ":arrow_up:": true,
	":arrow_down:": true, ":pushpin:": true, ":see_no_evil:": true, ":page_facing_up:": true, ":busts_in_silhouette:": true,
	":truck:": true, ":bento:": true, ":camera_flash:": true, ":label:": true, ":globe_with_meridians:": true, ":card_file_box:": true,
	":green_heart:": true}

// Pick chooses the gitmoji of a change classified as typ. The draft subject
// (may be empty) is used for signals the diff cannot carry, such as "typo"
// or "hotfix".
func Pick(typ string, breaking bool, d *diff.Diff, subject string) Emoji {
	msg := strings.ToLower(subject)
	if reWIP.MatchString(msg) {
		return get(":construction:")
	}
	if typ == "revert" {
		return get(":rewind:")
	}
	if breaking {
		return get(":boom:")
	}
	s := summarize(d)
	// Signals that do not depend on the type: the files say it all.
	switch {
	case s.n == 0:
		return ForType(typ)
	case s.all(isGitignore):
		return get(":see_no_evil:")
	case s.all(isLicense):
		return get(":page_facing_up:")
	case s.all(isContributors):
		return get(":busts_in_silhouette:")
	case s.all(isBotConfig):
		return get(":wrench:") // dependabot/renovate files live under .github but are configuration
	case s.moves():
		return get(":truck:")
	case s.allCat(features.CatI18n) && typ != "test":
		return get(":globe_with_meridians:")
	case s.allCat(features.CatAsset) && typ != "docs" && typ != "test":
		return get(":bento:")
	case s.logs != 0 && typ != "docs" && typ != "test" && typ != "ci" && typ != "build":
		if s.logs < 0 {
			return get(":mute:")
		}
		return get(":loud_sound:")
	}
	switch typ {
	case "feat":
		switch {
		case s.all(isTypes):
			return get(":label:")
		case s.all(isDatabase):
			return get(":card_file_box:")
		case s.allCat(features.CatStyle):
			return get(":lipstick:")
		case reI18n.MatchString(msg):
			return get(":globe_with_meridians:")
		case reA11y.MatchString(msg):
			return get(":wheelchair:")
		case reAnalytics.MatchString(msg):
			return get(":chart_with_upwards_trend:")
		case reFlag.MatchString(msg):
			return get(":triangular_flag_on_post:")
		case reAnimation.MatchString(msg):
			return get(":dizzy:")
		case reResponsive.MatchString(msg):
			return get(":iphone:")
		case reSEO.MatchString(msg):
			return get(":mag:")
		case reAuthz.MatchString(msg):
			return get(":passport_control:")
		case reValidation.MatchString(msg):
			return get(":safety_vest:")
		case reDeprecate.MatchString(msg):
			return get(":wastebasket:")
		case s.pureRemoval():
			return get(":fire:")
		}
	case "fix":
		// Measured on gitmoji repositories: authors write 🐛 for nearly
		// every fix, including security, a11y and CSS ones. Only two
		// intentions are used consistently enough to be worth guessing.
		switch {
		case reHotfix.MatchString(msg):
			return get(":ambulance:")
		case s.allCat(features.CatCI):
			return get(":green_heart:")
		}
	case "docs":
		switch {
		case reTypo.MatchString(msg):
			return get(":pencil2:")
		case s.cat[features.CatDocs] == 0 && s.cat[features.CatChangelog] == 0 && s.comments():
			return get(":bulb:")
		}
	case "style":
		switch {
		case s.allCat(features.CatStyle):
			return get(":lipstick:")
		case reLint.MatchString(msg):
			return get(":rotating_light:")
		case reTypo.MatchString(msg):
			return get(":pencil2:")
		}
	case "refactor":
		switch {
		case reDeadCode.MatchString(msg) && s.pureRemoval():
			return get(":coffin:")
		case s.pureRemoval():
			return get(":fire:")
		case reDeprecate.MatchString(msg):
			return get(":wastebasket:")
		case reArch.MatchString(msg):
			return get(":building_construction:")
		case s.all(isTypes):
			return get(":label:")
		case s.all(isDatabase):
			return get(":card_file_box:")
		}
	case "test":
		switch {
		case s.all(isMock):
			return get(":clown_face:")
		case s.allCat(features.CatSnapshot):
			return get(":camera_flash:")
		case reFailing.MatchString(msg):
			return get(":test_tube:")
		}
	case "build", "chore", "ci":
		if e := s.dependencies(); !e.IsZero() {
			return e
		}
		switch {
		case typ == "ci" && reCIFix.MatchString(msg):
			return get(":green_heart:")
		case s.versionBump() || typ != "ci" && reRelease.MatchString(msg):
			return get(":bookmark:")
		case reBump.MatchString(msg) && s.cat[features.CatBuild]+s.cat[features.CatLock]+s.cat[features.CatCI] > 0:
			return get(":arrow_up:") // "bump x from a to b" with a manifest in the diff, even when the lines could not be parsed
		case reDowngrade.MatchString(msg) && s.cat[features.CatBuild]+s.cat[features.CatLock] > 0:
			return get(":arrow_down:")
		case rePin.MatchString(msg) && s.cat[features.CatBuild]+s.cat[features.CatLock] > 0:
			return get(":pushpin:")
		case typ == "ci" && s.allCat(features.CatCI):
			return get(":construction_worker:")
		case typ == "chore" && reTypo.MatchString(msg):
			return get(":pencil2:")
		case typ == "chore" && reDeprecate.MatchString(msg):
			return get(":wastebasket:")
		case typ == "chore" && s.all(isTypes):
			return get(":label:")
		}
	}
	return ForType(typ)
}

// summary is what Pick needs to know about a diff.
type summary struct {
	files   []diff.File
	n       int
	cat     map[features.Category]int
	added   int
	removed int
	renamed int
	deleted int
	logs    int // +1 only log lines added, -1 only log lines removed, 0 otherwise
}

func summarize(d *diff.Diff) *summary {
	s := &summary{cat: map[features.Category]int{}}
	if d == nil {
		return s
	}
	s.files = d.Files
	s.n = len(d.Files)
	seen, logAdded, logRemoved, other := 0, 0, 0, 0
	for _, f := range d.Files {
		s.cat[features.Classify(f.Path)]++
		s.added += f.Added
		s.removed += f.Removed
		switch f.Status {
		case diff.Renamed, diff.Copied:
			s.renamed++
		case diff.Deleted:
			s.deleted++
		}
		for _, l := range f.Lines {
			if strings.TrimSpace(l.Text) == "" {
				continue
			}
			seen++
			switch {
			case !reLogLine.MatchString(l.Text):
				other++
			case l.Added:
				logAdded++
			default:
				logRemoved++
			}
		}
	}
	// Only trust the line analysis when the parser kept every line.
	if other == 0 && seen > 0 && !d.Truncated {
		switch {
		case logRemoved > 0 && logAdded == 0:
			s.logs = -1
		case logAdded > 0:
			s.logs = 1
		}
	}
	return s
}

func (s *summary) all(pred func(f diff.File) bool) bool {
	if s.n == 0 {
		return false
	}
	for _, f := range s.files {
		if !pred(f) {
			return false
		}
	}
	return true
}

func (s *summary) allCat(c features.Category) bool { return s.n > 0 && s.cat[c] == s.n }

// pureRemoval: lines were removed and none added.
func (s *summary) pureRemoval() bool { return s.removed > 0 && s.added == 0 }

// moves: the change is essentially renames, with at most a few edited lines
// per file (import paths, package clauses).
func (s *summary) moves() bool {
	if s.renamed == 0 || s.renamed*2 < s.n {
		return false
	}
	for _, f := range s.files {
		if f.Added+f.Removed > 10 {
			return false
		}
	}
	return true
}

// comments: every changed line is a comment line.
func (s *summary) comments() bool {
	seen := 0
	for _, f := range s.files {
		if f.Binary {
			return false
		}
		for _, l := range f.Lines {
			if strings.TrimSpace(l.Text) == "" {
				continue
			}
			if !reComment.MatchString(l.Text) {
				return false
			}
			seen++
		}
	}
	return seen > 0
}

func base(f diff.File) string { return strings.ToLower(path.Base(f.Path)) }

func isGitignore(f diff.File) bool { return base(f) == ".gitignore" }

var licenseNames = map[string]bool{"license": true, "license.md": true, "license.txt": true, "licence": true, "licence.md": true, "licence.txt": true,
	"copying": true, "copying.md": true, "copying.txt": true, "unlicense": true, "notice": true, "notice.md": true, "notice.txt": true, "license-mit": true, "license-apache": true}

func isLicense(f diff.File) bool {
	lp := strings.ToLower(f.Path)
	return licenseNames[base(f)] || strings.HasPrefix(lp, "licenses/") || strings.HasPrefix(lp, "license/")
}

var contributorNames = map[string]bool{"authors": true, "authors.md": true, "authors.txt": true, "contributors": true, "contributors.md": true,
	"contributors.txt": true, "maintainers": true, "maintainers.md": true, ".mailmap": true, "humans.txt": true, ".all-contributorsrc": true}

func isContributors(f diff.File) bool { return contributorNames[base(f)] }

func isBotConfig(f diff.File) bool {
	b := base(f)
	return b == "dependabot.yml" || b == "dependabot.yaml" || b == "renovate.json" || b == "renovate.json5" || strings.HasPrefix(b, ".renovaterc")
}

func isTypes(f diff.File) bool {
	b := base(f)
	lp := strings.ToLower(f.Path)
	return strings.HasSuffix(b, ".d.ts") || strings.HasSuffix(b, ".d.mts") || strings.HasSuffix(b, ".d.cts") || strings.HasSuffix(b, ".pyi") ||
		strings.HasSuffix(b, ".flow") || strings.HasPrefix(lp, "types/") || strings.Contains(lp, "/types/") || strings.HasPrefix(lp, "@types/") || strings.Contains(lp, "/@types/")
}

func isDatabase(f diff.File) bool {
	b := base(f)
	lp := strings.ToLower(f.Path)
	return strings.HasSuffix(b, ".sql") || strings.HasSuffix(b, ".prisma") || strings.Contains(lp, "migration") || strings.Contains(lp, "db/migrate/") ||
		strings.Contains(lp, "alembic/versions/") || strings.Contains(lp, "/seeds/") || strings.Contains(lp, "/seeders/")
}

func isMock(f diff.File) bool {
	b := base(f)
	lp := strings.ToLower(f.Path)
	return strings.Contains(lp, "__mocks__/") || strings.Contains(lp, "/mocks/") || strings.Contains(b, "mock") || strings.Contains(b, "stub") || strings.Contains(b, "fake")
}

// --- dependencies -----------------------------------------------------------

// manifests maps a dependency manifest to the parser of one of its lines.
var manifests = map[string]func(line string) (name, version string, ok bool){
	"package.json": parseJSONDep, "composer.json": parseJSONDep, "deno.json": parseJSONDep, "deno.jsonc": parseJSONDep, "bower.json": parseJSONDep,
	"go.mod":         parseGoDep,
	"cargo.toml":     parseTOMLDep,
	"pyproject.toml": parseTOMLDep,
	"pipfile":        parseTOMLDep,
	"gemfile":        parseGemDep,
	"pubspec.yaml":   parseYAMLDep,
	"mix.exs":        parseMixDep,
	"build.gradle":   parseGradleDep, "build.gradle.kts": parseGradleDep,
	"package.swift": parseSwiftDep,
}

var (
	reJSONDep   = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*"([^"]*)"\s*,?\s*$`)
	reGoDep     = regexp.MustCompile(`^\s*([A-Za-z0-9][^\s]*[./][^\s]*)\s+(v\d[^\s]*)`)
	reTOMLDep   = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-]+)\s*=\s*(?:"([^"]*)"|\{[^}]*version\s*=\s*"([^"]*)")`)
	rePyListDep = regexp.MustCompile(`^\s*"([A-Za-z0-9_.\-]+)(?:\[[^\]]*\])?\s*([<>=!~^][^"]*)?"\s*,?\s*$`)
	reReqDep    = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-]+)(?:\[[^\]]*\])?\s*(?:([<>=!~]{1,2})\s*([^\s;#]+))?`)
	reGemDep    = regexp.MustCompile(`^\s*gem\s+['"]([^'"]+)['"](?:\s*,\s*['"]([^'"]+)['"])?`)
	reYAMLDep   = regexp.MustCompile(`^\s{2,}([A-Za-z0-9_\-]+):\s*([\^~>=<]*\d[^\s#]*|any)?\s*$`)
	reMixDep    = regexp.MustCompile(`\{\s*:([a-z0-9_]+)\s*,\s*"([^"]*)"`)
	reGradleDep = regexp.MustCompile(`['"]([^'":\s]+:[^'":\s]+):([^'"\s]+)['"]`)
	reSwiftDep  = regexp.MustCompile(`\.package\(.*?url:\s*"([^"]+)".*?(?:from:|exact:|branch:|revision:|\.upToNextMajor\(from:|\.upToNextMinor\(from:)\s*"([^"]+)"`)
	reVersion   = regexp.MustCompile(`^(?:[\^~>=<!*v ]|==)*\d`)
)

var jsonNotDeps = map[string]bool{"name": true, "version": true, "main": true, "module": true, "types": true, "description": true, "license": true,
	"author": true, "homepage": true, "type": true, "packagemanager": true, "node": true, "npm": true, "pnpm": true, "yarn": true, "bun": true}

func parseJSONDep(line string) (string, string, bool) {
	m := reJSONDep.FindStringSubmatch(line)
	if m == nil || jsonNotDeps[strings.ToLower(m[1])] || !looksLikeVersion(m[2]) {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseGoDep(line string) (string, string, bool) {
	l := strings.TrimSpace(line)
	if strings.HasPrefix(l, "//") || strings.HasPrefix(l, "go ") || strings.HasPrefix(l, "toolchain ") || strings.HasPrefix(l, "module ") || strings.HasPrefix(l, "replace ") || strings.HasPrefix(l, "exclude ") {
		return "", "", false
	}
	l = strings.TrimPrefix(l, "require ")
	m := reGoDep.FindStringSubmatch(l)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var tomlNotDeps = map[string]bool{"name": true, "version": true, "edition": true, "description": true, "license": true, "readme": true,
	"repository": true, "homepage": true, "documentation": true, "rust-version": true, "requires-python": true, "python": true, "python_version": true,
	"build-backend": true, "authors": true, "keywords": true, "categories": true, "publish": true, "exclude": true, "include": true, "default-run": true}

func parseTOMLDep(line string) (string, string, bool) {
	if m := reTOMLDep.FindStringSubmatch(line); m != nil {
		if tomlNotDeps[strings.ToLower(m[1])] {
			return "", "", false
		}
		v := m[2]
		if v == "" {
			v = m[3]
		}
		if !looksLikeVersion(v) && v != "*" {
			return "", "", false
		}
		return m[1], v, true
	}
	if m := rePyListDep.FindStringSubmatch(line); m != nil { // PEP 621 dependencies = ["requests>=2"]
		return m[1], m[2], true
	}
	return "", "", false
}

func parseRequirementsDep(line string) (string, string, bool) {
	l := strings.TrimSpace(line)
	if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "-") {
		return "", "", false
	}
	m := reReqDep.FindStringSubmatch(l)
	if m == nil || m[1] == "" {
		return "", "", false
	}
	return m[1], m[2] + m[3], true
}

func parseGemDep(line string) (string, string, bool) {
	m := reGemDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var yamlNotDeps = map[string]bool{"sdk": true, "flutter": true, "version": true, "name": true, "description": true, "environment": true, "path": true, "git": true, "ref": true, "url": true}

func parseYAMLDep(line string) (string, string, bool) {
	m := reYAMLDep.FindStringSubmatch(line)
	if m == nil || yamlNotDeps[m[1]] {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseMixDep(line string) (string, string, bool) {
	m := reMixDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseGradleDep(line string) (string, string, bool) {
	m := reGradleDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseSwiftDep(line string) (string, string, bool) {
	m := reSwiftDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func looksLikeVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v == "*" || v == "latest" || reVersion.MatchString(v) || strings.HasPrefix(v, "workspace:") || strings.HasPrefix(v, "npm:") ||
		strings.HasPrefix(v, "file:") || strings.HasPrefix(v, "link:") || strings.HasPrefix(v, "git+") || strings.HasPrefix(v, "github:")
}

func manifestParser(f diff.File) func(string) (string, string, bool) {
	b := base(f)
	if p, ok := manifests[b]; ok {
		return p
	}
	switch {
	case strings.HasPrefix(b, "requirements") && strings.HasSuffix(b, ".txt") || b == "constraints.txt":
		return parseRequirementsDep
	case strings.HasPrefix(b, "dockerfile") || strings.HasSuffix(b, ".dockerfile"):
		return parseDockerDep
	case features.Classify(f.Path) == features.CatCI && (strings.HasSuffix(b, ".yml") || strings.HasSuffix(b, ".yaml")):
		return parseUsesDep // GitHub Actions pinned in workflows are dependencies too
	}
	return nil
}

func parseUsesDep(line string) (string, string, bool) {
	m := reUsesDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func parseDockerDep(line string) (string, string, bool) {
	m := reFromDep.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// dependencies returns the dependency emoji when the change is only about
// dependencies: manifests and lock files, and every changed manifest line is
// a dependency line. Additions give ➕, removals ➖, upgrades ⬆️, downgrades
// ⬇️, and pinning a range to an exact version 📌. Mixed changes give the
// generic 📦️, and a change to lock files alone is an upgrade (what a bot
// bumping a transitive dependency produces). The zero Emoji means the change
// is not a dependency change.
func (s *summary) dependencies() Emoji {
	type change struct{ old, new string }
	deps := map[string]*change{}
	manifestFiles, lockFiles := 0, 0
	for _, f := range s.files {
		parse := manifestParser(f)
		if parse == nil {
			if features.Classify(f.Path) == features.CatLock {
				lockFiles++
				continue
			}
			return Emoji{}
		}
		manifestFiles++
		if len(f.Lines) < f.Added+f.Removed {
			return Emoji{} // some lines were dropped by the parser: do not guess
		}
		for _, l := range f.Lines {
			if strings.TrimSpace(l.Text) == "" || (base(f) == "package.json" || base(f) == "composer.json") && strings.Contains(l.Text, `"version"`) {
				continue // a version bump next to a dependency bump is still about dependencies
			}
			name, version, ok := parse(l.Text)
			if !ok {
				return Emoji{}
			}
			c := deps[name]
			if c == nil {
				c = &change{}
				deps[name] = c
			}
			if l.Added {
				c.new = version
			} else {
				c.old = version
			}
		}
	}
	if manifestFiles == 0 {
		if lockFiles > 0 && lockFiles == s.n {
			return get(":arrow_up:")
		}
		return Emoji{}
	}
	if len(deps) == 0 {
		return Emoji{}
	}
	added, removed, up, down, pinned, other := 0, 0, 0, 0, 0, 0
	for _, c := range deps {
		switch {
		case c.old == "" && c.new != "":
			added++
		case c.new == "" && c.old != "":
			removed++
		case c.old == c.new:
			other++
		case isRange(c.old) && !isRange(c.new) && sameBase(c.old, c.new):
			pinned++
		default:
			switch compareVersions(c.old, c.new) {
			case 1:
				up++
			case -1:
				down++
			default:
				other++
			}
		}
	}
	total := added + removed + up + down + pinned + other
	switch {
	case total == 0:
		return Emoji{}
	case added == total:
		return get(":heavy_plus_sign:")
	case removed == total:
		return get(":heavy_minus_sign:")
	case up == total || up > 0 && up+pinned == total:
		return get(":arrow_up:")
	case down == total:
		return get(":arrow_down:")
	case pinned == total:
		return get(":pushpin:")
	}
	return get(":package:")
}

// truncatedLines reports whether some changed lines were dropped by the parser.
func (s *summary) truncatedLines() bool {
	for _, f := range s.files {
		if len(f.Lines) < f.Added+f.Removed && !f.Binary {
			return true
		}
	}
	return false
}

func isRange(v string) bool {
	v = strings.TrimSpace(v)
	return v == "*" || v == "latest" || strings.ContainsAny(v, "^~<>*x") || strings.Contains(v, " - ") || strings.Contains(v, "||")
}

// sameBase: "^1.2.0" pinned to "1.2.0" or "1.2.5".
func sameBase(rng, exact string) bool {
	a, b := versionNumbers(rng), versionNumbers(exact)
	return len(a) > 0 && len(b) > 0 && a[0] == b[0]
}

var reNumbers = regexp.MustCompile(`\d+`)

func versionNumbers(v string) []int {
	var out []int
	for _, n := range reNumbers.FindAllString(v, 4) {
		x, _ := strconv.Atoi(n)
		out = append(out, x)
	}
	return out
}

// compareVersions returns 1 when b is newer than a, -1 when older, 0 when
// it cannot tell. A pre-release ("2.0.0-rc.1") is older than its release.
func compareVersions(a, b string) int {
	aBase, aPre := splitPrerelease(a)
	bBase, bPre := splitPrerelease(b)
	x, y := versionNumbers(aBase), versionNumbers(bBase)
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	for i := 0; i < len(x) && i < len(y); i++ {
		if y[i] > x[i] {
			return 1
		}
		if y[i] < x[i] {
			return -1
		}
	}
	switch {
	case len(y) > len(x):
		return 1
	case len(y) < len(x):
		return -1
	case aPre != "" && bPre == "":
		return 1
	case aPre == "" && bPre != "":
		return -1
	case aPre < bPre:
		return 1
	case aPre > bPre:
		return -1
	}
	return 0
}

func splitPrerelease(v string) (string, string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '-'); i > 0 && i+1 < len(v) && v[i+1] != ' ' {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// versionBump: the change only touches version manifests, changelogs, lock
// files and the "version" field of package manifests.
func (s *summary) versionBump() bool {
	bumped := false
	for _, f := range s.files {
		switch features.Classify(f.Path) {
		case features.CatVersion:
			bumped = true
			continue
		case features.CatChangelog, features.CatLock:
			continue
		}
		if manifestParser(f) == nil {
			return false
		}
		for _, l := range f.Lines {
			t := strings.TrimSpace(l.Text)
			if t == "" {
				continue
			}
			if !(strings.HasPrefix(t, `"version"`) || strings.HasPrefix(t, "version ") || strings.HasPrefix(t, "version=") || strings.HasPrefix(t, "version:")) {
				return false
			}
			bumped = true
		}
	}
	return bumped && !s.truncatedLines()
}
