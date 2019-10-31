# seqmut ("Siegmund") - Sequence Locks in Go

Don't use this.

## Sequence locks

The lock consists of a sequence counter and a regular mutex. 

Readers do a fenced read of the sequence before entering critical section, and a second fence read after exiting it.
If the second sequence read is not equal to the first, or the sequence is odd, the reader retries.

Writers acquire the mutex and increment the sequence before entering critical section. 
When exiting critical section writers first increment the sequence again, and then release the mutex.

The sequence starts at 0. Hence, it will always be odd when a writer is in its critical section.

In the happy case, a reader thus only performs two fenced reads to verify it has successfully executed its critical section.

## API

Readers acquire a "ticket to ride" stamp, and perform their critical work in a loop body.
At the end of the loop body, they verify if they were successful:

```
var rw RWMutex
stamp := rw.RStamp()
for {
    // Start critical section

    // ..

    // End critical section
    if rw.Ok(stamp) {
        break
    }
}
```

Writers use the lock just like normal Go mutexes:

```
var rw RWMutex
rw.Lock()
// Start critical section

// ..

// End critical section
rw.Unlock()
```

## Safety

The go race detector does not like this code, which is why you should not use it:

```
WARNING: DATA RACE
Read at 0x00c0000140b0 by goroutine 84:
  seqmut.BenchmarkContendedRWMutex.func2()
      /home/jake/Code/toy/seqmut/rwmutex_test.go:239 +0x5a
  testing.(*B).RunParallel.func1()
      /usr/local/go/src/testing/benchmark.go:665 +0x160

Previous write at 0x00c0000140b0 by goroutine 15:
  seqmut.BenchmarkContendedRWMutex.func1()
      /home/jake/Code/toy/seqmut/rwmutex_test.go:222 +0x8f

```

However, I *think* the code is safe. 
Putting this on github is part of an effort to confirm this and learn more about the Go memory model and the race detector.

I'm curious if there's a way I can 

1) Confirm this code is indeed safe
2) Help the race detector see that it is safe

According to https://golang.org/ref/mem, the race detector is *right* that there are cases here where ordering of some operations is not well defined.
Specifically, readers race with writers inside critical sections. 

However, the memory model reference does not say behavior is undefined in the "the compiler may empty your bank account and print pictures of elephants" way.
What it says is simply that you can't know if you get to see a write or not if you don't fulfil the second criteria in the "Happens Before" section.

The trick being that we can cheaply detect if a race occurred, and if so we retry.

In other words: *The goal and hope is that there should be no case where a reader does not re-run its critical section if there was a race.*


## Orderings

Given the two concurrent histories, orange signifying operations of a writer, blue operations of a reader:

![history graph](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/overview.png)

Where `R(..)` signifies an unfenced read, `W(..)` signifies an unfenced write, `Load(..)` signifies an atomic read and `Inc(..)`
signifies an atomic increment.  

These are the two histories where the lock will consider the read to have taken place in a deterministic way:

![happy path 1](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/happy1.png)

![happy path 2](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/happy2.png)

And the following are the histories where the read value is unknown, and where the lock would then require a retry:

![sad path 1](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/rs_ws_re_we.png)

![sad path 1](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/rs_ws_we_re.png)

![sad path 1](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/ws_rs_re_we.png)

![sad path 1](https://raw.githubusercontent.com/jakewins/seqmut/master/histories/ws_rs_we_re.png)

## Performance

In uncontended reads, the sequence lock performs two fenced reads. This is compared to the regular Go RWLock which performs two atomic writes.

Hence, the sequence lock is substantially faster:

```
$ go test -bench=.
goos: linux
goarch: amd64
pkg: seqmut
BenchmarkUncontendedRWMutex-4       	2000000000	         0.91 ns/op
BenchmarkContendedRWMutex-4         	500000000	         2.92 ns/op
BenchmarkUncontendedSyncRWMutex-4   	50000000	        27.5 ns/op
BenchmarkContendedSyncRWMutex-4     	50000000	        28.4 ns/op
```

## Use cases

For use cases like implementing disk page caches, the locks guarding pages become bottle necks if the working set fits in RAM.
Each cache line read takes in the order of 1-100ns depending on where it is in caches, while the read lock takes in the order of 20ns.

Hence, in the "best" case, 100ns is main memory fetch, 20ns is mutex shenanigans.

Sequence locks have overheads closer to 1ns, which is significant here.


