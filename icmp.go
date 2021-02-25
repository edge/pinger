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

func (p *icmpDriver) Address() net.Addr {
	return p.config.Addr
}

func (p *icmpDriver) Connect(ctx context.Context) error {
	c, err := p.protocolHandler.Listen("")
	if err != nil {
		return err
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.messageProvider = newICMPMessageProvider(p.protocolHandler, p.config.Addr)
	p.packetConn = c
	p.chanRawPacket = make(chan RawPacket)

	go p.recv()
	return nil
}

func (p *icmpDriver) Disconnect() error {
	p.cancel()
	err := p.packetConn.Close()
	return err
}

func (p *icmpDriver) Ping() (RawPacket, error) {
	errc := make(chan error, 1)
	defer close(errc)
	go func() {
		if err := p.send(); err != nil {
			errc <- err
		}
	}()

	for {
		select {
		case <-p.ctx.Done():
			return RawPacket{}, p.ctx.Err()
		case err := <-errc:
			return RawPacket{}, err
		case raw := <-p.chanRawPacket:
			if err := p.handlePacket(raw); err != nil {
				if isICMPIgnoredPacket(err) {
					continue
				}
				return RawPacket{}, err
			}
			return raw, nil
		}
	}
}

func (p *icmpDriver) handlePacket(raw RawPacket) error {
	msg, err := p.protocolHandler.Parse(raw.Message)
	if err != nil {
		return err
	}
	echo := msg.Body.(*icmp.Echo)
	// ignore if not echo reply
	if msg.Type != p.protocolHandler.ReplyType() {
		return ErrICMPIgnoredPacket
	}
	// ignore if id mismatched
	if echo.ID != p.messageProvider.id {
		return ErrICMPIgnoredPacket
	}

	track, _, err := p.messageProvider.ReadData(msg)
	if err != nil {
		return err
	}
	// ignore if tracker mismatched
	if track != p.messageProvider.tracker {
		return ErrICMPIgnoredPacket
	}

	// TODO: use ReadData 2nd return to set RTT
	return nil
}

func (p *icmpDriver) recv() error {
	defer close(p.chanRawPacket)
	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
			raw, err := p.recvPacket()
			if netErr, ok := err.(*net.OpError); ok {
				if netErr.Timeout() {
					continue
				} else {
					return err
				}
			}
			p.chanRawPacket <- raw
		}
	}
}

func (p *icmpDriver) recvPacket() (RawPacket, error) {
	if err := p.packetConn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout)); err != nil {
		return RawPacket{}, err
	}
	b, nb, ttl, err := p.protocolHandler.Read(p.packetConn)
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

func (p *icmpDriver) send() error {
	msg := p.messageProvider.Provide()
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return err
	}

	for {
		_, err := p.packetConn.WriteTo(msgBytes, p.config.Addr)
		if err != nil {
			netErr, ok := err.(*net.OpError)
			if ok && netErr.Err == syscall.ENOBUFS {
				continue
			}
			return err
		}
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
