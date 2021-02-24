package pinger

import (
	"bytes"
	"context"
	"math"
	"math/rand"
	"net"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	timeSliceLength = 8
	trackerLength   = 8
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
	chanRawPacket   chan RawPacket
}

type icmpMessageProvider struct {
	id      int
	msgType icmp.Type
	seq     int
	tracker int64
}

// ICMP pinger.
// This pinger requires the process to have root privileges.
func ICMP(cfg ICMPConfig) (Pinger, error) {
	if err := validateICMPConfig(&cfg); err != nil {
		return nil, err
	}
	p := New(&icmpDriver{
		config: cfg,
	})
	return p, nil
}

func newICMPMessageProvider(addr *net.IPAddr) *icmpMessageProvider {
	var msgType icmp.Type
	if isIPv4(addr.IP) {
		msgType = ipv4.ICMPTypeEcho
	} else {
		msgType = ipv6.ICMPTypeEchoRequest
	}

	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)

	return &icmpMessageProvider{
		id:      rng.Intn(math.MaxInt16),
		msgType: msgType,
		seq:     0,
		tracker: rng.Int63n(math.MaxInt64),
	}
}

func (messageProvider *icmpMessageProvider) Provide(addr *net.IPAddr) *icmp.Message {
	t := timeToBytes(time.Now())
	t = append(t, intToBytes(messageProvider.tracker)...)
	if remainSize := timeSliceLength - trackerLength; remainSize > 0 {
		t = append(t, bytes.Repeat([]byte{1}, remainSize)...)
	}

	msg := &icmp.Message{
		Type: messageProvider.msgType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   messageProvider.id,
			Seq:  messageProvider.seq,
			Data: t,
		},
	}

	messageProvider.seq++
	return msg
}

func (p *icmpDriver) Address() net.Addr {
	return p.config.Addr
}

func (p *icmpDriver) Connect(ctx context.Context) error {
	c, err := p.newConnection()
	if err != nil {
		return err
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.messageProvider = newICMPMessageProvider(p.config.Addr)
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

	select {
	case <-p.ctx.Done():
		return RawPacket{}, p.ctx.Err()
	case err := <-errc:
		return RawPacket{}, err
	case packet := <-p.chanRawPacket:
		return packet, nil
	}
}

func (p *icmpDriver) newConnection() (c *icmp.PacketConn, err error) {
	connected := false
	if isIPv4(p.config.Addr.IP) {
		if c, err = icmp.ListenPacket("ip4:icmp", ""); err == nil {
			connected = true
			err = c.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
		}
	} else {
		if c, err = icmp.ListenPacket("ip6:ipv6-icmp", ""); err == nil {
			connected = true
			err = c.IPv6PacketConn().SetControlMessage(ipv6.FlagHopLimit, true)
		}
	}
	if connected && err != nil {
		// failsafe - if there is any error, scrap the connection
		c.Close()
		c = nil
	}
	return
}

func (p *icmpDriver) recv() error {
	defer close(p.chanRawPacket)
	for {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
			packet, err := p.recvPacket()
			if netErr, ok := err.(*net.OpError); ok {
				if netErr.Timeout() {
					continue
				} else {
					return err
				}
			}
			p.chanRawPacket <- packet
		}
	}
}

func (p *icmpDriver) recvPacket() (RawPacket, error) {
	if err := p.packetConn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout)); err != nil {
		return RawPacket{}, err
	}
	b := make([]byte, 512)
	var n, ttl int
	var err error
	if isIPv4(p.config.Addr.IP) {
		var cm *ipv4.ControlMessage
		n, cm, _, err = p.packetConn.IPv4PacketConn().ReadFrom(b)
		if cm != nil {
			ttl = cm.TTL
		}
	} else {
		var cm *ipv6.ControlMessage
		n, cm, _, err = p.packetConn.IPv6PacketConn().ReadFrom(b)
		if cm != nil {
			ttl = cm.HopLimit
		}
	}
	if err != nil {
		return RawPacket{}, err
	}

	packet := RawPacket{
		Message: b,
		Size:    n,
		TTL:     time.Duration(ttl),
	}
	return packet, nil
}

func (p *icmpDriver) send() error {
	msg := p.messageProvider.Provide(p.config.Addr)
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

func validateICMPConfig(cfg *ICMPConfig) error {
	// Addr required
	if cfg.Addr == nil {
		return ErrNoAddress
	}
	// ReadTimeout optional, default to 100ms
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 100 * time.Millisecond
	}
	return nil
}
