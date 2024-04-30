package main

import (
	"context"
	"da-server/celestia"
	"da-server/db"
	"da-server/eth"
	"da-server/zklinknova"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	ethereumRpcEndPoint = "evm_rpc_endpoint"
)

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loadEnv()

	dbClient, err := db.NewConnectorFromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("error init DB client")
	}
	defer dbClient.Close()

	ethClientInstance, err := ethclient.Dial(ethereumRpcEndPoint)
	if err != nil {
		log.Fatal().Err(err).Msg("error init Ethereum RPC client")
	}
	defer ethClientInstance.Close()

	zklinkClient, err := zklinknova.NewClientFromEnv(ctx, dbClient)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating ZkLink Nova RPC client")
	}
	celestisClient, err := celestia.NewClientFromEnv(ctx, dbClient)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating Celestia RPC client")
	}
	ethClient, err := eth.NewClientFromEnv(dbClient, ethClientInstance)
	if err != nil {
		log.Fatal().Err(err).Msg("error creating Ethereum RPC client")
	}

	intervalInMillisecond, err := strconv.ParseInt(os.Getenv("POLLING_INTERNAL_IN_MILLISECOND"), 10, 64)
	if err != nil {
		log.Fatal().Err(err).
			Str("POLLING_INTERNAL_IN_MILLISECOND", os.Getenv("POLLING_INTERNAL_IN_MILLISECOND")).
			Msg("error parsing environment variable POLLING_INTERNAL_IN_MILLISECOND to number")
	}
	log.Printf("interval: %dms\n", intervalInMillisecond)

	interval := time.Duration(intervalInMillisecond) * time.Millisecond
	zklinkClient.Run(ctx, interval)
	celestisClient.Run(ctx, interval)
	ethClient.RunSubmit(ctx, interval)
	ethClient.RunMonitorTx(ctx, interval)

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
		log.Fatal().Msg("error loading .env file")
	}
}
