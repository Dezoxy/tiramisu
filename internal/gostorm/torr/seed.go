package torr

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
)

// ErrNotIdentical means a local copy is not the torrent's file: a different release.
var ErrNotIdentical = errors.New("local copy differs from the torrent's file")

// ProveLocalCopy checks that r holds exactly file fileID of this torrent (Dezoxy fork,
// index seeding from the library's own disks): the same size, and the first and last
// whole pieces inside the file match the torrent's SHA-1 hashes. Pieces that straddle a
// neighbouring file cannot be hashed from one file alone, so the proof is the whole
// pieces nearest the head and the tail; a different release matches none of them. It
// returns how many pieces it hashed.
func (t *Torrent) ProveLocalCopy(fileID int, r io.ReaderAt, size int64) (int, error) {
	tt, f, ok := t.fileByID(fileID)
	if !ok {
		return 0, fmt.Errorf("file %d not found in torrent", fileID)
	}
	info := tt.Info()
	if info.NumPieces() == 0 {
		return 0, errors.New("torrent has no v1 piece hashes")
	}
	return proveCopy(r, size, f.Offset(), f.Length(), info.PieceLength, func(i int) ([]byte, bool) {
		if i < 0 || i >= info.NumPieces() {
			return nil, false
		}
		h := info.Piece(i).Hash()
		return h[:], true
	})
}

// proveCopy hashes the first and last whole pieces of a file at fileOffset in the
// torrent and compares them with hashOf.
func proveCopy(r io.ReaderAt, size, fileOffset, fileLen, pieceLen int64, hashOf func(int) ([]byte, bool)) (int, error) {
	if pieceLen <= 0 {
		return 0, errors.New("invalid piece length")
	}
	if size != fileLen {
		return 0, fmt.Errorf("%w: size %d, torrent has %d", ErrNotIdentical, size, fileLen)
	}
	first := (fileOffset + pieceLen - 1) / pieceLen // first piece starting inside the file
	last := (fileOffset+fileLen)/pieceLen - 1       // last piece ending inside the file
	if first > last {
		return 0, errors.New("file holds no whole piece; it cannot be proven from one file")
	}
	pieces := []int64{first}
	if last != first {
		pieces = append(pieces, last)
	}
	buf := make([]byte, pieceLen)
	for _, p := range pieces {
		want, ok := hashOf(int(p))
		if !ok {
			return 0, fmt.Errorf("no hash for piece %d", p)
		}
		if _, err := r.ReadAt(buf, p*pieceLen-fileOffset); err != nil {
			return 0, fmt.Errorf("read piece %d: %w", p, err)
		}
		got := sha1.Sum(buf)
		if string(got[:]) != string(want) {
			return 0, fmt.Errorf("%w: piece %d", ErrNotIdentical, p)
		}
	}
	return len(pieces), nil
}
