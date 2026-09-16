package model

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

//go:embed meta.bin
var embeddedMeta []byte

// Meta combines the global probabilities with local evidence. It is a small
// multinomial logistic regression trained on chronological simulations of
// repository histories (scripts/train_meta.py).
type Meta struct {
	NumFeatures int
	weights     []float32 // NumFeatures × C
	bias        []float32
}

var (
	metaOnce sync.Once
	metaM    *Meta
	metaErr  error
)

// DefaultMeta returns the embedded meta-model.
func DefaultMeta() (*Meta, error) {
	metaOnce.Do(func() { metaM, metaErr = LoadMeta(embeddedMeta) })
	return metaM, metaErr
}

// LoadMeta parses a meta-model.
func LoadMeta(b []byte) (*Meta, error) {
	if len(b) < 12 || string(b[:4]) != "CCMM" {
		return nil, errors.New("meta: bad magic")
	}
	le := binary.LittleEndian
	n, C := int(le.Uint32(b[4:])), int(le.Uint32(b[8:]))
	if n <= 0 || n > 1024 || C < 2 || C > 255 || len(b) < 12+4*n*C+4*C {
		return nil, errors.New("meta: truncated")
	}
	m := &Meta{NumFeatures: n, weights: readF32(b[12:], n*C), bias: readF32(b[12+4*n*C:], C)}
	return m, nil
}

// Predict returns probabilities for a feature vector of length NumFeatures.
func (m *Meta) Predict(f []float64) []float64 {
	C := len(m.bias)
	z := make([]float64, C)
	for c := range z {
		z[c] = float64(m.bias[c])
	}
	for i, x := range f {
		if i >= m.NumFeatures {
			break
		}
		row := m.weights[i*C : i*C+C]
		for c := range z {
			z[c] += x * float64(row[c])
		}
	}
	mx := math.Inf(-1)
	for _, v := range z {
		mx = math.Max(mx, v)
	}
	var sum float64
	p := make([]float64, C)
	for c, v := range z {
		p[c] = math.Exp(v - mx)
		sum += p[c]
	}
	for c := range p {
		p[c] /= sum
	}
	return p
}

// MetaFeatures builds the meta-model input from global probabilities and
// local evidence, in the layout documented in scripts/train_meta.py.
func MetaFeatures(p, knn, conf, file []float64, seen bool, size int, maxSim float64) []float64 {
	f := make([]float64, 0, 4*len(p)+3)
	lg := func(v []float64) {
		for _, x := range v {
			f = append(f, math.Log(x+1e-6))
		}
	}
	lg(p)
	lg(knn)
	lg(conf)
	lg(file)
	s := 0.0
	if seen {
		s = 1
	}
	f = append(f, s, math.Log1p(float64(size))/8, maxSim)
	return f
}
