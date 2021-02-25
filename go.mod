module github.com/tiero/ewt

go 1.15

require (
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/btcsuite/btcwallet v0.11.0
	github.com/lightninglabs/gozmq v0.0.0-20191113021534-d20a764486bf
	github.com/sirupsen/logrus v1.8.0
	github.com/vulpemventures/go-elements v0.1.1
	github.com/ybbus/jsonrpc/v2 v2.1.6
)

replace github.com/tiero/ewt/pkg/zmq => ./pkg/mux
