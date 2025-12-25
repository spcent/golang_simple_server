package examples

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/spcent/golang_simple_server/pkg/pubsub"
)

type InboundConsumers struct {
	Pub pubsub.PubSub
}

func (c *InboundConsumers) Start(ctx context.Context) (stop func()) {
	// example: subscribe github push
	subGH, _ := c.Pub.Subscribe("in.github.push", pubsub.SubOptions{
		BufferSize: 64,
		Policy:     pubsub.DropOldest,
	})

	// example: subscribe stripe payment succeeded
	subStripe, _ := c.Pub.Subscribe("in.stripe.payment_intent.succeeded", pubsub.SubOptions{
		BufferSize: 64,
		Policy:     pubsub.DropOldest,
	})

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-subGH.C():
				if !ok {
					return
				}
				logInbound("github", msg)
			case msg, ok := <-subStripe.C():
				if !ok {
					return
				}
				logInbound("stripe", msg)
			}
		}
	}()

	return func() {
		subGH.Cancel()
		subStripe.Cancel()
		<-done
	}
}

func logInbound(provider string, msg pubsub.Message) {
	// msg.Data is json.RawMessage
	raw, _ := msg.Data.(json.RawMessage)
	log.Printf("[inbound][%s] topic=%s type=%s id=%s trace=%s bytes=%d at=%s",
		provider, msg.Topic, msg.Type, msg.ID, msg.Meta["trace_id"], len(raw), time.Now().Format(time.RFC3339))
}
