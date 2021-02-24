package pinger

import (
	"context"
	"net"
	"time"
)

type dummyDriver struct {
	ctx       context.Context
	cancel    context.CancelFunc
	errChance int
	wait      time.Duration
}

// Dummy pinger.
// Simply provide a wait duration and it will pretend to ping.
// Useful for tests.
func Dummy(wait time.Duration) Pinger {
	return New(&dummyDriver{
		wait: wait,
	})
}

func (d *dummyDriver) Address() net.Addr {
	l, err := net.ResolveIPAddr("ip4", "localhost")
	if err != nil {
		panic(err)
	}
	return l
}

func (d *dummyDriver) Connect(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)
	return nil
}

func (d *dummyDriver) Disconnect() error {
	d.cancel()
	return nil
}

func (d *dummyDriver) Ping() (RawPacket, error) {
	raw := RawPacket{
		Message: []byte{},
	}
	time.Sleep(d.wait)
	return raw, nil
}
