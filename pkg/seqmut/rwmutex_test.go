package seqmut

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

const MaxUint64 = ^uint64(0)

func TestReadHappyPath(t *testing.T) {
	var rw RWMutex
	v := 0

	var readValue int
	stamp := rw.RStamp()
	for {
		// Start critical section

		readValue = v

		// End critical section
		if rw.Ok(stamp) {
			break
		}
	}

	assert.Equal(t, v, readValue)
}

func TestTwoReadersHappy(t *testing.T) {
	var rw RWMutex
	v := 0

	var read1 int
	var read2 int
	stamp1 := rw.RStamp()
	stamp2 := rw.RStamp()

	read1 = v
	read2 = v

	assert.True(t, rw.Ok(stamp1))
	assert.True(t, rw.Ok(stamp2))
	assert.Equal(t, v, read1)
	assert.Equal(t, v, read2)
}

func TestOkIsFalseIfWriterArrivesAfterStampAcquired(t *testing.T) {
	var rw RWMutex

	stamp := rw.RStamp()
	rw.Lock()

	assert.False(t, rw.Ok(stamp))

	// Retry fails as well, since writer is still active
	assert.False(t, rw.Ok(stamp))
}

func TestOkIsFalseIfWriterArrivesBeforeStampAcquired(t *testing.T) {
	var rw RWMutex

	rw.Lock()
	stamp := rw.RStamp()

	assert.False(t, rw.Ok(stamp))

	// Retry fails as well, since writer is still active
	assert.False(t, rw.Ok(stamp))
}

func TestOkIsFalseIfWriterArrivesBeforeStampAcquiredAndLeavesBeforeOk(t *testing.T) {
	var rw RWMutex

	rw.Lock()
	stamp := rw.RStamp()
	rw.Unlock()

	assert.False(t, rw.Ok(stamp))

	// After a retry, we are ok
	assert.True(t, rw.Ok(stamp))
}

// Clone of the sync.RWMutex hammer test
func TestRWMutex(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(-1))
	n := 1000
	if testing.Short() {
		n = 5
	}
	HammerRWMutex(1, 1, n, 0)
	HammerRWMutex(1, 3, n, 0)
	HammerRWMutex(1, 10, n, 0)
	HammerRWMutex(4, 1, n, 0)
	HammerRWMutex(4, 3, n, 0)
	HammerRWMutex(4, 10, n, 0)
	HammerRWMutex(10, 1, n, 0)
	HammerRWMutex(10, 3, n, 0)
	HammerRWMutex(10, 10, n, 0)
	HammerRWMutex(10, 5, n, 0)
}

// Same as above, but with the sequence close to wrapping around back to 0
func TestRWMutex_SequenceWrapAround(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(-1))
	n := 1000
	initialSequence := MaxUint64 - 251
	if testing.Short() {
		initialSequence = MaxUint64 - 1
		n = 5
	}
	HammerRWMutex(1, 1, n, initialSequence)
	HammerRWMutex(1, 3, n, initialSequence)
	HammerRWMutex(1, 10, n, initialSequence)
	HammerRWMutex(4, 1, n, initialSequence)
	HammerRWMutex(4, 3, n, initialSequence)
	HammerRWMutex(4, 10, n, initialSequence)
	HammerRWMutex(10, 1, n, initialSequence)
	HammerRWMutex(10, 3, n, initialSequence)
	HammerRWMutex(10, 10, n, initialSequence)
	HammerRWMutex(10, 5, n, initialSequence)
}

func HammerRWMutex(gomaxprocs, numReaders, numIterations int, initialSequence uint64) {
	runtime.GOMAXPROCS(gomaxprocs)

	// Initial sequence must be even, sanity check early
	if (initialSequence & 1) != 0 {
		panic(fmt.Sprintf("initial sequence must be even, got %d", initialSequence))
	}

	// Number of active readers + 10000 * number of active writers.
	var activity int32
	var rwm RWMutex
	rwm.sequence = initialSequence

	cdone := make(chan bool)
	go writer(&rwm, numIterations, &activity, cdone)
	var i int
	for i = 0; i < numReaders/2; i++ {
		go optimisticReader(&rwm, numIterations, &activity, cdone)
	}
	go writer(&rwm, numIterations, &activity, cdone)
	for ; i < numReaders; i++ {
		go optimisticReader(&rwm, numIterations, &activity, cdone)
	}
	// Wait for the 2 writers and all readers to finish.
	for i := 0; i < 2+numReaders; i++ {
		<-cdone
	}
}

func optimisticReader(rwm *RWMutex, numIterations int, activity *int32, cdone chan bool) {
	for i := 0; i < numIterations; i++ {
		var n1, n2 int32
		stamp := rwm.RStamp()
		for {
			// Do the read twice to get some more surface area
			n1 = atomic.LoadInt32(activity)
			for i := 0; i < 100; i++ {
			}
			n2 = atomic.LoadInt32(activity)
			if rwm.Ok(stamp) {
				break
			}
		}
		if n1 != 0 || n2 != 0 {
			panic(fmt.Sprintf("wlock(%d,%d)\n", n1, n2))
		}
	}
	cdone <- true
}

func writer(rwm *RWMutex, num_iterations int, activity *int32, cdone chan bool) {
	for i := 0; i < num_iterations; i++ {
		rwm.Lock()
		n := atomic.AddInt32(activity, 10000)
		if n != 10000 {
			panic(fmt.Sprintf("wlock(%d)\n", n))
		}
		for i := 0; i < 100; i++ {
		}
		atomic.AddInt32(activity, -10000)
		rwm.Unlock()
	}
	cdone <- true
}

func BenchmarkUncontendedRWMutex(b *testing.B) {
	var rw RWMutex

	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			stamp := rw.RStamp()
			for {

				if rw.Ok(stamp) {
					break
				}
			}
		}
	})
}

func BenchmarkContendedRWMutex(b *testing.B) {
	var val uint32
	var rw RWMutex
	cdone := make(chan bool)

	go func() {
		for {
			rw.Lock()
			if val == 7 {
				// Exit signal
				break
			}
			val = 1
			for i := 0; i < 100; i++ {
			}
			val = 0
			rw.Unlock()
			runtime.Gosched()
		}
		cdone <- true
	}()

	b.ResetTimer()

	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			var read uint32
			stamp := rw.RStamp()
			for {
				read = val
				if rw.Ok(stamp) {
					break
				}
			}
			if read != 0 {
				panic(fmt.Sprintf("wlock(%d)", read))
			}
		}
	})

	rw.Lock()
	val = 7
	rw.Unlock()

	<-cdone
}

func BenchmarkUncontendedSyncRWMutex(b *testing.B) {
	var rw sync.RWMutex

	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			rw.RLock()
			rw.RUnlock()
		}
	})
}

func BenchmarkContendedSyncRWMutex(b *testing.B) {
	var val uint32
	var rw sync.RWMutex
	cdone := make(chan bool)

	go func() {
		for {
			rw.Lock()
			if val == 7 {
				// Exit signal
				break
			}
			val = 1
			for i := 0; i < 100; i++ {
			}
			val = 0
			rw.Unlock()
			runtime.Gosched()
		}
		cdone <- true
	}()

	b.ResetTimer()

	b.RunParallel(func (pb *testing.PB) {
		for pb.Next() {
			var read uint32
			rw.RLock()
			read = val
			rw.RUnlock()

			if read != 0 {
				panic(fmt.Sprintf("wlock(%d)", read))
			}
		}
	})

	rw.Lock()
	val = 7
	rw.Unlock()

	<-cdone
}
