package pinger

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"

	"golang.org/x/net/icmp"
)

type icmpMessageProvider struct {
	mut *sync.Mutex

	id      int
	msgType icmp.Type
	seq     int
	tracker int64
}

func newICMPMessageProvider(h icmpProtocolHandler, addr *net.IPAddr) *icmpMessageProvider {
	src := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(src)

	return &icmpMessageProvider{
		mut: &sync.Mutex{},

		id:      rng.Intn(math.MaxInt16),
		msgType: h.RequestType(),
		seq:     0,
		tracker: rng.Int63n(math.MaxInt64),
	}
}

func (p *icmpMessageProvider) ReadData(msg *icmp.Message) (int64, time.Time, error) {
	echo := msg.Body.(*icmp.Echo)
	if ld := len(echo.Data); ld < timeSize+trackerSize {
		return 0, time.Unix(0, 0), errICMPReplyTooShort(timeSize+trackerSize, ld)
	}
	timestamp := bytesToTime(echo.Data[:timeSize])
	tracker := bytesToInt(echo.Data[timeSize:(timeSize + trackerSize)])
	return tracker, timestamp, nil
}

func (p *icmpMessageProvider) Provide() *icmp.Message {
	p.mut.Lock()
	defer p.mut.Unlock()

	msg := &icmp.Message{
		Type: p.msgType,
		Code: 0,
		Body: &icmp.Echo{
			ID:  p.id,
			Seq: p.seq,
		},
	}
	p.WriteData(msg)

	p.seq++
	return msg
}

func (p *icmpMessageProvider) WriteData(msg *icmp.Message) {
	// data[:8] is int64 timestamp
	data := timeToBytes(time.Now())
	// data[8:] is tracker id
	data = append(data, intToBytes(p.tracker)...)
	if ld := (timeSize + trackerSize) - len(data); ld > 0 {
		data = append(data, bytes.Repeat([]byte{1}, ld)...)
	}
	msg.Body.(*icmp.Echo).Data = data
}

func errICMPReplyTooShort(expected, actual int) error {
	return fmt.Errorf("reply too short (expected %dB; actual %dB)", expected, actual)
}
