package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	grpc "github.com/tiero/ewt/internal/interface/grpc"
	rest "github.com/tiero/ewt/internal/interface/rest"
)

func main() {

	log.Info("starting esplora...")

	grpcAddress := fmt.Sprintf(
		":%+v",
		8080,
	)
	restAddress := fmt.Sprintf(
		":%+v",
		8081,
	)

	// gRPC
	esploraGrpcServer, err := grpc.NewServer(grpcAddress)
	if err != nil {
		log.WithError(err).Fatal()
	}

	// REST via gRPC gateway
	esploraRestServer, err := rest.NewServer(restAddress, grpcAddress)
	if err != nil {
		log.WithError(err).Fatal()
	}

	log.Info("esplora gRPC server listening on port " + grpcAddress)
	if err := esploraGrpcServer.Start(); err != nil {
		log.WithError(err).Fatal()
	}

	log.Info("esplora REST server listening on port " + restAddress)
	if err := esploraRestServer.Start(); err != nil {
		log.WithError(err).Fatal()
	}

	// Gracefully handle SIGTERM & SIGINT signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	log.Info("shutting down esplora...")
}
