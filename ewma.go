package main

import (
	"time"
)

// ewma hold the state for an Exponential Weighted Moving Average. For details see:
// https://en.wikipedia.org/wiki/Moving_average#Exponential_moving_average
// https://github.com/dgryski/trifles/blob/master/ewmaest/ewmaest.go
// https://stackoverflow.com/a/936720
type ewma struct {
	total         int
	completed     int
	lastCompleted int

	start time.Time
	last  time.Time

	perItem time.Duration

	α, β float64
}

// newEWMA returns a new EWMA for total items.
//nolint:gomnd
func newEWMA(start time.Time, totalItems int) *ewma {
	return &ewma{
		start: start,
		last:  start,
		total: totalItems,
		α:     0.10,
		β:     0.5,
	}
}

// Report tells the ewma how many items have been processed.
func (e *ewma) Report(totalCompletedItems int) {
	// return early if no new information is being reported
	if totalCompletedItems == 0 || e.completed == totalCompletedItems {
		return
	}

	e.completed = totalCompletedItems

	lastBlockTime := time.Since(e.last)
	e.last = time.Now()

	lastItemEstimate := lastBlockTime / time.Duration(e.completed-e.lastCompleted)
	e.lastCompleted = e.completed

	// use the first measurement directly, without applying α
	if e.perItem == 0 {
		e.perItem = lastItemEstimate

		return
	}

	e.perItem = time.Duration(e.α*float64(lastItemEstimate)) + time.Duration((1-e.α)*float64(e.perItem))
}

// ETA returns the estimated remaining time.
func (e *ewma) ETA() time.Duration {
	remaining := e.total - e.completed

	perItem := e.perItem
	if e.completed > 0 {
		perItem = time.Duration(e.β * float64(e.last.Sub(e.start)) / float64(e.completed))
		perItem += time.Duration((1 - e.β) * float64(e.perItem))
	}

	d := time.Duration(remaining) * perItem

	return d
}
