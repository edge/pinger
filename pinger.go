package pinger

import (
	"context"
	"errors"
	"net"
	"time"
)

// Pinger state error.
var (
	ErrAlreadyConnected = errors.New("already connected")
	ErrNoAddress        = errors.New("no address")
	ErrNotConnected     = errors.New("not connected")
)

// Driver handles the underlying pinging connection and provides metadata to Pinger.
type Driver interface {
	Address() net.Addr // Address to ping.

	Connect(context.Context) error  // Connect to host.
	Disconnect() error              // Disconnect from host.
	Ping(*Timer) (RawPacket, error) // Ping host.
}

// Pinger reflects a standard pinging API.
// See New() for detail on a standard, private implementation that uses a Driver for portability.
type Pinger interface {
	Connect(context.Context) error // Connect to host.
	Disconnect() error             // Disconnect from host.
	Ping() (Packet, error)         // Ping host.
}

// Packet describes a fully processed packet built from other, constituent packet types.
type Packet struct {
	PacketMeta
	RawPacket
	TimedPacket
}

// PacketMeta describes metadata from the ping environment, including the request, rather than originating from the ping response.
type PacketMeta struct {
	Address net.Addr // Address of the host being pinged.
}

// RawPacket describes the raw data available from a ping response.
type RawPacket struct {
	Message []byte        // Message in response packet.
	Size    int           // Size of response message in bytes.
	TTL     time.Duration // TTL (Time To Live) of the packet.
}

// TimedPacket describes statistical data available for a ping response.
type TimedPacket struct {
	RTT  time.Duration // RTT (Round Trip Time) reflects the time between sending a ping and receiving a response.
	Sent time.Time     // Sent time of request.
}

type pinger struct {
	ctx       context.Context
	connected bool
	driver    Driver
}

// New pinger.
// The standard implementation wraps a Driver and attaches metadata and timings to the response packet before returning it to user code.
func New(d Driver) Pinger {
	return &pinger{
		driver: d,
	}
}

func (p *pinger) Connect(ctx context.Context) error {
	if p.connected {
		return ErrAlreadyConnected
	}
	err := p.driver.Connect(ctx)
	if err == nil {
		p.connected = true
	}
	return err
}

func (p *pinger) Disconnect() error {
	if !p.connected {
		return ErrNotConnected
	}
	err := p.driver.Disconnect()
	if err == nil {
		p.connected = false
	}
	return err
}

func (p *pinger) Ping() (Packet, error) {
	timer := &Timer{}
	raw, err := p.driver.Ping(timer)
	if err != nil {
		return Packet{}, err
	}
	packet := Packet{
		RawPacket: raw,
		PacketMeta: PacketMeta{
			Address: p.driver.Address(),
		},
		TimedPacket: TimedPacket{
			RTT:  timer.Elapsed(),
			Sent: timer.Started,
		},
	}

	return packet, nil
}
