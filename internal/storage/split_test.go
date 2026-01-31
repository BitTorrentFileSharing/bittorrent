package storage_test

import (
	"crypto/sha1"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

func TestSplitAndHash(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "sample.txt")

	if err := os.WriteFile(path, []byte(strings.Repeat("A", 1000)), 0o600); err != nil {
		t.Fatal(err)
	}

	pieces, hashes, err := storage.Split(path, 256)
	if err != nil {
		t.Fatal(err)
	}

	if len(pieces) != 4 {
		t.Fatalf("Wanted 4 pieces, got %d", len(pieces))
	}

	if len(hashes[0]) != sha1.Size {
		t.Fatalf("bad hash len")
	}
}
