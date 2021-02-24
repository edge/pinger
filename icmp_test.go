package pinger

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func doTestICMP(a *assert.Assertions, pinger Pinger) *Packet {
	if err := pinger.Connect(context.Background()); err != nil {
		netErr, ok := err.(*net.OpError)
		if ok {
			a.Fail(fmt.Sprint(netErr))
		} else {
			a.Nil(err)
		}
		return nil
	}
	defer func() {
		a.Nil(pinger.Disconnect())
	}()

	packet, err := pinger.Ping()
	if err != nil {
		netErr, ok := err.(*net.OpError)
		if ok {
			a.Fail(fmt.Sprint(netErr))
		} else {
			a.Nil(err)
		}
		return nil
	}
	return &packet
}

func Test_ICMP_IPv4(t *testing.T) {
	a := assert.New(t)
	addr, err := net.ResolveIPAddr("ip4", "edge.network")
	if !a.Nil(err) {
		return
	}
	pinger, err := ICMP(ICMPConfig{
		Addr: addr,
	})
	if !a.Nil(err) {
		return
	}
	doTestICMP(a, pinger)
}

func Test_ICMP_IPv6(t *testing.T) {
	a := assert.New(t)
	addr, err := net.ResolveIPAddr("ip6", "::1")
	if !a.Nil(err) {
		return
	}
	pinger, err := ICMP(ICMPConfig{
		Addr: addr,
	})
	if !a.Nil(err) {
		return
	}
	doTestICMP(a, pinger)
}
