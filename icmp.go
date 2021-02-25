package pinger

import (
	"bytes"
	"context"
	"math"
	"math/rand"
	"net"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	defaultReadTimeout = 100 * time.Millisecond
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
	protocolHandler icmpProtocolHandler
	chanRawPacket   chan RawPacket
}

type icmpMessageProvider struct {
	mut *sync.Mutex

	id      int
	msgType icmp.Type
	seq     int
	tracker int64
}

type icmpProtocolHandler interface {
	Listen(addr string) (*icmp.PacketConn, error)
	MessageType() icmp.Type
	Read(*icmp.PacketConn) (b []byte, nb int, ttl int, err error)
}

type icmpIPv4Handler struct{}
type icmpIPv6Handler struct{}

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

func newICMPMessageProvider(h icmpProtocolHandler, addr *net.IPAddr) *icmpMessageProvider {
	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)

	return &icmpMessageProvider{
		mut: &sync.Mutex{},

		id:      rng.Intn(math.MaxInt16),
		msgType: h.MessageType(),
		seq:     0,
		tracker: rng.Int63n(math.MaxInt64),
	}
}

func newProtocolHandler(addr *net.IPAddr) (h icmpProtocolHandler) {
	if isIPv4(addr.IP) {
		h = &icmpIPv4Handler{}
	} else {
		h = &icmpIPv6Handler{}
	}
	return
}

func (p *icmpMessageProvider) Provide(addr *net.IPAddr) *icmp.Message {
	p.mut.Lock()
	defer p.mut.Unlock()

	t := timeToBytes(time.Now())
	t = append(t, intToBytes(p.tracker)...)
	if remainSize := timeSliceLength - trackerLength; remainSize > 0 {
		t = append(t, bytes.Repeat([]byte{1}, remainSize)...)
	}

	msg := &icmp.Message{
		Type: p.msgType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   p.id,
			Seq:  p.seq,
			Data: t,
		},
	}

	p.seq++
	return msg
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

	select {
	case <-p.ctx.Done():
		return RawPacket{}, p.ctx.Err()
	case err := <-errc:
		return RawPacket{}, err
	case packet := <-p.chanRawPacket:
		return packet, nil
	}
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
	b, nb, ttl, err := p.protocolHandler.Read(p.packetConn)
	if err != nil {
		return RawPacket{}, err
	}
	packet := RawPacket{
		Message: b,
		Size:    nb,
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

func (h *icmpIPv4Handler) Listen(addr string) (conn *icmp.PacketConn, err error) {
	ok := false
	if conn, err = icmp.ListenPacket("ip4:icmp", addr); err == nil {
		ok = true
		err = conn.IPv4PacketConn().SetControlMessage(ipv4.FlagTTL, true)
	}
	if ok && err != nil {
		conn.Close()
		conn = nil
	}
	return
}

func (h *icmpIPv4Handler) MessageType() icmp.Type {
	return ipv4.ICMPTypeEcho
}

func (h *icmpIPv4Handler) Read(conn *icmp.PacketConn) (b []byte, nb int, ttl int, err error) {
	var cm *ipv4.ControlMessage
	nb, cm, _, err = conn.IPv4PacketConn().ReadFrom(b)
	if cm != nil {
		ttl = cm.TTL
	}
	return
}

func (h *icmpIPv6Handler) Listen(addr string) (conn *icmp.PacketConn, err error) {
	ok := false
	if conn, err = icmp.ListenPacket("ip6:ipv6-icmp", addr); err == nil {
		ok = true
		err = conn.IPv6PacketConn().SetControlMessage(ipv6.FlagHopLimit, true)
	}
	if ok && err != nil {
		conn.Close()
		conn = nil
	}
	return
}

func (h *icmpIPv6Handler) MessageType() icmp.Type {
	return ipv6.ICMPTypeEchoRequest
}

func (h *icmpIPv6Handler) Read(conn *icmp.PacketConn) (b []byte, nb int, ttl int, err error) {
	var cm *ipv6.ControlMessage
	nb, cm, _, err = conn.IPv6PacketConn().ReadFrom(b)
	if cm != nil {
		ttl = cm.HopLimit
	}
	return
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
