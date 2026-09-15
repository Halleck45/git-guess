// Package model loads the embedded classifier and scores feature vectors.
//
// Binary format (little endian), written by scripts/train.py:
//
//	magic "CCM1"
//	u32 feature_version, u32 hash_bits, u32 num_dense, u32 num_classes, u32 num_sparse, f32 scale
//	classes: (u8 len, bytes) × num_classes
//	bias:    f32 × C
//	dense:   f32 × (num_dense × C)         row-major [dense_index][class]
//	indices: u32 × num_sparse               sorted ascending hashed indices
//	sparse:  i16 × (num_sparse × C)         quantized weights, w = q × scale
//	prior:   f32 × C                        class distribution of the training set
//	temp:    f32                            softmax temperature
package model

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Halleck45/conventional/internal/features"
)

// Model is a linear multinomial classifier over the feature space.
type Model struct {
	Classes     []string
	Prior       []float32
	Temperature float32
	numDense    int
	hashBits    int
	bias        []float32
	dense       []float32 // numDense × C
	indices     []uint32
	sparse      []int16 // len(indices) × C
	scale       float32
}

// Candidate is one class with its probability.
type Candidate struct {
	Type string  `json:"type"`
	P    float64 `json:"p"`
}

// Contribution explains one feature's weight on the predicted class.
type Contribution struct {
	Feature string  `json:"feature"`
	Weight  float64 `json:"weight"`
}

// Load parses a model from bytes.
func Load(b []byte) (*Model, error) {
	if len(b) < 28 || string(b[:4]) != "CCM1" {
		return nil, errors.New("model: bad magic")
	}
	le := binary.LittleEndian
	fv := le.Uint32(b[4:])
	if fv != features.Version {
		return nil, fmt.Errorf("model: feature version %d, binary expects %d", fv, features.Version)
	}
	m := &Model{hashBits: int(le.Uint32(b[8:])), numDense: int(le.Uint32(b[12:]))}
	C := int(le.Uint32(b[16:]))
	nS := int(le.Uint32(b[20:]))
	m.scale = math.Float32frombits(le.Uint32(b[24:]))
	if m.hashBits != features.HashBits || m.numDense != features.NumDense {
		return nil, fmt.Errorf("model: feature layout mismatch (hash_bits=%d num_dense=%d)", m.hashBits, m.numDense)
	}
	off := 28
	for i := 0; i < C; i++ {
		if off >= len(b) {
			return nil, errors.New("model: truncated classes")
		}
		n := int(b[off])
		off++
		if off+n > len(b) {
			return nil, errors.New("model: truncated classes")
		}
		m.Classes = append(m.Classes, string(b[off:off+n]))
		off += n
	}
	need := 4*C + 4*m.numDense*C + 4*nS + 2*nS*C + 4*C + 4
	if off+need > len(b) {
		return nil, fmt.Errorf("model: truncated weights (need %d bytes, have %d)", need, len(b)-off)
	}
	m.bias = readF32(b[off:], C)
	off += 4 * C
	m.dense = readF32(b[off:], m.numDense*C)
	off += 4 * m.numDense * C
	m.indices = make([]uint32, nS)
	for i := range m.indices {
		m.indices[i] = le.Uint32(b[off+4*i:])
	}
	off += 4 * nS
	m.sparse = make([]int16, nS*C)
	for i := range m.sparse {
		m.sparse[i] = int16(le.Uint16(b[off+2*i:]))
	}
	off += 2 * nS * C
	m.Prior = readF32(b[off:], C)
	off += 4 * C
	m.Temperature = math.Float32frombits(le.Uint32(b[off:]))
	if m.Temperature <= 0 {
		m.Temperature = 1
	}
	return m, nil
}

func readF32(b []byte, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return out
}

