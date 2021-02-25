package pinger

import (
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

type icmpIPv6Handler struct{}

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

func (h *icmpIPv6Handler) Parse(b []byte) (*icmp.Message, error) {
	return icmp.ParseMessage(58, b)
}

func (h *icmpIPv6Handler) Read(conn *icmp.PacketConn) (b []byte, nb int, ttl int, err error) {
	b = make([]byte, 512)
	var cm *ipv6.ControlMessage
	nb, cm, _, err = conn.IPv6PacketConn().ReadFrom(b)
	if cm != nil {
		ttl = cm.HopLimit
	}
	return
}

func (h *icmpIPv6Handler) ReplyType() icmp.Type {
	return ipv6.ICMPTypeEchoReply
}

func (h *icmpIPv6Handler) RequestType() icmp.Type {
	return ipv6.ICMPTypeEchoRequest
}
