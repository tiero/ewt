package zmq

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	log "github.com/sirupsen/logrus"
	"github.com/tiero/ewt/pkg/elements"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"

	"github.com/lightninglabs/gozmq"
	"github.com/ybbus/jsonrpc/v2"
)

const (
	// rawBlockZMQCommand is the command used to receive raw block
	// notifications from bitcoind through ZMQ.
	rawBlockZMQCommand = "rawblock"

	// rawTxZMQCommand is the command used to receive raw transaction
	// notifications from bitcoind through ZMQ.
	rawTxZMQCommand = "rawtx"

	// maxRawBlockSize is the maximum size in bytes for a raw block received
	// from bitcoind through ZMQ.
	maxRawBlockSize = 4e6

	// maxRawTxSize is the maximum size in bytes for a raw transaction
	// received from elementsd through ZMQ.
	maxRawTxSize = 4e6

	// seqNumLen is the length of the sequence number of a message sent from
	// elementsd through ZMQ.
	seqNumLen = 4
)

// Subscriber ...
type Subscriber struct {
	started int32 // To be used atomically.
	stopped int32 // To be used atomically.

	// rescanClientCounter is an atomic counter that assigns a unique ID to
	// each new bitcoind rescan client using the current bitcoind
	// connection.
	rescanClientCounter uint64

	// chain identifies the current network the bitcoind node is
	// running on.
	chain *network.Network

	// client is the RPC client to the elementsd node.
	client *jsonrpc.RPCClient

	// zmqBlockConn is the ZMQ connection we'll use to read raw block
	// events.
	zmqBlockConn *gozmq.Conn

	// zmqTxConn is the ZMQ connection we'll use to read raw transaction
	// events.
	zmqTxConn *gozmq.Conn

	// rescanClients is the set of active bitcoind rescan clients to which
	// ZMQ event notfications will be sent to.
	rescanClientsMtx sync.Mutex
	rescanClients    map[uint64]*Consumer

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewSubscriber ...
func NewSubscriber(chain *network.Network,
	jsonrpcEndpoint, zmqBlockHost, zmqTxHost string,
	zmqPollInterval time.Duration) (*Subscriber, error) {

	rpcClient := jsonrpc.NewClient(jsonrpcEndpoint)

	// Establish two different ZMQ connections to bitcoind to retrieve block
	// and transaction event notifications. We'll use two as a separation of
	// concern to ensure one type of event isn't dropped from the connection
	// queue due to another type of event filling it up.
	zmqBlockConn, err := gozmq.Subscribe(
		zmqBlockHost, []string{rawBlockZMQCommand}, zmqPollInterval,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to subscribe for zmq block "+
			"events: %v", err)
	}

	zmqTxConn, err := gozmq.Subscribe(
		zmqTxHost, []string{rawTxZMQCommand}, zmqPollInterval,
	)
	if err != nil {
		//zmqBlockConn.Close()
		return nil, fmt.Errorf("unable to subscribe for zmq tx "+
			"events: %v", err)
	}

	return &Subscriber{
		chain:         chain,
		client:        &rpcClient,
		zmqTxConn:     zmqTxConn,
		zmqBlockConn:  zmqBlockConn,
		rescanClients: make(map[uint64]*Consumer),
		quit:          make(chan struct{}),
	}, nil
}

// Start attempts to establish a RPC and ZMQ connection to a elements node. If
// successful, a goroutine is spawned to read events from the ZMQ connection.
// It's possible for this function to fail due to a limited number of connection
// attempts. This is done to prevent waiting forever on the connection to be
// established in the case that the node is down.
func (s *Subscriber) Start() error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return nil
	}

	s.wg.Add(2)
	go s.blockEventHandler()
	go s.txEventHandler()

	return nil
}

