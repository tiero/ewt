package main

import (
	"fmt"
	"time"

	"github.com/tiero/ewt/pkg/zmq"
	"github.com/vulpemventures/go-elements/network"

	log "github.com/sirupsen/logrus"
)

func main() {
	fmt.Println("Hello from ewt daemon")

	sub, err := zmq.NewSubscriber(&network.Regtest, "http://admin1:123@localhost:7041", ":28332", ":28333", time.Millisecond)
	if err != nil {
		log.Error("creating subscriber: ", err)
	}

	err = sub.Start()
	if err != nil {
		log.Error("starting reading events: ", err)
	}

	consumer := sub.NewConsumer()
	err = consumer.Start()
	if err != nil {
		log.Error("starting consuming events: ", err)
	}

	err = consumer.NotifyReceived([]string{"0014619ecfb33710cb9bc1d93bdf4cdd8330ef40cd18"})
	if err != nil {
		log.Error("listening notifications: ", err)
	}

	for {
		c := <-consumer.Notifications()
		tx, ok := c.(zmq.RelevantTx)
		if !ok {
			continue
		}
		log.Infof("Received new tx with id %s at time %s", tx.TxRecord.Tx.TxHash().String(), tx.TxRecord.Received.String())
	}

}
