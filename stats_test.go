package pinger

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	// testStatsAllowMaxRTT is an upper bound to permit some variation in test timings, as we cannot ensure reported RTT average is exact.
	testStatsAllowMaxRTT = 60 * time.Millisecond

	testStatsWaitTime = 50 * time.Millisecond
)

func Test_Stats(t *testing.T) {
	a := assert.New(t)
	pinger, stats := Track(Dummy(testStatsWaitTime))

	if !a.Nil(pinger.Connect(context.Background())) {
		return
	}
	defer func() {
		a.Nil(pinger.Disconnect())
	}()

	wg := &sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			pinger.Ping()
			wg.Done()
		}()
	}
	wg.Wait()

	report := stats.Calculate()
	a.Equal(10, report.NumPings)
	a.Equal(10, report.NumSuccessful)
	a.Equal(0, report.NumFailed)

	a.GreaterOrEqual(report.MeanRTT, testStatsWaitTime)
	a.LessOrEqual(report.MeanRTT, testStatsAllowMaxRTT)
}

func Test_Stats_Errors(t *testing.T) {
	a := assert.New(t)
	pinger, stats := Track(Errors(1, Dummy(testStatsWaitTime)))

	if !a.Nil(pinger.Connect(context.Background())) {
		return
	}
	defer func() {
		a.Nil(pinger.Disconnect())
	}()

	wg := &sync.WaitGroup{}
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			pinger.Ping()
			wg.Done()
		}()
	}
	wg.Wait()

	report := stats.Calculate()
	a.Equal(10, report.NumPings)
	a.Equal(0, report.NumSuccessful)
	a.Equal(10, report.NumFailed)
	// we don't test any other stats here as they won't be meaningfully calculated without successful packets
}
