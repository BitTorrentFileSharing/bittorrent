package tests

import (
	"bittorrent/internal/metainfo"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSome(t *testing.T) {
	m := metainfo.Meta{FileName: "some file name))"}
	m.Write("abc")
	bytes, _ := json.Marshal(m)
	fmt.Println(string(bytes))
}
