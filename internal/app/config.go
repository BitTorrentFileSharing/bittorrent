// CLI Flags parsed here

package app

import "flag"

type Config struct {
	SeedPath       string
	MetaPath       string
	Listen         string
	DestDir        string
	DHTListen      string
	PeersCSV       string
	BootstrapCSV   string
	KeepSeedingSec int
}

func ParseFlags() *Config {
	var c Config
	flag.StringVar(&c.SeedPath, "seed", "", "path to payload to seed")
	flag.StringVar(&c.MetaPath, "get", "", "path to .bit file to download")
	flag.StringVar(&c.PeersCSV, "peer", "", "comma-separated peers")
	flag.StringVar(&c.Listen, "addr", ":6881", "listen addr (seeder)")
	flag.StringVar(&c.DestDir, "dest", ".", "download output dir")
	flag.StringVar(&c.DHTListen, "dht-listen", ":0", "UDP addr for DHT ('' to disable)")
	flag.StringVar(&c.BootstrapCSV, "bootstrap", "", "comma-separated UDP bootstrap nodes")
	flag.IntVar(&c.KeepSeedingSec, "keep", 0, "seconds to keep seeding after complete")
	flag.Parse()
	return &c
}
