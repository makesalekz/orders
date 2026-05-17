package data

import (
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	nats "github.com/nats-io/nats.go"
)

const orderConfirmedSubject = "orders.order.confirmed"

type OrderConfirmedEvent struct {
	TenantID       int64  `json:"tenant_id"`
	OrderID        int64  `json:"order_id"`
	CounterpartyID int64  `json:"counterparty_id"`
	Timestamp      string `json:"timestamp"`
}

type OrderConfirmedPublisher interface {
	Publish(event OrderConfirmedEvent)
}

type natsOrderConfirmedPublisher struct {
	nc  *nats.Conn
	log *log.Helper
}

func NewOrderConfirmedPublisher(nc *nats.Conn, logger log.Logger) OrderConfirmedPublisher {
	return &natsOrderConfirmedPublisher{
		nc:  nc,
		log: log.NewHelper(logger),
	}
}

func (p *natsOrderConfirmedPublisher) Publish(event OrderConfirmedEvent) {
	if p.nc == nil {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		p.log.Errorf("failed to marshal order confirmed event: %v", err)
		return
	}

	if err := p.nc.Publish(orderConfirmedSubject, payload); err != nil {
		p.log.Errorf("failed to publish order confirmed event: %v", err)
	}
}
