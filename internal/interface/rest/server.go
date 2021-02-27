package ewtrest

import (
	"context"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	gw "github.com/tiero/ewt/api-spec/protobuf/gen/go/esplora"
)

// Server defines the REST server interface
type Server interface {
	Start() error
}

type server struct {
	address      string
	grpcEndpoint string
}

// NewServer returns a REST Server
func NewServer(address, grpcEndpoint string) (Server, error) {
	return &server{
		address:      address,
		grpcEndpoint: grpcEndpoint}, nil
}

//Start runs the REST server
func (s *server) Start() error {
	go s.serveGrpcGateway()
	return nil
}

func (s *server) serveGrpcGateway() error {

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register gRPC server endpoint
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithInsecure()}
	err := gw.RegisterEsploraHandlerFromEndpoint(ctx, mux, s.address, opts)
	if err != nil {
		return err
	}

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	return http.ListenAndServe(s.address, mux)
}
