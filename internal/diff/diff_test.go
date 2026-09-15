package diff

import "testing"

func TestParseEdgeCases(t *testing.T) {
	in := `diff --git a/src/a b.txt b/src/a b.txt
index 1..2 100644
--- a/src/a b.txt	
+++ b/src/a b.txt	
@@ -1 +1 @@
-old
+new
diff --git "a/caf\303\251.md" "b/caf\303\251.md"
new file mode 100644
--- /dev/null
+++ "b/caf\303\251.md"
@@ -0,0 +1,2 @@
+hello
+world
diff --git a/old/name.go b/new/name.go
similarity index 90%
rename from old/name.go
rename to new/name.go
index 3..4 100644
--- a/old/name.go
+++ b/new/name.go
@@ -10,3 +10,3 @@ func main() {
-	x := 1
+	x := 2
diff --git a/img.png b/img.png
new file mode 100644
Binary files /dev/null and b/img.png differ
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
\ No newline at end of file
diff --git a/script.sh b/script.sh
old mode 100644
new mode 100755
`
	d := ParseString(in)
	if len(d.Files) != 6 {
		t.Fatalf("got %d files", len(d.Files))
	}
	f := d.Files
	if f[0].Path != "src/a b.txt" || f[0].Added != 1 || f[0].Removed != 1 {
		t.Errorf("space path: %+v", f[0])
	}
	if f[1].Path != "café.md" || f[1].Status != Added || f[1].Added != 2 {
		t.Errorf("quoted path: %+v", f[1])
	}
	if f[2].Path != "new/name.go" || f[2].OldPath != "old/name.go" || f[2].Status != Renamed || len(f[2].Hunks) != 1 || f[2].Hunks[0] != "func main() {" {
		t.Errorf("rename: %+v", f[2])
	}
	if !f[3].Binary || f[3].Status != Added {
		t.Errorf("binary: %+v", f[3])
	}
	if f[4].Status != Deleted || f[4].Removed != 1 || f[4].Added != 0 {
		t.Errorf("deleted: %+v", f[4])
	}
	if f[5].Mode != "100755" {
		t.Errorf("mode: %+v", f[5])
	}
}

func TestParsePlainUnified(t *testing.T) {
	in := `--- a/foo.py
+++ b/foo.py
@@ -1,2 +1,2 @@
-print 1
+print(1)
`
	d := ParseString(in)
	if len(d.Files) != 1 || d.Files[0].Path != "foo.py" || d.Files[0].Added != 1 {
		t.Fatalf("%+v", d.Files)
	}
}

func TestParseGarbage(t *testing.T) {
	d := ParseString("hello world\nnot a diff\n")
	if len(d.Files) != 0 {
		t.Fatal("expected no files")
	}
}

func TestCaps(t *testing.T) {
	in := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n"
	for i := 0; i < MaxLinesPerFile+10; i++ {
		in += "+line\n"
	}
	d := ParseString(in)
	if !d.Truncated || len(d.Files[0].Lines) != MaxLinesPerFile || d.Files[0].Added != MaxLinesPerFile+10 {
		t.Fatalf("truncated=%v lines=%d added=%d", d.Truncated, len(d.Files[0].Lines), d.Files[0].Added)
	}
}

func TestParseNoPrefixAndQuotedRename(t *testing.T) {
	in := "diff --git foo bar.txt foo bar.txt\nindex 1..2 100644\n--- foo bar.txt\n+++ foo bar.txt\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git \"a/z\\\"y.go\" \"b/z\\\"x.go\"\nsimilarity index 100%\nrename from \"z\\\"y.go\"\nrename to \"z\\\"x.go\"\n" +
		"diff --git i/m.go w/m.go\n--- i/m.go\n+++ w/m.go\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --cc merged.go\nindex 1,2..3\n--- a/merged.go\n+++ b/merged.go\n@@@ -1,2 -1,2 +1,2 @@@\n  x\n -a\n +b\n"
	d := ParseString(in)
	if len(d.Files) != 4 {
		t.Fatalf("files=%d", len(d.Files))
	}
	if d.Files[0].Path != "foo bar.txt" {
		t.Errorf("noprefix: %q", d.Files[0].Path)
	}
	if d.Files[1].Path != `z"x.go` || d.Files[1].OldPath != `z"y.go` || d.Files[1].Status != Renamed {
		t.Errorf("quoted rename: %+v", d.Files[1])
	}
	if d.Files[2].Path != "m.go" {
		t.Errorf("mnemonic prefix: %q", d.Files[2].Path)
	}
	if d.Files[3].Path != "merged.go" {
		t.Errorf("combined: %q", d.Files[3].Path)
	}
}
