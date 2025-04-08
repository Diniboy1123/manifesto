package main

import (
	"flag"
	"log"

	"github.com/Diniboy1123/manifesto/config"
	"github.com/Diniboy1123/manifesto/internal/utils"
	"github.com/Diniboy1123/manifesto/server"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")

	flag.Parse()

	if err := config.LoadConfig(*configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	utils.CleanCacheDir()
	config.WatchConfig()
	server.Start()
}
