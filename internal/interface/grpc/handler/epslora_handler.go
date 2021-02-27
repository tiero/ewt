package handler

import (
	"context"

	pb "github.com/tiero/ewt/api-spec/protobuf/gen/go/esplora"
)

type esploraHandler struct {
	pb.UnimplementedEsploraServer
}

// NewEsploraHandler returns a pb.EsploraServer
func NewEsploraHandler() pb.EsploraServer {
	return &esploraHandler{}
}

func (e esploraHandler) Tx(ctx context.Context, req *pb.TxRequest) (*pb.TxReply, error) {
	return &pb.TxReply{}, nil
}
