package tests

import (
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSome(_ *testing.T) {
	m := metainfo.Meta{FileName: "some file name))"}
	m.Write("abc")
	bytes, _ := json.Marshal(m)
	fmt.Println(string(bytes))
}
