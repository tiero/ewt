package ewtgrpc

import (
	"context"
	"net"

	"google.golang.org/grpc"

	pb "github.com/tiero/ewt/api-spec/protobuf/gen/go/esplora"
	"github.com/tiero/ewt/internal/interface/grpc/handler"
)

// Server defines the gRPC server interface
type Server interface {
	Start() error
}

type server struct {
	address string
}

// NewServer returns a gRPC Server
func NewServer(address string) (Server, error) {
	return &server{
		address: address,
	}, nil
}

//Start runs the gRPC server
func (s *server) Start() error {
	go s.serveMux()

	return nil
}

func (s *server) serveMux() error {

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	esploraHandler := handler.NewEsploraHandler()

	grpcServer := grpc.NewServer()

	pb.RegisterEsploraServer(
		grpcServer,
		esploraHandler,
	)

	lis, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}

	return grpcServer.Serve(lis)
}