// Logits returns raw class scores (before temperature).
func (m *Model) Logits(v *features.Vector) []float64 {
	C := len(m.Classes)
	z := make([]float64, C)
	for c := range z {
		z[c] = float64(m.bias[c])
	}
	for i, x := range v.Dense {
		if x == 0 {
			continue
		}
		row := m.dense[i*C : i*C+C]
		for c := range z {
			z[c] += float64(x) * float64(row[c])
		}
	}
	scale := float64(m.scale)
	for _, s := range v.Sparse {
		r := m.row(s.Index)
		if r < 0 {
			continue
		}
		row := m.sparse[r*C : r*C+C]
		x := float64(s.Value) * scale
		for c := range z {
			z[c] += x * float64(row[c])
		}
	}
	return z
}

func (m *Model) row(idx uint32) int {
	i := sort.Search(len(m.indices), func(i int) bool { return m.indices[i] >= idx })
	if i < len(m.indices) && m.indices[i] == idx {
		return i
	}
	return -1
}

// Probabilities applies temperature scaling and softmax.
func (m *Model) Probabilities(logits []float64) []float64 {
	p := make([]float64, len(logits))
	T := float64(m.Temperature)
	mx := math.Inf(-1)
	for _, z := range logits {
		if z/T > mx {
			mx = z / T
		}
	}
	var sum float64
	for i, z := range logits {
		p[i] = math.Exp(z/T - mx)
		sum += p[i]
	}
	for i := range p {
		p[i] /= sum
	}
	return p
}

// Predict returns candidates sorted by decreasing probability.
func (m *Model) Predict(v *features.Vector) []Candidate {
	return m.Rank(m.Probabilities(m.Logits(v)))
}

// Rank turns a probability vector into sorted candidates.
func (m *Model) Rank(p []float64) []Candidate {
	out := make([]Candidate, len(p))
	for i := range p {
		out[i] = Candidate{Type: m.Classes[i], P: p[i]}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].P > out[j].P })
	return out
}

// ClassIndex returns the index of a class name, or -1.
func (m *Model) ClassIndex(name string) int {
	for i, c := range m.Classes {
		if c == name {
			return i
		}
	}
	return -1
}

// Explain lists the features contributing most (positively and negatively)
// to the margin of class `cls` against the runner-up `other`. The vector
// must have been extracted with KeepNames.
func (m *Model) Explain(v *features.Vector, cls, other int, n int) (pos, neg []Contribution) {
	C := len(m.Classes)
	var all []Contribution
	for i, x := range v.Dense {
		if x == 0 {
			continue
		}
		w := float64(x) * float64(m.dense[i*C+cls]-m.dense[i*C+other])
		all = append(all, Contribution{Feature: features.DenseNames[i], Weight: w})
	}
	for i, s := range v.Sparse {
		r := m.row(s.Index)
		if r < 0 {
			continue
		}
		w := float64(s.Value) * float64(m.scale) * float64(m.sparse[r*C+cls]-m.sparse[r*C+other])
		name := ""
		if i < len(v.Names) {
			name = v.Names[i]
		}
		all = append(all, Contribution{Feature: name, Weight: w})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Weight > all[j].Weight })
	for _, c := range all {
		if c.Weight > 0 && len(pos) < n {
			pos = append(pos, c)
		}
	}
	for i := len(all) - 1; i >= 0 && len(neg) < n; i-- {
		if all[i].Weight < 0 {
			neg = append(neg, all[i])
		}
	}
	return pos, neg
}

// AdaptPrior re-weights probabilities with a repository-specific class
// distribution. counts are the observed types in the repository history.
// lambda in [0,1] controls the strength (0 = no adaptation).
func (m *Model) AdaptPrior(p []float64, counts map[string]int, lambda float64) []float64 {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 || lambda == 0 {
		return p
	}
	out := make([]float64, len(p))
	var sum float64
	for i, cls := range m.Classes {
		repo := (float64(counts[cls]) + 1) / (float64(total) + float64(len(m.Classes)))
		train := float64(m.Prior[i])
		if train <= 0 {
			train = 1e-4
		}
		out[i] = p[i] * math.Pow(repo/train, lambda)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}