// Stop terminates the RPC and ZMQ connection to a elementsd node and removes any
// active rescan clients.
func (s *Subscriber) Stop() {
	if !atomic.CompareAndSwapInt32(&s.stopped, 0, 1) {
		return
	}

	/* 	for _, client := range c.rescanClients {
	   		client.Stop()
	   	}
	*/
	close(s.quit)

	s.zmqBlockConn.Close()
	s.zmqTxConn.Close()

	s.wg.Wait()
}

// blockEventHandler reads raw blocks events from the ZMQ block socket and
// forwards them along to the current rescan clients.
//
// NOTE: This must be run as a goroutine.
func (s *Subscriber) blockEventHandler() {
	defer s.wg.Done()

	log.Info("Started listening for elementsd block notifications via ZMQ "+
		"on: ", s.zmqBlockConn.RemoteAddr())

	// Set up the buffers we expect our messages to consume. ZMQ
	// messages from bitcoind include three parts: the command, the
	// data, and the sequence number.
	//
	// We'll allocate a fixed data slice that we'll reuse when reading
	// blocks from bitcoind through ZMQ. There's no need to recycle this
	// slice (zero out) after using it, as further reads will overwrite the
	// slice and we'll only be deserializing the bytes needed.
	var (
		command [len(rawBlockZMQCommand)]byte
		seqNum  [seqNumLen]byte
		data    = make([]byte, maxRawBlockSize)
	)

	for {
		// Before attempting to read from the ZMQ socket, we'll make
		// sure to check if we've been requested to shut down.
		select {
		case <-s.quit:
			return
		default:
		}

		// Poll an event from the ZMQ socket.
		var (
			bufs = [][]byte{command[:], data, seqNum[:]}
			err  error
		)
		bufs, err = s.zmqBlockConn.Receive(bufs)
		if err != nil {
			// EOF should only be returned if the connection was
			// explicitly closed, so we can exit at this point.
			if err == io.EOF {
				return
			}

			// It's possible that the connection to the socket
			// continuously times out, so we'll prevent logging this
			// error to prevent spamming the logs.
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				log.Trace("Re-establishing timed out ZMQ " +
					"block connection")
				continue
			}

			log.Errorf("Unable to receive ZMQ %v message: %v",
				rawBlockZMQCommand, err)
			continue
		}

		// We have an event! We'll now ensure it is a block event,
		// deserialize it, and report it to the different rescan
		// clients.
		eventType := string(bufs[0])
		switch eventType {
		case rawBlockZMQCommand:

			// TODO deserialize the elements block in a proper way
			block := &wire.MsgBlock{}
			r := bytes.NewReader(bufs[1])
			if err := block.Deserialize(r); err != nil {
				log.Errorf("Unable to deserialize block: %v",
					err)
				continue
			}

			s.rescanClientsMtx.Lock()
			for _, client := range s.rescanClients {
				select {
				case client.zmqBlockNtfns <- &elements.Block{}:
				case <-client.quit:
				case <-s.quit:
					s.rescanClientsMtx.Unlock()
					return
				}
			}
			s.rescanClientsMtx.Unlock()
		default:
			// It's possible that the message wasn't fully read if
			// bitcoind shuts down, which will produce an unreadable
			// event type. To prevent from logging it, we'll make
			// sure it conforms to the ASCII standard.
			if eventType == "" || !isASCII(eventType) {
				continue
			}

			log.Warnf("Received unexpected event type from %v "+
				"subscription: %v", rawBlockZMQCommand,
				eventType)
		}
	}
}

