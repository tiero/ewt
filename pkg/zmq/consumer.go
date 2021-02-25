package zmq

import (
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcwallet/chain"
	log "github.com/sirupsen/logrus"
	"github.com/tiero/ewt/pkg/elements"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/transaction"
)

// Consumer ...
type Consumer struct {
	// notifyBlocks signals whether the client is sending block
	// notifications to the caller. This must be used atomically.
	notifyBlocks uint32

	started int32 // To be used atomically.
	stopped int32 // To be used atomically.

	// birthday is the earliest time for which we should begin scanning the
	// chain.
	birthday time.Time

	// chain identifies the current network the bitcoind node is
	// running on.
	chain *network.Network

	// id is the unique ID of this client assigned by the backing bitcoind
	// connection.
	id uint64

	// chainConn is the backing client to our rescan client that contains
	// the RPC and ZMQ connections to a bitcoind node.
	chainConn *Subscriber

	// bestBlock keeps track of the tip of the current best chain.
	bestBlockMtx sync.RWMutex
	bestBlock    elements.BlockID

	// rescanUpdate is a channel will be sent items that we should match
	// transactions against while processing a chain rescan to determine if
	// they are relevant to the client.
	rescanUpdate chan interface{}

	// watchedScripts, watchedOutPoints, and watchedTxs are the set of
	// items we should match transactions against while processing a chain
	// rescan to determine if they are relevant to the client.
	watchMtx         sync.RWMutex
	watchedScripts   map[string]struct{}
	watchedOutPoints map[elements.OutPoint]struct{}
	watchedTxs       map[chainhash.Hash]struct{}

	// mempool keeps track of all relevant transactions that have yet to be
	// confirmed. This is used to shortcut the filtering process of a
	// transaction when a new confirmed transaction notification is
	// received.
	//
	// NOTE: This requires the watchMtx to be held.
	mempool map[chainhash.Hash]struct{}

	// expiredMempool keeps track of a set of confirmed transactions along
	// with the height at which they were included in a block. These
	// transactions will then be removed from the mempool after a period of
	// 288 blocks. This is done to ensure the transactions are safe from a
	// reorg in the chain.
	//
	// NOTE: This requires the watchMtx to be held.
	expiredMempool map[int32]map[chainhash.Hash]struct{}

	// notificationQueue is a concurrent unbounded queue that handles
	// dispatching notifications to the subscriber of this client.
	//
	// TODO: Rather than leaving this as an unbounded queue for all types of
	// notifications, try dropping ones where a later enqueued notification
	// can fully invalidate one waiting to be processed. For example,
	// BlockConnected notifications for greater block heights can remove the
	// need to process earlier notifications still waiting to be processed.
	notificationQueue *chain.ConcurrentQueue

	// zmqTxNtfns is a channel through which ZMQ transaction events will be
	// retrieved from the backing elements connection.
	zmqTxNtfns chan *transaction.Transaction

	// zmqBlockNtfns is a channel through which ZMQ block events will be
	// retrieved from the backing elements connection.
	zmqBlockNtfns chan *elements.Block

	quit chan struct{}
	wg   sync.WaitGroup
}

func (c *Consumer) Id() uint64 {
	return c.id
}

// Start initializes the bitcoind rescan client using the backing bitcoind
// connection and starts all goroutines necessary in order to process rescans
// and ZMQ notifications.
//
// NOTE: This is part of the chain.Interface interface.
func (c *Consumer) Start() error {
	if !atomic.CompareAndSwapInt32(&c.started, 0, 1) {
		return nil
	}

	// Start the notification queue and immediately dispatch a
	// ClientConnected notification to the caller. This is needed as some of
	// the callers will require this notification before proceeding.
	c.notificationQueue.Start()
	c.notificationQueue.ChanIn() <- ClientConnected{}

	// Retrieve the best block of the chain.
	/* 	bestHash, bestHeight, err := c.GetBestBlock()
	   	if err != nil {
	   		return fmt.Errorf("unable to retrieve best block: %v", err)
	   	}
	   	bestHeader, err := c.GetBlockHeaderVerbose(bestHash)
	   	if err != nil {
	   		return fmt.Errorf("unable to retrieve header for best block: "+
	   			"%v", err)
	   	}

	   	c.bestBlockMtx.Lock()
	   	c.bestBlock = waddrmgr.BlockStamp{
	   		Hash:      *bestHash,
	   		Height:    bestHeight,
	   		Timestamp: time.Unix(bestHeader.Time, 0),
	   	}
	   	c.bestBlockMtx.Unlock() */

	c.wg.Add(1)
	go c.rescanHandler()

	return nil
}

