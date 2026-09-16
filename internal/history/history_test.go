package history

import (
	"math"
	"testing"

	"github.com/Halleck45/conventional/internal/diff"
	"github.com/Halleck45/conventional/internal/features"
	"github.com/Halleck45/conventional/internal/model"
)

func TestNormalizeDot(t *testing.T) {
	a := Normalize([]features.Sparse{{Index: 1, Value: 3}, {Index: 5, Value: 4}})
	if math.Abs(Dot(a, a)-1) > 1e-6 {
		t.Fatalf("norm: %f", Dot(a, a))
	}
	b := Normalize([]features.Sparse{{Index: 5, Value: 1}})
	if math.Abs(Dot(a, b)-0.8) > 1e-6 {
		t.Fatalf("dot: %f", Dot(a, b))
	}
	if Dot(a, nil) != 0 {
		t.Fatal("empty")
	}
}

func TestGather(t *testing.T) {
	m, err := model.Default()
	if err != nil {
		t.Fatal(err)
	}
	docs := diff.ParseString("diff --git a/docs/a.md b/docs/a.md\n--- a/docs/a.md\n+++ b/docs/a.md\n@@ -1 +1,2 @@\n # a\n+more words about usage\n")
	code := diff.ParseString("diff --git a/src/x.go b/src/x.go\n--- a/src/x.go\n+++ b/src/x.go\n@@ -1 +1,2 @@\n package x\n+func Foo() error { return nil }\n")
	idx := &Index{}
	for i := 0; i < 30; i++ {
		d := docs
		typ := uint8(m.ClassIndex("chore")) // this repo calls docs changes "chore"
		if i%2 == 1 {
			d = code
			typ = uint8(m.ClassIndex("feat"))
		}
		e := newEntry(m, "sha", d)
		e.Type = typ
		idx.Entries = append(idx.Entries, *e)
	}
	v := features.Extract(docs, features.Options{})
	z := m.Logits(v)
	ev := idx.Gather(m, v.Sparse, []string{"docs/a.md"}, z, -1)
	if ev.Size != 30 || ev.MaxSim < 0.99 || !ev.Seen {
		t.Fatalf("evidence: %+v", ev)
	}
	chore := m.ClassIndex("chore")
	if ev.KNN[chore] < 0.8 || ev.File[chore] < 0.6 {
		t.Errorf("local evidence should point to chore: knn=%v file=%v", ev.KNN, ev.File)
	}
	for _, dist := range [][]float64{ev.KNN, ev.Conf, ev.File} {
		var s float64
		for _, x := range dist {
			s += x
		}
		if math.Abs(s-1) > 1e-6 {
			t.Errorf("not a distribution: %v", dist)
		}
	}
	if got := idx.Gather(m, v.Sparse, nil, z, 5); got.Size != 5 {
		t.Errorf("limit ignored: %d", got.Size)
	}
	f := model.MetaFeatures(m.Probabilities(z), ev.KNN, ev.Conf, ev.File, ev.Seen, ev.Size, ev.MaxSim)
	if len(f) != 47 {
		t.Fatalf("meta features: %d", len(f))
	}
}
