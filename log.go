package pinger

import (
	"context"
	"fmt"
	"time"

	"github.com/edge/logger"
)

type pingerLogger struct {
	log     *logger.Instance
	context string
	next    Pinger
}

// Log pinger function calls, including connecting and disconnecting.
func Log(log *logger.Instance, context string, next Pinger) Pinger {
	return &pingerLogger{
		log:     log,
		context: context,
		next:    next,
	}
}

func (p *pingerLogger) Connect(ctx context.Context) error {
	err := p.next.Connect(ctx)
	lc := p.log.Context(p.context).Label("func", "connect")
	if err != nil {
		lc.Error(err)
	} else {
		lc.Trace("OK")
	}
	return err
}

func (p *pingerLogger) Disconnect() error {
	err := p.next.Disconnect()
	lc := p.log.Context(p.context).Label("func", "disconnect")
	if err != nil {
		lc.Error(err)
	} else {
		lc.Trace("OK")
	}
	return err
}

func (p *pingerLogger) Ping() (Packet, error) {
	pkt, err := p.next.Ping()
	lc := p.log.Context(p.context).Label("func", "ping")
	if err != nil {
		lc.Error(err)
	} else {
		lc.Trace(p.formatPacket(pkt))
	}
	return pkt, err
}

func (p *pingerLogger) formatPacket(pkt Packet) string {
	return fmt.Sprintf("received %dB from %s in %dms", pkt.Size, pkt.Address.String(), (pkt.RTT / time.Millisecond))
}