// Stop stops the bitcoind rescan client from processing rescans and ZMQ
// notifications.
//
// NOTE: This is part of the chain.Interface interface.
func (c *Consumer) Stop() {
	if !atomic.CompareAndSwapInt32(&c.stopped, 0, 1) {
		return
	}

	close(c.quit)

	// Remove this client's reference from the bitcoind connection to
	// prevent sending notifications to it after it's been stopped.
	c.chainConn.RemoveClient(c.id)

	c.notificationQueue.Stop()
}

// Notifications returns a channel to retrieve notifications from.
//
// NOTE: This is part of the chain.Interface interface.
func (c *Consumer) Notifications() <-chan interface{} {
	return c.notificationQueue.ChanOut()
}

// NotifyReceived allows the chain backend to notify the caller whenever a
// transaction pays to any of the given scripts hex.
//
// NOTE: This is part of the chain.Interface interface.
func (c *Consumer) NotifyReceived(scripts []string) error {
	c.NotifyBlocks()

	select {
	case c.rescanUpdate <- scripts:
	case <-c.quit:
		return elements.ErrClientShuttingDown
	}

	return nil
}

// NotifySpent allows the chain backend to notify the caller whenever a
// transaction spends any of the given outpoints.
func (c *Consumer) NotifySpent(outPoints []*elements.OutPoint) error {
	c.NotifyBlocks()

	select {
	case c.rescanUpdate <- outPoints:
	case <-c.quit:
		return elements.ErrClientShuttingDown
	}

	return nil
}

// NotifyTx allows the chain backend to notify the caller whenever any of the
// given transactions confirm within the chain.
func (c *Consumer) NotifyTx(txids []chainhash.Hash) error {
	c.NotifyBlocks()

	select {
	case c.rescanUpdate <- txids:
	case <-c.quit:
		return elements.ErrClientShuttingDown
	}

	return nil
}

// NotifyBlocks allows the chain backend to notify the caller whenever a block
// is connected or disconnected.
//
// NOTE: This is part of the chain.Interface interface.
func (c *Consumer) NotifyBlocks() error {
	// We'll guard the goroutine being spawned below by the notifyBlocks
	// variable we'll use atomically. We'll make sure to reset it in case of
	// a failure before spawning the goroutine so that it can be retried.
	if !atomic.CompareAndSwapUint32(&c.notifyBlocks, 0, 1) {
		return nil
	}

	// Re-evaluate our known best block since it's possible that blocks have
	// occurred between now and when the client was created. This ensures we
	// don't detect a new notified block as a potential reorg.

	/* 	bestHash, bestHeight, err := c.GetBestBlock()
	   	if err != nil {
	   		atomic.StoreUint32(&c.notifyBlocks, 0)
	   		return fmt.Errorf("unable to retrieve best block: %v", err)
	   	}
	   	bestHeader, err := c.GetBlockHeaderVerbose(bestHash)
	   	if err != nil {
	   		atomic.StoreUint32(&c.notifyBlocks, 0)
	   		return fmt.Errorf("unable to retrieve header for best block: "+
	   			"%v", err)
	   	}

	   	c.bestBlockMtx.Lock()
	   	c.bestBlock.Hash = *bestHash
	   	c.bestBlock.Height = bestHeight
	   	c.bestBlock.Timestamp = time.Unix(bestHeader.Time, 0)
	   	c.bestBlockMtx.Unlock() */

	// Include the client in the set of rescan clients of the backing
	// bitcoind connection in order to receive ZMQ event notifications for
	// new blocks and transactions.
	c.chainConn.AddClient(c)

	c.wg.Add(1)
	go c.ntfnHandler()

	return nil
}

// shouldNotifyBlocks determines whether the client should send block
// notifications to the caller.
func (c *Consumer) shouldNotifyBlocks() bool {
	return atomic.LoadUint32(&c.notifyBlocks) == 1
}

