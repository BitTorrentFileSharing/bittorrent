package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

func main() {
	in := flag.String("split", "", "Path to the file to split and seed")
	piece := flag.Int("piece", storage.DefaultPiece, "Piece size in bytes")
	flag.Parse()

	if *in == "" {
		log.Fatal("usage: bittorrent -split myfile.bin")
	}
	pieces, hashes, err := storage.Split(*in, *piece)
	if err != nil {
		log.Fatal(err)
	}

	meta := &metainfo.Meta{
		FileName:   filepath.Base(*in),
		FileLength: int64(len(pieces) * *piece), // close enough for now
		PieceSize:  *piece,
		Hashes:     hashes,
	}
	outPath := *in + ".bit" // I might implement torrent spec later
	if err := meta.Write(outPath); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("[+] wrote %d pieces -> %s\n", len(pieces), outPath)
}
