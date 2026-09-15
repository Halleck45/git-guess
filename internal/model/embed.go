package model

import (
	_ "embed"
	"sync"
)

//go:embed model.bin
var embedded []byte

var (
	defaultOnce sync.Once
	defaultM    *Model
	defaultErr  error
)

// Default returns the embedded model.
func Default() (*Model, error) {
	defaultOnce.Do(func() { defaultM, defaultErr = Load(embedded) })
	return defaultM, defaultErr
}