// onRelevantTx is a callback that's executed whenever a transaction is relevant
// to the caller. This means that the transaction matched a specific item in the
// client's different filters. This will queue a RelevantTx notification to the
// caller.
func (c *Consumer) onRelevantTx(tx *elements.TransactionExtended,
	blockDetails *btcjson.BlockDetails) {

	var blk *elements.BlockID
	if blockDetails != nil {
		blockHash, err := chainhash.NewHashFromStr(blockDetails.Hash)
		if err != nil {
			log.Errorf("Unable to send onRelevantTx notification, failed "+
				"parse block: %v", err)
			return
		}
		blk = &elements.BlockID{
			Height:    blockDetails.Height,
			Hash:      *blockHash,
			Timestamp: time.Unix(blockDetails.Time, 0),
		}
	}

	select {
	case c.notificationQueue.ChanIn() <- RelevantTx{
		TxRecord: tx,
		Block:    blk,
	}:
	case <-c.quit:
	}
}

// rescanHandler handles the logic needed for the caller to trigger a chain
// rescan.
//
// NOTE: This must be called as a goroutine.
func (c *Consumer) rescanHandler() {
	defer c.wg.Done()

	for {
		select {
		case update := <-c.rescanUpdate:
			switch update := update.(type) {

			// We're clearing the filters.
			case struct{}:
				c.watchMtx.Lock()
				c.watchedOutPoints = make(map[elements.OutPoint]struct{})
				c.watchedScripts = make(map[string]struct{})
				c.watchedTxs = make(map[chainhash.Hash]struct{})
				c.watchMtx.Unlock()

			// We're adding the addresses to our filter.
			case []string:
				c.watchMtx.Lock()
				for _, addr := range update {
					c.watchedScripts[addr] = struct{}{}
				}
				c.watchMtx.Unlock()

			default:
				log.Warnf("Received unexpected filter type %T",
					update)
			}
		case <-c.quit:
			return
		}
	}
}

// ntfnHandler handles the logic to retrieve ZMQ notifications from the backing
// bitcoind connection.
//
// NOTE: This must be called as a goroutine.
func (c *Consumer) ntfnHandler() {
	defer c.wg.Done()

	for {
		select {
		case tx := <-c.zmqTxNtfns:
			if _, _, err := c.filterTx(tx, nil, true); err != nil {
				log.Errorf("Unable to filter transaction %v: %v",
					tx.TxHash(), err)
			}
		case newBlock := <-c.zmqBlockNtfns:
			// If the new block's previous hash matches the best
			// hash known to us, then the new block is the next
			// successor, so we'll update our best block to reflect
			// this and determine if this new block matches any of
			// our existing filters.
			c.bestBlockMtx.RLock()
			bestBlock := c.bestBlock
			c.bestBlockMtx.RUnlock()
			if newBlock.Header.PrevBlock == bestBlock.Hash {
				newBlockHeight := bestBlock.Height + 1
				_ = c.filterBlock(newBlock, newBlockHeight, true)

				// With the block succesfully filtered, we'll
				// make it our new best block.
				bestBlock.Hash = newBlock.Header.BlockHash()
				bestBlock.Height = newBlockHeight
				bestBlock.Timestamp = newBlock.Header.Timestamp

				c.bestBlockMtx.Lock()
				c.bestBlock = bestBlock
				c.bestBlockMtx.Unlock()

				continue
			}

		case <-c.quit:
			return
		}
	}
}

// filterBlock filters a block for watched outpoints and addresses, and returns
// any matching transactions, sending notifications along the way.
func (c *Consumer) filterBlock(block *elements.Block, height int32, notify bool) []*elements.TransactionExtended {

	// If this block happened before the client's birthday or we have
	// nothing to filter for, then we'll skip it entirely.
	/* 	blockHash := block.BlockHash()
	   	if !c.shouldFilterBlock(block.Header.Timestamp) {
	   		if notify {
	   			c.onFilteredBlockConnected(height, &block.Header, nil)
	   			c.onBlockConnected(
	   				&blockHash, height, block.Header.Timestamp,
	   			)
	   		}
	   		return nil
	   	} */

	if c.shouldNotifyBlocks() {
		log.Debugf("Filtering block %d (%s) with %d transactions",
			height, block.Header.BlockHash(), len(block.Transactions))
	}

	// Create a block details template to use for all of the confirmed
	// transactions found within this block.
	blockDetails := &btcjson.BlockDetails{
		Hash:   block.Header.BlockHash().String(),
		Height: height,
		Time:   block.Header.Timestamp.Unix(),
	}

	// Now, we'll through all of the transactions in the block keeping track
	// of any relevant to the caller.
	var relevantTxs []*elements.TransactionExtended

	for i, tx := range block.Transactions {
		// Update the index in the block details with the index of this
		// transaction.
		blockDetails.Index = i
		isRelevant, rec, err := c.filterTx(tx, blockDetails, notify)
		if err != nil {
			log.Warnf("Unable to filter transaction %v: %v",
				tx.TxHash(), err)
			continue
		}

		if isRelevant {
			relevantTxs = append(relevantTxs, rec)
		}
	}

	// Update the expiration map by setting the block's confirmed
	// transactions and deleting any in the mempool that were confirmed
	// over 288 blocks ago.
	/* 	c.watchMtx.Lock()
	   	c.expiredMempool[height] = confirmedTxs
	   	if oldBlock, ok := c.expiredMempool[height-288]; ok {
	   		for txHash := range oldBlock {
	   			delete(c.mempool, txHash)
	   		}
	   		delete(c.expiredMempool, height-288)
	   	}
	   	c.watchMtx.Unlock() */

	/* if notify {
		c.onFilteredBlockConnected(height, &block.Header, relevantTxs)
		c.onBlockConnected(&blockHash, height, block.Header.Timestamp)
	} */

	return relevantTxs
}

