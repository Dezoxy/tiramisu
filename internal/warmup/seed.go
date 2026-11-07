package warmup

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// SeedFromFile writes a file's head and tail windows from a local copy that the caller
// has proven identical to the torrent's file (Dezoxy fork): the library's own disks fill
// the index cache, so switching a title to streaming costs no swarm traffic. Index mode
// only. It returns the bytes written to each window.
func (d *DiskWarmupCache) SeedFromFile(hash string, fileID int, r io.ReaderAt, size int64) (head, tail int64, err error) {
	if d == nil || !persistTails {
		return 0, 0, errors.New("index cache not enabled (warmup_dir)")
	}
	if size <= 0 {
		return 0, 0, errors.New("empty file")
	}

	headLen := min(FileSize, size)
	if d.GetAvailableRange(hash, fileID) < headLen {
		// The writer returns the buffer to its pool, so it must come from there.
		bufPtr := warmupWritePool.Get().(*[]byte)
		if int64(len(*bufPtr)) < headLen {
			b := make([]byte, max(int64(warmupWriteBuf), headLen))
			bufPtr = &b
		}
		if _, err := r.ReadAt((*bufPtr)[:headLen], 0); err != nil {
			warmupWritePool.Put(bufPtr)
			return 0, 0, fmt.Errorf("read head: %w", err)
		}
		// Through the single writer, which keeps the head dense; waiting on the queue
		// rather than dropping the write as the streaming path may.
		d.writeCh <- warmupWrite{hash, fileID, bufPtr, int(headLen), 0}
		deadline := time.Now().Add(30 * time.Second)
		path := d.filePath(hash, fileID)
		for {
			d.sizeCache.Delete(path)
			d.missing.Delete(path)
			if d.GetAvailableRange(hash, fileID) >= headLen {
				break
			}
			if time.Now().After(deadline) {
				return 0, 0, errors.New("head write did not complete")
			}
			time.Sleep(20 * time.Millisecond)
		}
		head = headLen
	}

	if !d.TailReady(hash, fileID) {
		tailStart := max(size-TailWarmupSize, 0)
		buf := make([]byte, size-tailStart)
		if _, err := r.ReadAt(buf, tailStart); err != nil {
			return head, 0, fmt.Errorf("read tail: %w", err)
		}
		d.writeTail(hash, fileID, buf, tailStart, size)
		if !d.TailReady(hash, fileID) && size >= TailWarmupSize {
			return head, 0, errors.New("tail write did not complete")
		}
		tail = int64(len(buf))
	}
	return head, tail, nil
}
