package features

import (
	"testing"

	"github.com/Halleck45/conventional/internal/diff"
)

const sample = `diff --git a/src/auth/login.ts b/src/auth/login.ts
index 1111111..2222222 100644
--- a/src/auth/login.ts
+++ b/src/auth/login.ts
@@ -10,6 +10,9 @@ export function login(user: User) {
   const token = createToken(user);
+  if (!user.email) {
+    throw new Error("missing email");
+  }
   return token;
 }
diff --git a/src/auth/login.test.ts b/src/auth/login.test.ts
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/src/auth/login.test.ts
@@ -0,0 +1,5 @@
+describe("login", () => {
+  it("throws without email", () => {
+    expect(() => login({})).toThrow();
+  });
+});
`

func TestExtractSample(t *testing.T) {
	d := diff.ParseString(sample)
	if len(d.Files) != 2 {
		t.Fatalf("files: %d", len(d.Files))
	}
	if d.Files[1].Status != diff.Added || d.Files[1].Added != 5 {
		t.Fatalf("second file: %+v", d.Files[1])
	}
	v := Extract(d, Options{KeepNames: true})
	if len(v.Dense) != NumDense {
		t.Fatalf("dense len %d", len(v.Dense))
	}
	for i, n := range DenseNames {
		if n == "" {
			t.Errorf("dense feature %d has no name", i)
		}
	}
	if v.Dense[dFracCat+int(CatTest)] != 0.5 || v.Dense[dFilesTouchingTestsAndSource] != 1 {
		t.Errorf("category fractions wrong: %v", v.Dense[dFracCat:dFracCat+int(numCategories)])
	}
	want := map[string]bool{"p:src": true, "pp:src/auth": true, "e:ts": true, "c:test": true, "cs:test_added": true, "a:email": true, "A1:if": true, "h:login": true, "x:describe": true}
	for _, n := range v.Names {
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("missing features: %v", want)
	}
	base, msg := ExtractParts(d, "add email validation")
	if len(msg) == 0 || base.Dense[HasMessageIndex] != 0 {
		t.Errorf("parts: msg=%d has_message=%v", len(msg), base.Dense[HasMessageIndex])
	}
	full := Extract(d, Options{Message: "add email validation"})
	if len(full.Sparse) != len(base.Sparse)+len(msg) && full.Dense[HasMessageIndex] != 1 {
		t.Errorf("full vs parts mismatch")
	}
	// determinism
	v2 := Extract(d, Options{})
	if len(v2.Sparse) != len(v.Sparse) {
		t.Fatal("non deterministic")
	}
	for i := range v.Sparse {
		if v.Sparse[i] != v2.Sparse[i] {
			t.Fatal("non deterministic values")
		}
	}
}

func BenchmarkExtract(b *testing.B) {
	d := diff.ParseString(sample)
	for i := 0; i < b.N; i++ {
		Extract(d, Options{})
	}
}
