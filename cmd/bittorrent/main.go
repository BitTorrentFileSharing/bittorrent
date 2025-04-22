package main

import (
	"log"

	"github.com/BitTorrentFileSharing/bittorrent/internal/app"
)

func main() {
	cfg := app.ParseFlags()

	var err error
	switch {
	case cfg.SeedPath != "":
		err = app.RunSeeder(cfg)
	case cfg.MetaPath != "":
		err = app.RunLeecher(cfg)
	default:
		log.Fatalf("use -seed FILE or -get FILE")
	}
	if err != nil {
		log.Fatal(err)
	}
}