func (s *Subscriber) txEventHandler() {

	log.Info("Started listening for elementsd transaction notifications "+
		"via ZMQ on: ", s.zmqTxConn.RemoteAddr())

	// Set up the buffers we expect our messages to consume. ZMQ
	// messages from bitcoind include three parts: the command, the
	// data, and the sequence number.
	//
	// We'll allocate a fixed data slice that we'll reuse when reading
	// transactions from bitcoind through ZMQ. There's no need to recycle
	// this slice (zero out) after using it, as further reads will overwrite
	// the slice and we'll only be deserializing the bytes needed.
	var (
		command [len(rawTxZMQCommand)]byte
		seqNum  [seqNumLen]byte
		data    = make([]byte, maxRawTxSize)
	)

	for {
		// Poll an event from the ZMQ socket.
		var (
			bufs = [][]byte{command[:], data, seqNum[:]}
			err  error
		)
		bufs, err = s.zmqTxConn.Receive(bufs)
		if err != nil {
			// EOF should only be returned if the connection was
			// explicitly closed, so we can exit at this point.
			if err == io.EOF {
				return
			}

			// It's possible that the connection to the socket
			// continuously times out, so we'll prevent logging this
			// error to prevent spamming the logs.
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				log.Trace("Re-establishing timed out ZMQ " +
					"transaction connection")
				continue
			}

			log.Errorf("Unable to receive ZMQ %v message: %v",
				rawTxZMQCommand, err)
			continue
		}

		// We have an event! We'll now ensure it is a transaction event,
		// deserialize it, and report it to the different rescan
		// clients.
		eventType := string(bufs[0])
		switch eventType {
		case rawTxZMQCommand:
			var tx *transaction.Transaction
			bytesBuffer := bytes.NewBuffer(bufs[1])
			if tx, err = transaction.NewTxFromBuffer(bytesBuffer); err != nil {
				log.Errorf("Unable to deserialize "+
					"transaction: %v", err)
				continue
			}

			s.rescanClientsMtx.Lock()
			for _, client := range s.rescanClients {
				select {
				case client.zmqTxNtfns <- tx:
				case <-client.quit:
				case <-s.quit:
					s.rescanClientsMtx.Unlock()
					return
				}
			}
			s.rescanClientsMtx.Unlock()

		default:
			// It's possible that the message wasn't fully read if
			// bitcoind shuts down, which will produce an unreadable
			// event type. To prevent from logging it, we'll make
			// sure it conforms to the ASCII standard.
			if eventType == "" || !isASCII(eventType) {
				continue
			}

			log.Warnf("Received unexpected event type from %v "+
				"subscription: %v", rawTxZMQCommand, eventType)
		}
	}
}

// NewConsumer returns a bitcoind client using the current bitcoind
// connection. This allows us to share the same connection using multiple
// clients.
func (s *Subscriber) NewConsumer() *Consumer {
	return &Consumer{
		quit: make(chan struct{}),

		id: atomic.AddUint64(&s.rescanClientCounter, 1),

		chain:     s.chain,
		chainConn: s,

		rescanUpdate:     make(chan interface{}),
		watchedScripts:   make(map[string]struct{}),
		watchedOutPoints: make(map[elements.OutPoint]struct{}),
		watchedTxs:       make(map[chainhash.Hash]struct{}),

		notificationQueue: chain.NewConcurrentQueue(20),
		zmqTxNtfns:        make(chan *transaction.Transaction),
		zmqBlockNtfns:     make(chan *elements.Block),

		mempool:        make(map[chainhash.Hash]struct{}),
		expiredMempool: make(map[int32]map[chainhash.Hash]struct{}),
	}
}

// AddClient adds a client to the set of active rescan clients of the current
// chain connection. This allows the connection to include the specified client
// in its notification delivery.
//
// NOTE: This function is safe for concurrent access.
func (s *Subscriber) AddClient(client *Consumer) {
	s.rescanClientsMtx.Lock()
	defer s.rescanClientsMtx.Unlock()

	s.rescanClients[client.id] = client
}

// RemoveClient removes the client with the given ID from the set of active
// rescan clients. Once removed, the client will no longer receive block and
// transaction notifications from the chain connection.
//
// NOTE: This function is safe for concurrent access.
func (s *Subscriber) RemoveClient(id uint64) {
	s.rescanClientsMtx.Lock()
	defer s.rescanClientsMtx.Unlock()

	delete(s.rescanClients, id)
}

// isASCII is a helper method that checks whether all bytes in `data` would be
// printable ASCII characters if interpreted as a string.
func isASCII(s string) bool {
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
