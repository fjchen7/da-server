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
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loadEnv()

	dbClient, err := db.NewConnectorFromEnv()
	if err != nil {
		log.Fatalf("An error occurred when creating db client: %v\n", err)
	}

	celestisClient, err := celestia.NewClientFromEnv1(ctx, dbClient)
	if err != nil {
		log.Fatalf("An error occurred when creating celestia client: %v\n", err)
	}
	zklinkClient, err := zklinknova.NewClientFromEnv1(ctx, dbClient)
	if err != nil {
		log.Fatalf("An error occurred when creating zklink nova client: %v\n", err)
	}
	ethClient, err := eth.NewClientFromEnv1(ctx, dbClient)
	if err != nil {
		log.Fatalf("An error occurred when creating ethereum client: %v\n", err)
	}

	intervalInMillisecond, err := strconv.ParseInt(os.Getenv("POLLING_INTERNAL_IN_MILLISECOND"), 10, 64)
	if err != nil {
		log.Fatalf("An error occurred when reading environment variable POLLING_BATCH_INTERNAL_IN_MILLISECOND: %v\n", err)
	}

	zklinkClient.Run(ctx, intervalInMillisecond)
	celestisClient.Run(ctx, intervalInMillisecond)
	ethClient.Run(ctx, intervalInMillisecond)

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
