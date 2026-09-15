package model

import (
	"math"
	"testing"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/features"
)

func TestDefaultModel(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Classes) != 11 || m.Classes[0] != "feat" {
		t.Fatalf("classes: %v", m.Classes)
	}
	d := diff.ParseString("diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1,2 @@\n # hello\n+Some documentation about usage.\n")
	v := features.Extract(d, features.Options{KeepNames: true})
	c := m.Predict(v)
	var sum float64
	for _, x := range c {
		sum += x.P
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("probabilities sum to %f", sum)
	}
	if c[0].Type != "docs" {
		t.Errorf("README change predicted as %s (%.2f)", c[0].Type, c[0].P)
	}
	pos, neg := m.Explain(v, m.ClassIndex(c[0].Type), m.ClassIndex(c[1].Type), 5)
	if len(pos) == 0 {
		t.Errorf("no positive contributions")
	}
	_ = neg
	// prior adaptation keeps a valid distribution and moves mass toward the repo habits
	p := m.Probabilities(m.Logits(v))
	adapted := m.AdaptPrior(p, map[string]int{"chore": 90, "docs": 10}, 0.5)
	sum = 0
	for _, x := range adapted {
		sum += x
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("adapted sum %f", sum)
	}
	if adapted[m.ClassIndex("chore")] <= p[m.ClassIndex("chore")] {
		t.Errorf("prior did not increase chore: %f -> %f", p[m.ClassIndex("chore")], adapted[m.ClassIndex("chore")])
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	if _, err := Load([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Load(append([]byte("CCM1"), make([]byte, 24)...)); err == nil {
		t.Fatal("expected error on wrong feature version")
	}
}

func BenchmarkPredict(b *testing.B) {
	m, _ := Default()
	d := diff.ParseString("diff --git a/src/x.go b/src/x.go\n--- a/src/x.go\n+++ b/src/x.go\n@@ -1 +1,3 @@\n package x\n+func Foo() error { return nil }\n+var y = 1\n")
	v := features.Extract(d, features.Options{})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Predict(v)
	}
}
