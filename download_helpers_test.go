package gocivitailib_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
)

// patternReader yields n bytes of a repeating pseudo-random block without
// ever holding more than the block in memory.
type patternReader struct {
	remaining int64
	pos       int
}

var patternBlock = func() []byte {
	b := make([]byte, 1<<20)
	x := uint32(2463534242)
	for i := range b {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b[i] = byte(x)
	}
	return b
}()

func newPattern(n int64) *patternReader { return &patternReader{remaining: n} }

func (p *patternReader) Read(b []byte) (int, error) {
	if p.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(b)) > p.remaining {
		b = b[:p.remaining]
	}
	n := copy(b, patternBlock[p.pos:])
	p.pos = (p.pos + n) % len(patternBlock)
	p.remaining -= int64(n)
	return n, nil
}

// patternSHA256 is the upper-case hex SHA256 of newPattern(n), as Civitai
// writes it.
func patternSHA256(t *testing.T, n int64) string {
	t.Helper()
	h := sha256.New()
	if _, err := io.Copy(h, newPattern(n)); err != nil {
		t.Fatal(err)
	}
	return upper(hex.EncodeToString(h.Sum(nil)))
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// countingWriter counts bytes and calls onWrite after every write.
type countingWriter struct {
	n       int64
	onWrite func(total int64)
}

func (w *countingWriter) Write(b []byte) (int, error) {
	w.n += int64(len(b))
	if w.onWrite != nil {
		w.onWrite(w.n)
	}
	return len(b), nil
}
