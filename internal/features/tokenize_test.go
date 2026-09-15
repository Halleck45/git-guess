package features

import (
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := map[string][]string{
		"getUserName(id, 42)":          {"get", "user", "name", "id", "0"},
		"const HTTPServer2 = new Foo;": {"const", "http", "server", "0", "new", "foo"},
		"x":                            nil,
		"foo_bar-baz.qux":              {"foo", "bar", "baz", "qux"},
		"if err != nil {":              {"if", "err", "nil"},
	}
	for in, want := range cases {
		got := tokenizeInto(nil, in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestLineShape(t *testing.T) {
	cases := map[string]string{
		"  return nil":       "return",
		"// comment":         "<comment>",
		"}":                  "<close>",
		"  <div>":            "<tag>",
		"@Injectable()":      "@injectable",
		"#include <stdio.h>": "#include",
		"# a python comment": "<comment>",
		"'use strict'":       "<string>",
		"":                   "<empty>",
	}
	for in, want := range cases {
		if got := lineShape(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]Category{
		"src/foo.go":                        CatSource,
		"src/foo_test.go":                   CatTest,
		"packages/core/__tests__/x.spec.ts": CatTest,
		"tests/unit/test_foo.py":            CatTest,
		"docs/guide/intro.md":               CatDocs,
		"README.md":                         CatDocs,
		".github/workflows/ci.yml":          CatCI,
		".github/CODEOWNERS":                CatConfig,
		"package.json":                      CatBuild,
		"go.mod":                            CatBuild,
		"pnpm-lock.yaml":                    CatLock,
		"CHANGELOG.md":                      CatChangelog,
		"LICENSE":                           CatLicense,
		"src/styles/app.scss":               CatStyle,
		"locales/fr.json":                   CatI18n,
		"assets/logo.png":                   CatAsset,
		"src/__snapshots__/a.snap":          CatSnapshot,
		"vendor/lib/x.go":                   CatGenerated,
		"api/v1/foo.pb.go":                  CatGenerated,
		"db/migrate/20240101_add_users.rb":  CatSchema,
		"scripts/release.sh":                CatScript,
		"templates/index.html.twig":         CatTemplate,
		"config/app.yaml":                   CatData,
		"Dockerfile":                        CatBuild,
		"Makefile":                          CatBuild,
		"src/version.go":                    CatVersion,
		"src/main/java/com/x/FooTest.java":  CatTest,
		"src/test/java/com/x/Foo.java":      CatTest,
		"examples/basic/main.go":            CatDocs,
		"cypress/e2e/login.cy.ts":           CatTest,
		".eslintrc.js":                      CatConfig,
		"crates/foo/benches/bench.rs":       CatTest,
		"doc/source/whatsnew/v2.0.0.rst":    CatChangelog,
		"src/components/Button.stories.tsx": CatTest,
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
