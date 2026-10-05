// Package random provides explicitly named, reproducible PCG streams.
package random

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

type Source struct {
	pcg *rand.PCG
	rng *rand.Rand
}

// New derives a stream independently of the number of calls to any other stream.
// A Source belongs to one worker; it must not be shared across goroutines.
func New(seed uint64, label string, index uint64) *Source {
	var header [16]byte
	binary.LittleEndian.PutUint64(header[:8], seed)
	binary.LittleEndian.PutUint64(header[8:], index)
	h := sha256.New()
	h.Write(header[:])
	h.Write([]byte(label))
	digest := h.Sum(nil)
	pcg := rand.NewPCG(binary.LittleEndian.Uint64(digest[:8]), binary.LittleEndian.Uint64(digest[8:16]))
	return &Source{pcg: pcg, rng: rand.New(pcg)}
}

func (s *Source) Uint64() uint64       { return s.rng.Uint64() }
func (s *Source) NormFloat64() float64 { return s.rng.NormFloat64() }

func (s *Source) IntN(n int) int                    { return s.rng.IntN(n) }
func (s *Source) MarshalBinary() ([]byte, error)    { return s.pcg.MarshalBinary() }
func (s *Source) UnmarshalBinary(data []byte) error { return s.pcg.UnmarshalBinary(data) }
