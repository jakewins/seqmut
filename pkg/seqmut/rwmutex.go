package seqmut

import (
	"sync"
	"sync/atomic"
)

type Stamp uint64

type RWMutex struct {
	mut sync.Mutex
	sequence uint64
}

func (rw *RWMutex) RStamp() *Stamp {
	stamp := Stamp(atomic.LoadUint64(&rw.sequence))
	return &stamp
}

// Used to end a critical section for the optimistic read lock.
// if Ok returns true, the critical section was successful and you can go
// about your business. If it returns false, there was a racing writer, and
// you need to retry; the stamp will have been updated to a new ticket to ride.
func (rw *RWMutex) Ok(stamp *Stamp) (ok bool) {
	current := rw.RStamp()

	// If a writer was holding the mutex before we showed up, and is *still* holding it
	// now that we're on our way out the door, the sequence will have remained the same
	// from our perspective. To guard against this, we guarantee that the sequence is odd
	// any time a writer is active, so we check that here before doing another fenced read
	if (*stamp & 1) == 1 {
		*stamp = *current
		return false
	}

	if *current != *stamp {
		*stamp = *current
		return false
	}

	return true
}

func (rw *RWMutex) Lock() {
	rw.mut.Lock()
	atomic.AddUint64(&rw.sequence, 1)
}

func (rw *RWMutex) Unlock() {
	atomic.AddUint64(&rw.sequence, 1)
	rw.mut.Unlock()
}
