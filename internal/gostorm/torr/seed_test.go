package torr

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"testing"
)

// Dezoxy fork: a local copy is proven by the whole pieces nearest its ends.
func TestProveCopy(t *testing.T) {
	const pl = 16
	// A torrent of three files: 10 bytes, our 70-byte file at offset 10, 20 bytes.
	torrent := make([]byte, 100)
	for i := range torrent {
		torrent[i] = byte(i * 7)
	}
	hashes := map[int][]byte{}
	for p := 0; p*pl < len(torrent); p++ {
		end := min((p+1)*pl, len(torrent))
		h := sha1.Sum(torrent[p*pl : end])
		hashes[p] = h[:]
	}
	hashOf := func(i int) ([]byte, bool) { h, ok := hashes[i]; return h, ok }
	file := torrent[10:80]

	n, err := proveCopy(bytes.NewReader(file), 70, 10, 70, pl, hashOf)
	if err != nil || n != 2 {
		t.Fatalf("identical copy: n=%d err=%v", n, err) // pieces 1 and 4 are whole inside it
	}

	other := append([]byte(nil), file...)
	other[60] ^= 0xff // torrent byte 70: inside piece 4, the last whole one
	if _, err := proveCopy(bytes.NewReader(other), 70, 10, 70, pl, hashOf); !errors.Is(err, ErrNotIdentical) {
		t.Fatalf("different release accepted: %v", err)
	}
	if _, err := proveCopy(bytes.NewReader(file[:69]), 69, 10, 70, pl, hashOf); !errors.Is(err, ErrNotIdentical) {
		t.Fatalf("size mismatch accepted: %v", err)
	}
	// A file inside a single piece cannot be proven on its own.
	if _, err := proveCopy(bytes.NewReader(torrent[2:9]), 7, 2, 7, pl, hashOf); err == nil || errors.Is(err, ErrNotIdentical) {
		t.Fatalf("unprovable file: %v", err)
	}
}
