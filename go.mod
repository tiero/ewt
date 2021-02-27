module github.com/tiero/ewt

go 1.15

require (
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/btcsuite/btcwallet v0.11.0
	github.com/golang/protobuf v1.4.3
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.3.0
	github.com/lightninglabs/gozmq v0.0.0-20191113021534-d20a764486bf
	github.com/sirupsen/logrus v1.8.0
	github.com/vulpemventures/go-elements v0.1.1
	github.com/ybbus/jsonrpc/v2 v2.1.6
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de // indirect
	google.golang.org/genproto v0.0.0-20210224155714-063164c882e6
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.25.1-0.20201208041424-160c7477e0e8
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/tiero/ewt/pkg/zmq => ./pkg/mux

replace github.com/tiero/ewt/api-spec/protobuf/gen/go/esplora => ./api-spec/protobuf/gen/go/esplora
