package pinger

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// Inbuilt error.
var (
	ErrForcedError = errors.New("forced failure by error pinger")
)

type errorPinger struct {
	next Pinger

	chance float64
	rng    *rand.Rand
}

// Errors adds a chance of forced error to ping calls.
// Error chance is represented as a float in the range 0.0-1.0, where 0 means no errors will ever occur and 1 means every ping will fail.
// This is mainly useful for tests.
func Errors(chance float64, next Pinger) Pinger {
	return &errorPinger{
		next:   next,
		chance: chance,
	}
}

func (p *errorPinger) Connect(ctx context.Context) error {
	p.rng = rand.New(rand.NewSource(time.Now().Unix()))
	return p.next.Connect(ctx)
}

func (p *errorPinger) Disconnect() error {
	return p.next.Disconnect()
}

func (p *errorPinger) Ping() (Packet, error) {
	if p.hasError() {
		return Packet{}, ErrForcedError
	}
	return p.next.Ping()
}

func (p *errorPinger) hasError() bool {
	n := p.rng.Float64()
	return p.chance > n
}
