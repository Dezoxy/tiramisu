package main

// Dezoxy fork: POST /api/index/seed fills a streamed file's index windows from the
// library's own copy of it, once its piece hashes prove the copy identical.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tiramisu/internal/gostorm/torr"
	"tiramisu/internal/vfs"
	"tiramisu/internal/warmup"
)

type seedRequest struct {
	Virtual string `json:"virtual"` // path under the FUSE mount, e.g. tv/Show/Season.1/x.mkv
	Local   string `json:"local"`   // the library's copy, under index_seed_root
}

type seedResponse struct {
	Hash          string `json:"hash"`
	FileID        int    `json:"file_id"`
	PiecesChecked int    `json:"pieces_checked"`
	HeadBytes     int64  `json:"head_bytes"`
	TailBytes     int64  `json:"tail_bytes"`
}

// pathUnder resolves p and reports whether it stays inside root, after symlinks.
func pathUnder(root, p string) (string, bool) {
	if root == "" || p == "" || !filepath.IsAbs(p) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(p))
	if err != nil {
		return "", false
	}
	rootReal, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return real, true
}

// virtualStub maps a path under the FUSE mount to its stub under the source tree.
func virtualStub(source, virtual string) (string, bool) {
	rel := filepath.Clean("/" + virtual) // anchor, so ".." cannot climb out
	if rel == "/" {
		return "", false
	}
	return filepath.Join(source, rel), true
}

func handleIndexSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if warmup.DiskWarmup == nil || gc().WarmupDir == "" || gc().IndexSeedRoot == "" {
		http.Error(w, "index seeding is not enabled (warmup_dir, index_seed_root)", http.StatusNotFound)
		return
	}
	var req seedRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	local, ok := pathUnder(gc().IndexSeedRoot, req.Local)
	if !ok {
		http.Error(w, "local path must exist under index_seed_root", http.StatusForbidden)
		return
	}
	stub, ok := virtualStub(physicalSourcePath, req.Virtual)
	if !ok {
		http.Error(w, "bad virtual path", http.StatusBadRequest)
		return
	}
	meta, err := stubMeta(stub)
	if err != nil {
		http.Error(w, "no streamed file at that virtual path", http.StatusNotFound)
		return
	}
	hash, fileID := vfs.ExtractHashAndIndex(meta.URL)
	if hash == "" || fileID <= 0 {
		http.Error(w, "virtual file has no torrent identity", http.StatusUnprocessableEntity)
		return
	}

	resp, status, err := seedIndex(r.Context(), hash, fileID, local)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	logger.Printf("[IndexSeed] %s from %s: %d pieces proven, head %dKB, tail %dKB",
		filepath.Base(stub), local, resp.PiecesChecked, resp.HeadBytes>>10, resp.TailBytes>>10)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func seedIndex(ctx context.Context, hash string, fileID int, local string) (seedResponse, int, error) {
	resp := seedResponse{Hash: hash, FileID: fileID}
	f, err := os.Open(local)
	if err != nil {
		return resp, http.StatusNotFound, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return resp, http.StatusBadRequest, errors.New("local path is not a regular file")
	}

	// The proof needs only the metainfo, which the wake loads from the state DB.
	if nativeBridge == nil {
		return resp, http.StatusServiceUnavailable, errors.New("engine not ready")
	}
	wctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := nativeBridge.Wake(wctx, "magnet:?xt=urn:btih:"+hash, fileID); err != nil {
		return resp, http.StatusServiceUnavailable, fmt.Errorf("wake torrent: %w", err)
	}
	t := torr.PeekTorrent(hash)
	n, err := t.ProveLocalCopy(fileID, f, fi.Size())
	if err != nil {
		if errors.Is(err, torr.ErrNotIdentical) {
			return resp, http.StatusConflict, err
		}
		return resp, http.StatusUnprocessableEntity, err
	}
	resp.PiecesChecked = n
	resp.HeadBytes, resp.TailBytes, err = warmup.DiskWarmup.SeedFromFile(hash, fileID, f, fi.Size())
	if err != nil {
		return resp, http.StatusInternalServerError, err
	}
	return resp, http.StatusOK, nil
}
