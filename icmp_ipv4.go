package pinger

import (
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type icmpIPv4Handler struct{}

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

func (h *icmpIPv4Handler) Parse(b []byte) (*icmp.Message, error) {
	return icmp.ParseMessage(1, b)
}

func (h *icmpIPv4Handler) Read(conn *icmp.PacketConn) (b []byte, nb int, ttl int, err error) {
	b = make([]byte, 512)
	var cm *ipv4.ControlMessage
	nb, cm, _, err = conn.IPv4PacketConn().ReadFrom(b)
	if cm != nil {
		ttl = cm.TTL
	}
	return
}

func (h *icmpIPv4Handler) ReplyType() icmp.Type {
	return ipv4.ICMPTypeEchoReply
}

func (h *icmpIPv4Handler) RequestType() icmp.Type {
	return ipv4.ICMPTypeEcho
}
