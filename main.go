package main

import (
	"da-server/celestia"
	"da-server/eth"
	"da-server/zklinknova"
	"context"
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

	celestisClient, err := celestia.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatalf("An error occurred when creating celestia client: %v\n", err)
	}
	zklinkClient, err := zklinknova.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatalf("An error occurred when creating zklink nova client: %v\n", err)
	}
	ethClient, err := eth.NewClientFromEnv(ctx)
	if err != nil {
		log.Fatalf("An error occurred when creating ethereum client: %v\n", err)
	}

	intervalInMillisecond, err := strconv.ParseInt(os.Getenv("POLLING_BATCH_INTERNAL_IN_MILLISECOND"), 10, 64)
	if err != nil {
		log.Fatalf("An error occurred when reading environment variable POLLING_BATCH_INTERNAL_IN_MILLISECOND: %v\n", err)
	}

	batches, err := zklinkClient.Poll(ctx, intervalInMillisecond)
	if err != nil {
		log.Fatalf("An error occurred when zklink client polls batches: %v\n", err)
	}
	daProofs, err := celestisClient.Subscribe(ctx, batches)
	if err != nil {
		log.Fatalf("An error occurred when celestia client subscribe batches: %v\n", err)
	}

	err = ethClient.Subscribe(ctx, daProofs)
	if err != nil {
		log.Fatalf("An error occurred when ethereum client subscribe DA Proofs: %v\n", err)
	}

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
