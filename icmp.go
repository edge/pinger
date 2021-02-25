package pinger

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
)

// ICMP internal error.
var (
	ErrICMPIgnoredPacket = errors.New("ignored packet")
)

const (
	defaultReadTimeout = 100 * time.Millisecond
)

// Size in bytes of echo message component.
// 8 bytes represents an int64.
const (
	timeSize    = 8
	trackerSize = 8
)

// ICMPConfig for an ICMP() pinger.
type ICMPConfig struct {
	Addr        *net.IPAddr   // Address of host.
	ReadTimeout time.Duration // ReadTimeout for packet receiver (optional).
}

type icmpDriver struct {
	config ICMPConfig

	ctx    context.Context
	cancel context.CancelFunc

	messageProvider *icmpMessageProvider
	packetConn      *icmp.PacketConn
	protocolHandler icmpProtocolHandler
	chanRawPacket   chan RawPacket
}

type icmpProtocolHandler interface {
	Listen(addr string) (*icmp.PacketConn, error)
	Parse([]byte) (*icmp.Message, error)
	Read(*icmp.PacketConn) (b []byte, nb int, ttl int, err error)
	ReplyType() icmp.Type
	RequestType() icmp.Type
}

// ICMP pinger.
// This pinger requires the process to have root privileges.
func ICMP(cfg ICMPConfig) (Pinger, error) {
	if err := validateICMPConfig(&cfg); err != nil {
		return nil, err
	}
	p := New(&icmpDriver{
		config:          cfg,
		protocolHandler: newProtocolHandler(cfg.Addr),
	})
	return p, nil
}

func newProtocolHandler(addr *net.IPAddr) (h icmpProtocolHandler) {
	if isIPv4(addr.IP) {
		h = &icmpIPv4Handler{}
	} else {
		h = &icmpIPv6Handler{}
	}
	return
}

func (d *icmpDriver) Address() net.Addr {
	return d.config.Addr
}

func (d *icmpDriver) Connect(ctx context.Context) error {
	c, err := d.protocolHandler.Listen("")
	if err != nil {
		return err
	}

	d.ctx, d.cancel = context.WithCancel(ctx)
	d.messageProvider = newICMPMessageProvider(d.protocolHandler, d.config.Addr)
	d.packetConn = c
	d.chanRawPacket = make(chan RawPacket)

	go d.recv()
	return nil
}

func (d *icmpDriver) Disconnect() error {
	d.cancel()
	err := d.packetConn.Close()
	return err
}

func (d *icmpDriver) Ping(timer *Timer) (RawPacket, error) {
	errc := make(chan error, 1)
	defer close(errc)
	go func() {
		if err := d.send(timer); err != nil {
			errc <- err
		}
	}()

	for {
		select {
		case <-d.ctx.Done():
			return RawPacket{}, d.ctx.Err()
		case err := <-errc:
			return RawPacket{}, err
		case raw := <-d.chanRawPacket:
			if err := d.handlePacket(raw); err != nil {
				if isICMPIgnoredPacket(err) {
					continue
				}
				return RawPacket{}, err
			}
			return raw, nil
		}
	}
}

func (d *icmpDriver) handlePacket(raw RawPacket) error {
	msg, err := d.protocolHandler.Parse(raw.Message)
	if err != nil {
		return err
	}
	echo := msg.Body.(*icmp.Echo)
	// ignore if not echo reply
	if msg.Type != d.protocolHandler.ReplyType() {
		return ErrICMPIgnoredPacket
	}
	// ignore if id mismatched
	if echo.ID != d.messageProvider.id {
		return ErrICMPIgnoredPacket
	}

	track, _, err := d.messageProvider.ReadData(msg)
	if err != nil {
		return err
	}
	// ignore if tracker mismatched
	if track != d.messageProvider.tracker {
		return ErrICMPIgnoredPacket
	}

	// TODO: use ReadData 2nd return to set RTT
	return nil
}

func (d *icmpDriver) recv() error {
	defer close(d.chanRawPacket)
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
			raw, err := d.recvPacket()
			if netErr, ok := err.(*net.OpError); ok {
				if netErr.Timeout() {
					continue
				} else {
					return err
				}
			}
			d.chanRawPacket <- raw
		}
	}
}

func (d *icmpDriver) recvPacket() (RawPacket, error) {
	if err := d.packetConn.SetReadDeadline(time.Now().Add(d.config.ReadTimeout)); err != nil {
		return RawPacket{}, err
	}
	b, nb, ttl, err := d.protocolHandler.Read(d.packetConn)
	if err != nil {
		return RawPacket{}, err
	}
	raw := RawPacket{
		Message: b,
		Size:    nb,
		TTL:     time.Duration(ttl),
	}
	return raw, nil
}

func (d *icmpDriver) send(timer *Timer) error {
	msg := d.messageProvider.Provide()
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	timer.Start()
	for {
		_, err := d.packetConn.WriteTo(msgBytes, d.config.Addr)
		if err != nil {
			netErr, ok := err.(*net.OpError)
			if ok && netErr.Err == syscall.ENOBUFS {
				continue
			}
			return err
		}
		timer.Stop()
		return nil
	}
}

func isICMPIgnoredPacket(err error) bool {
	return err == ErrICMPIgnoredPacket
}

func validateICMPConfig(cfg *ICMPConfig) error {
	// Addr required
	if cfg.Addr == nil {
		return ErrNoAddress
	}
	// ReadTimeout optional
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = defaultReadTimeout
	}
	return nil
}
