package plugin

import (
	"encoding/binary"
	"time"
)

const (
	requestCompletionWindow     = 15 * time.Minute
	requestCompletionFilterBits = 1 << 20
	requestCompletionFilterMask = requestCompletionFilterBits - 1
	requestCompletionWords      = requestCompletionFilterBits / 64
	requestCompletionHashes     = 4
)

// requestCompletionFilter keeps two fixed-size Bloom windows. A completion is
// retained for at least requestCompletionWindow without capacity eviction.
// False positives only suppress an estimated metric; duplicates within the
// window do not become false negatives as traffic grows.
type requestCompletionFilter struct {
	current        []uint64
	previous       []uint64
	currentStarted time.Time
}

func newRequestCompletionFilter() *requestCompletionFilter {
	return &requestCompletionFilter{
		current:  make([]uint64, requestCompletionWords),
		previous: make([]uint64, requestCompletionWords),
	}
}

func (filter *requestCompletionFilter) contains(key requestKey, now time.Time) bool {
	filter.rotate(now)
	for index := 0; index < requestCompletionHashes; index++ {
		position := binary.LittleEndian.Uint64(key[index*8:]) & requestCompletionFilterMask
		word := position / 64
		bit := uint64(1) << (position % 64)
		if filter.current[word]&bit == 0 && filter.previous[word]&bit == 0 {
			return false
		}
	}
	return true
}

func (filter *requestCompletionFilter) add(key requestKey, now time.Time) {
	filter.rotate(now)
	for index := 0; index < requestCompletionHashes; index++ {
		position := binary.LittleEndian.Uint64(key[index*8:]) & requestCompletionFilterMask
		word := position / 64
		filter.current[word] |= uint64(1) << (position % 64)
	}
}

func (filter *requestCompletionFilter) rotate(now time.Time) {
	if filter.currentStarted.IsZero() {
		filter.currentStarted = now
		return
	}
	if !now.After(filter.currentStarted) {
		return
	}
	elapsed := now.Sub(filter.currentStarted)
	steps := elapsed / requestCompletionWindow
	if steps == 0 {
		return
	}
	if steps == 1 {
		filter.previous, filter.current = filter.current, filter.previous
		clear(filter.current[:])
	} else {
		clear(filter.current[:])
		clear(filter.previous[:])
	}
	filter.currentStarted = filter.currentStarted.Add(steps * requestCompletionWindow)
}
