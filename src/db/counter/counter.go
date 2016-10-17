/**
Counter.go

Model for the counter itself

this file has functions to create, and update the counter,
as well as methods to synchronize the counter values over the network

*/
package counter

import (
	"bytes"
	"db/msg"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Counter struct {
	name   string
	hostId int
	actors []string

	markedForPush []int
	pushLocks     []sync.Mutex

	markedForPull []int
	pullLocks     []sync.Mutex

	CounterValues []int
	cvLocks       []sync.Mutex
}

/**
Serialize the counter values in this counter to a string
used for building sychronization messages
*/
func (this *Counter) Serialize() string {

	var msg msg.SyncMsg
	msg.CounterValues = this.CounterValues

	byteArr, _ := json.Marshal(msg)

	return string(byteArr)
}

/**
Spawn a new counter -- launches a new thread to continuously poll neighbors
*/
func SpawnCounter(name string, actors []string, hostId int) *Counter {
	var count *Counter
	count = new(Counter)

	count.name = name
	count.actors = actors
	count.hostId = hostId

	numActors := len(actors)
	count.CounterValues = make([]int, numActors)
	count.cvLocks = make([]sync.Mutex, numActors)
	count.markedForPush = make([]int, numActors)
	count.pushLocks = make([]sync.Mutex, numActors)
	count.markedForPull = make([]int, numActors)
	count.pullLocks = make([]sync.Mutex, numActors)

	for i := range count.CounterValues {
		count.CounterValues[i] = 0
	}

	go count.PollNeighbors()

	return count
}

/**
Update this counter's global state copies from a synchronization message
*/
func (this *Counter) UpdateFromMsg(msg msg.SyncMsg) {

	for i, k := range msg.CounterValues {
		if this.CounterValues[i] < k {
			this.cvLocks[i].Lock()
			if this.CounterValues[i] < k {
				this.CounterValues[i] = k
			}
			this.cvLocks[i].Unlock()
		}
	}

	return
}

/**
increment the counter by a given amount
*/
func (this *Counter) Add(value int) {

	this.cvLocks[this.hostId].Lock()
	this.CounterValues[this.hostId] += value

	this.cvLocks[this.hostId].Unlock()

	return
}

/**
Compute an return a counter value based on local copies of partial-counters only.
*/
func (this *Counter) GetLocalValue() int {

	accumulator := 0
	for _, val := range this.CounterValues {
		this.cvLocks[this.hostId].Lock()
		accumulator += val
		this.cvLocks[this.hostId].Unlock()
	}

	return accumulator
}

/**
Return a globally consistent value for the counter. Blocks on being able
to synchronize with all neighboring hosts.
May block indefinitely if not all hosts can be reached.

*/
func (this *Counter) GetGlobalValue() int {

	accumulator := 0

	this.FullSyncFromAllNeighbors()

	for _, k := range this.CounterValues {
		this.cvLocks[this.hostId].Lock()
		accumulator += k
		this.cvLocks[this.hostId].Unlock()
	}

	return accumulator

}

/**
Spawn a set of threads, each of which blocks on pushing its update to a neighbor
(with backoff, as discussed below)
This is called whenever a counter is locally updated via an API call.
*/
func (this *Counter) PushToAllNeighbors() {

	for i, _ := range this.actors {
		go this.syncToNeighborSingletonWithBackoff(i)
	}

	return
}

/**
Continously poll all neighbors for new sync data
Every counter will have a thread of this polling loop running continuously.

Neighbor syncs are in singleton threads to avoid contention and redundant sync threads.
*/
func (this *Counter) PollNeighbors() {

	for {
		for i, _ := range this.actors {
			go this.syncFromNeighborSingleton(i)
		}
		time.Sleep(time.Duration(250 + rand.Intn(500)))

	}

	return
}

/**
Get a full sync from all neighbor hosts. Block until the sync returns successfully for
each neighbor host.

This is run before the "consistent_value" call to get the consistent state onto a single host.

This function will block indefinitely if the network stays partitioned
*/
func (this *Counter) FullSyncFromAllNeighbors() {
	sem := make(chan int, len(this.actors))

	for i, _ := range this.actors {
		go func(i int, ctr *Counter) {
			for {
				if err := ctr.syncFromNeighbor(i); err == nil {
					sem <- 0
					break
				} else {
					time.Sleep(time.Duration(50 + rand.Intn(100)))
				}
			}
		}(i, this)
	}
	for i := 0; i < len(this.actors); i++ {
		<-sem
	}

	return
}

/**
Repeatedly attempt to sync data to a neighboring host until successful

because this process might run continuously for some time, it runs as a singleton thread. Any new threads created to sync to the same neighbor will just return immediately

Also, repeated sync failures lead to a randomized exponential backoff that prevents
the thread from consuming too many resources in the in case sustained network partitioning. Also, the randomized backup will help with possible request contention issues.
*/
func (this *Counter) syncToNeighborSingletonWithBackoff(actorNum int) error {
	this.pushLocks[actorNum].Lock()
	if this.markedForPush[actorNum] == 1 {
		this.pushLocks[actorNum].Unlock()
		return nil
	} else {
		this.markedForPush[actorNum] = 1
		this.pushLocks[actorNum].Unlock()
	}

	backoff := 10
	for {
		if err := this.syncToNeighbor(actorNum); err == nil {
			this.pushLocks[actorNum].Lock()
			this.markedForPush[actorNum] = 0
			this.pushLocks[actorNum].Unlock()
			break
		} else {
			time.Sleep(time.Duration(backoff + rand.Intn(backoff/2)))
			if backoff < 3000 {
				backoff *= 2
			}

		}
	}

	return nil
}

/**
Make a single attempt to push local counter data to a neighbor host

if the attempt fails, just return an error
*/
func (this *Counter) syncToNeighbor(actorNum int) error {
	url := fmt.Sprintf("http://%s:7777/counter/%s/setsync", this.actors[actorNum], this.name)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(this.Serialize())))
	req.Header.Set("Connection", "close")
	req.Close = true

	var client http.Client

	resp, err := client.Do(req)
	if err != nil {
		return err
	} else if resp.StatusCode != 200 {
		resp.Body.Close()
		return errors.New(resp.Status)
	} else {
		resp.Body.Close()
		return nil
	}

}

