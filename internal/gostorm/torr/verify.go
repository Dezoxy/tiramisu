package torr

import (
	"sort"

	"github.com/anacrolix/torrent"

	utils2 "tiramisu/internal/gostorm/utils"
)

// RangeVerified reports whether every piece covering [off, off+n) of file fileID has
// passed its checksum (Dezoxy fork). fileID is the 1-based id Status() gives the file,
// numbered over the files sorted by path.
func (t *Torrent) RangeVerified(fileID int, off, n int64) bool {
	if t == nil || t.Torrent == nil || t.Torrent.Info() == nil || n <= 0 || off < 0 {
		return false
	}
	files := append([]*torrent.File(nil), t.Torrent.Files()...)
	sort.Slice(files, func(i, j int) bool {
		return utils2.CompareStrings(files[i].Path(), files[j].Path())
	})
	if fileID < 1 || fileID > len(files) {
		return false
	}
	f := files[fileID-1]
	if off+n > f.Length() {
		return false
	}
	first, last, ok := pieceSpan(f.Offset()+off, n, t.Torrent.Info().PieceLength)
	if !ok {
		return false
	}
	for i := first; i <= last; i++ {
		if !t.Torrent.PieceState(i).Complete {
			return false
		}
	}
	return true
}

// pieceSpan returns the first and last piece covering n bytes at torrent offset off.
func pieceSpan(off, n, pieceLen int64) (first, last int, ok bool) {
	if pieceLen <= 0 || n <= 0 || off < 0 {
		return 0, 0, false
	}
	return int(off / pieceLen), int((off + n - 1) / pieceLen), true
}
