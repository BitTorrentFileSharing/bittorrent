// CLI Flags parsed here

package app

import "flag"

type Config struct {
	SeedPath       string
	MetaPath       string
	Listen         string
	PeersCSV       string
	DestDir        string
	KeepSeedingSec int
}

func ParseFlags() *Config {
	var c Config
	flag.StringVar(&c.SeedPath, "seed", "", "path to payload to seed")
	flag.StringVar(&c.MetaPath, "get", "", "path to .bit file to download")
	flag.StringVar(&c.PeersCSV, "peer", "", "comma‑separated peers")
	flag.StringVar(&c.Listen, "addr", ":6881", "listen addr (seeder)")
	flag.StringVar(&c.DestDir, "dest", ".", "download output dir")
	flag.IntVar(&c.KeepSeedingSec, "keep", 0, "seconds to keep seeding after complete")
	flag.Parse()
	return &c
}
