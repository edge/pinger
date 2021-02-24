package pinger

import (
	"context"
	"sync"
	"time"
)

// Report produced by a Stats aggregator.
type Report struct {
	NumPings      int
	NumSuccessful int
	NumFailed     int

	MeanRTT time.Duration
}

// Stats aggregator.
type Stats interface {
	Calculate() Report // Calculate a new report based on current statistics.
}

type statResult struct {
	Packet Packet
	Err    error
}

type stats struct {
	agg []statResult
}

type tracker struct {
	next  Pinger
	mut   *sync.Mutex
	stats *stats
}

// Track statistics for a pinger.
// For example, you might want to send five pings to a given host and get the average RTT.
// This middleware provides a wrapped Pinger and a Stats aggregator that you can calculate reports from at any time.
func Track(next Pinger) (Pinger, Stats) {
	s := &stats{
		agg: []statResult{},
	}
	t := &tracker{
		next:  next,
		mut:   &sync.Mutex{},
		stats: s,
	}
	return t, s
}

func (s *stats) Calculate() (rep Report) {
	pkts, errs := s.collectTyped()
	numPkts := len(pkts)
	numErrs := len(errs)

	rep.NumPings = len(s.agg)
	rep.NumSuccessful = numPkts
	rep.NumFailed = numErrs

	if numPkts > 0 {
		var totalRTT time.Duration = 0
		for _, pkt := range pkts {
			totalRTT += pkt.RTT
		}
		rep.MeanRTT = totalRTT / time.Duration(len(pkts))
	}

	return
}

func (s *stats) collectTyped() (pkts []Packet, errs []error) {
	for _, result := range s.agg {
		if result.Err != nil {
			errs = append(errs, result.Err)
		} else {
			pkts = append(pkts, result.Packet)
		}
	}
	return
}

func (t *tracker) Connect(ctx context.Context) error {
	return t.next.Connect(ctx)
}

func (t *tracker) Disconnect() error {
	return t.next.Disconnect()
}

func (t *tracker) Ping() (Packet, error) {
	pkt, err := t.next.Ping()

	t.mut.Lock()
	t.stats.agg = append(t.stats.agg, statResult{
		Packet: pkt,
		Err:    err,
	})
	t.mut.Unlock()

	return pkt, err
}