// filterTx determines whether a transaction is relevant to the client by
// inspecting the client's different filters.
func (c *Consumer) filterTx(tx *transaction.Transaction,
	blockDetails *btcjson.BlockDetails,
	notify bool) (bool, *elements.TransactionExtended, error) {

	rec := &elements.TransactionExtended{
		Tx:       tx,
		Received: time.Now(),
	}
	if blockDetails != nil {
		rec.Received = time.Unix(blockDetails.Time, 0)
	}

	// We'll begin the filtering process by holding the lock to ensure we
	// match exactly against what's currently in the filters.
	c.watchMtx.Lock()
	defer c.watchMtx.Unlock()

	// If we've already seen this transaction and it's now been confirmed,
	// then we'll shortcut the filter process by immediately sending a
	// notification to the caller that the filter matches.
	if _, ok := c.mempool[tx.TxHash()]; ok {
		if notify && blockDetails != nil {
			c.onRelevantTx(rec, blockDetails)
		}
		return true, rec, nil
	}

	// Otherwise, this is a new transaction we have yet to see. We'll need
	// to determine if this transaction is somehow relevant to the caller.
	var isRelevant bool

	// We'll start by checking all inputs and determining whether it spends
	// an existing outpoint or a pkScript encoded as an address in our watch
	// list.
	for _, txIn := range tx.Inputs {
		// If it matches an outpoint in our watch list, we can exit our
		// loop early.

		/* 	if _, ok := c.watchedOutPoints[txIn.PreviousOutPoint]; ok {
			isRelevant = true
			break
		} */

		// Otherwise, we'll check whether it matches a pkScript in our
		// watch list encoded as an address. To do so, we'll re-derive
		// the pkScript of the output the input is attempting to spend.
		scriptHex := hex.EncodeToString(txIn.Script)

		if _, ok := c.watchedScripts[scriptHex]; ok {
			isRelevant = true
			break
		}
	}

	// We'll also cycle through its outputs to determine if it pays to
	// any of the currently watched scripts. If an output matches, we'll
	// add it to our watch list.
	for i, txOut := range tx.Outputs {

		scriptHex := hex.EncodeToString(txOut.Script)

		if _, ok := c.watchedScripts[scriptHex]; ok {
			isRelevant = true
			op := elements.OutPoint{
				Hash:  tx.TxHash(),
				Index: uint32(i),
			}
			c.watchedOutPoints[op] = struct{}{}
		}
	}

	// If the transaction didn't pay to any of our watched scripts, we'll
	// check if we're currently watching for the hash of this transaction.
	if !isRelevant {
		if _, ok := c.watchedTxs[tx.TxHash()]; ok {
			isRelevant = true
		}
	}

	// If the transaction is not relevant to us, we can simply exit.
	if !isRelevant {
		return false, rec, nil
	}

	// Otherwise, the transaction matched our filters, so we should dispatch
	// a notification for it. If it's still unconfirmed, we'll include it in
	// our mempool so that it can also be notified as part of
	// FilteredBlockConnected once it confirms.
	if blockDetails == nil {
		c.mempool[tx.TxHash()] = struct{}{}
	}

	c.onRelevantTx(rec, blockDetails)

	return true, rec, nil
}
