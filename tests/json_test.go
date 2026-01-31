package tests

import (
	"encoding/json"
	"testing"

	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
)

func TestSome(t *testing.T) {
	t.Parallel()

	m := metainfo.Meta{FileName: "some file name))"}

	if err := m.Write(t.TempDir() + "/abc"); err != nil {
		t.Fatal(err)
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(string(bytes))
}
