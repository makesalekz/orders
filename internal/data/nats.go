package data

import (
	nats "github.com/nats-io/nats.go"
	"github.com/makesalekz/orders/internal/conf"
)

func NewNatsClient(conf *conf.Bootstrap) (*nats.Conn, func(), error) {
	natsURL := conf.GetNats()
	if natsURL == "" {
		return nil, func() {}, nil
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		nc.Close()
	}

	return nc, cleanup, nil
}
