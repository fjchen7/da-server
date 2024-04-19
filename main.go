package main

import (
	"context"
	"da-server/celestia"
	"da-server/db"
	"da-server/eth"
	"da-server/zklinknova"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loadEnv()

	dbClient, err := db.NewConnectorFromEnv()
	if err != nil {
		log.Fatalf("[Error] Encounter error when creating DB client: %v\n", err)
	}

	zklinkClient, err := zklinknova.NewClientFromEnv(ctx, dbClient)
	if err != nil {
		log.Fatalf("[Error] Encounter error when creating ZkLink nova client: %v\n", err)
	}
	celestisClient, err := celestia.NewClientFromEnv(ctx, dbClient)
	if err != nil {
		log.Fatalf("[Error] Encounter error when creating Celestia client: %v\n", err)
	}
	ethClient, err := eth.NewClientFromEnv(ctx, dbClient)
	if err != nil {
		log.Fatalf("[Error] Encounter error when creating Ethereum client: %v\n", err)
	}

	intervalInMillisecond, err := strconv.ParseInt(os.Getenv("POLLING_INTERNAL_IN_MILLISECOND"), 10, 64)
	if err != nil {
		log.Fatalf("[Error] Encounter error when reading environment variable POLLING_INTERNAL_IN_MILLISECOND: %v\n", err)
	}
	log.Printf("interval: %dms\n", intervalInMillisecond)

	interval := time.Duration(intervalInMillisecond) * time.Millisecond
	zklinkClient.Run(ctx, interval)
	celestisClient.Run(ctx, interval)
	ethClient.Run(ctx, interval)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signalChan:
		os.Exit(0)
	}
}

func loadEnv() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}
