package pinger

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func doTestHTTP(a *assert.Assertions, pinger Pinger) *Packet {
	if err := pinger.Connect(context.Background()); !a.Nil(err) {
		return nil
	}
	defer pinger.Disconnect()
	packet, err := pinger.Ping()
	if !a.Nil(err) {
		return nil
	}
	return &packet
}

func Test_HTTP_GET(t *testing.T) {
	a := assert.New(t)
	pinger, err := HTTP(http.MethodGet, "https://edge.network")
	if !a.Nil(err) {
		return
	}
	packet := doTestHTTP(a, pinger)
	if packet != nil {
		a.Less(0, packet.Size)
	}
}

func Test_HTTP_HEAD(t *testing.T) {
	a := assert.New(t)
	pinger, err := HTTP(http.MethodHead, "https://edge.network")
	if !a.Nil(err) {
		return
	}
	packet := doTestHTTP(a, pinger)
	if packet != nil {
		a.Equal(0, packet.Size)
	}
}
