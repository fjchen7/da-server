package main

import (
	"DA-server/celestia"
	"DA-server/eth"
	"DA-server/zklinknova"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	celestisClient, err := celestia.NewClientFromEnv(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when creating celestia client: %v\n", err)
		os.Exit(1)
	}
	zklinkClient, err := zklinknova.NewClientFromEnv(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when creating zklink nova client: %v\n", err)
		os.Exit(1)
	}
	ethClient, err := eth.NewClientFromEnv(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when creating ethereum client: %v\n", err)
		os.Exit(1)
	}

	intervalInMillisecond, err := strconv.ParseInt(os.Getenv("POLLING_BATCH_INTERNAL_IN_MILLISECOND"), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when reading environment variable POLLING_BATCH_INTERNAL_IN_MILLISECOND: %v\n", err)
		os.Exit(1)
	}

	batches, err := zklinkClient.Poll(context.Background(), intervalInMillisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when zklink client polls batches: %v\n", err)
		os.Exit(1)
	}
	daProofs, err := celestisClient.Subscribe(context.Background(), batches)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when celestia client subscribe batches: %v\n", err)
		os.Exit(1)
	}

	err = ethClient.Subscribe(context.Background(), daProofs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred when ethereum client subscribe DA Proofs: %v\n", err)
		os.Exit(1)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signalChan:
		os.Exit(0)
	}
}
