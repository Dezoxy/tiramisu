package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Dezoxy fork: the seed endpoint reads only under index_seed_root.
func TestPathUnder(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "movies", "a.mkv")
	os.MkdirAll(filepath.Dir(inside), 0o755)
	os.WriteFile(inside, []byte("x"), 0o644)
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("x"), 0o644)
	link := filepath.Join(root, "movies", "link.mkv")
	os.Symlink(outside, link)

	if _, ok := pathUnder(root, inside); !ok {
		t.Error("file under root refused")
	}
	for _, p := range []string{outside, link, filepath.Join(root, "..", filepath.Base(outside)), "movies/a.mkv", ""} {
		if _, ok := pathUnder(root, p); ok {
			t.Errorf("%q accepted", p)
		}
	}
	if _, ok := pathUnder("", inside); ok {
		t.Error("empty root accepted")
	}
}

func TestVirtualStub(t *testing.T) {
	if p, ok := virtualStub("/state/real", "tv/Show/x.mkv"); !ok || p != "/state/real/tv/Show/x.mkv" {
		t.Errorf("got %q %v", p, ok)
	}
	if p, ok := virtualStub("/state/real", "../../etc/passwd"); !ok || p != "/state/real/etc/passwd" {
		t.Errorf("climbed out: %q %v", p, ok)
	}
	if _, ok := virtualStub("/state/real", "/"); ok {
		t.Error("root accepted")
	}
}