/**
Make a single attempt to pull in a data sync from a neighbor host,
unless a sync to the same host is already in progress.

if a sync is already in progress in another thread, just return without doing anything

*/
func (this *Counter) syncFromNeighborSingleton(actorNum int) error {
	this.pullLocks[actorNum].Lock()
	if this.markedForPush[actorNum] == 1 {
		this.pullLocks[actorNum].Unlock()
		return nil
	} else {
		this.markedForPush[actorNum] = 1
		this.pullLocks[actorNum].Unlock()

		err := this.syncFromNeighbor(actorNum)
		this.pullLocks[actorNum].Lock()
		this.markedForPush[actorNum] = 0
		this.pullLocks[actorNum].Unlock()
		return err

	}

}

/**
Make a single attempt to pull in a data sync from a neighbor host.
if this fails, just return an error
*/
func (this *Counter) syncFromNeighbor(actorNum int) error {
	url := fmt.Sprintf("http://%s:7777/counter/%s/getsync", this.actors[actorNum], this.name)
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Connection", "close")
	req.Close = true

	var client http.Client

	resp, err := client.Do(req)

	if err != nil {
		return err
	} else if resp.StatusCode != 200 {
		resp.Body.Close()
		return errors.New(resp.Status)
	} else {
		msg := msg.BuildSyncMsg(resp.Body)
		fmt.Printf("Received sync from Neighbor:\n  %+v\n", msg.CounterValues)
		this.UpdateFromMsg(msg)
		resp.Body.Close()
		return nil
	}
}
