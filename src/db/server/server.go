/*
Server.go

The main API class and application entrypoint
All functions here (except main) correspond to API calls that the db host supports
*/

package main

import (
	"db/counter"
	"db/msg"
	"fmt"
	"github.com/go-martini/martini"
	"net"
	"net/http"
	"strconv"
)

var actors []string

var counters map[string]*counter.Counter

var thisHostNum int

func setConfig(req *http.Request) (int, string) {

	configMsg := msg.BuildConfigMsg(req.Body)

	actors = configMsg.Actors

	interfaces, _ := net.Interfaces()
	foundLocalIP := false

	var allAddrs []string
	for _, iface := range interfaces {
		if iface.Name == "lo0" {
			continue
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			allAddrs = append(allAddrs, ip.String())

			for actorIndex, actorAddr := range actors {
				if actorAddr == ip.String() {
					thisHostNum = actorIndex
					foundLocalIP = true
				}
				if foundLocalIP {
					break
				}

			}
			if foundLocalIP {
				break
			}

		}
		if foundLocalIP {
			break
		}
	}

	if !foundLocalIP {
		return 400, fmt.Sprintf("None of this Host's IPs: %+v were found in actors list", allAddrs)
	}

	fmt.Printf("Config received:\n  %+v\n", actors)
	fmt.Printf("  This Host Index: %d\n", thisHostNum)
	fmt.Printf("  Local Address: %s\n", actors[thisHostNum])

	return 200, "OK"

}

func getLocalCounterValue(params martini.Params) (int, string) {
	if len(actors) == 0 {
		return 412, "This Host has not yet been configured"
	}

	name := params["name"]

	if ctr, ok := counters[name]; ok {
		val := strconv.Itoa(ctr.GetLocalValue())
		fmt.Printf("fetching local counter value - %s: %s\n", name, val)
		return 200, val
	} else {
		return 404, "no counter by that name could be found on this host"
	}
}

func getGlobalCounterValue(params martini.Params) (int, string) {
	if len(actors) == 0 {
		return 412, "This Host has not yet been configured"
	}
	name := params["name"]

	if ctr, ok := counters[name]; ok {
		val := strconv.Itoa(ctr.GetGlobalValue())
		fmt.Printf("fetching global counter value - %s: %d", name, val)
		return 200, strconv.Itoa(ctr.GetGlobalValue())
	} else {
		return 404, "no counter by that name could be found on this host"
	}

}

func addToCounter(params martini.Params, req *http.Request) (int, string) {
	if len(actors) == 0 {
		return 412, "This Host has not yet been configured"
	}

	name := params["name"]

	addMsg := msg.BuildAddMsg(req.Body)

	_, ok := counters[name]
	if !ok {
		fmt.Printf("Counter %s not found. Creating a new one.\n", name)
		counters[name] = counter.SpawnCounter(name, actors, thisHostNum)
	}

	ctr, _ := counters[name]

	fmt.Printf("Adding to counter '%s': %d\n", name, addMsg.AddValue)
	ctr.Add(addMsg.AddValue)

	go ctr.PushToAllNeighbors()

	return 200, "OK"
}

func getSync(params martini.Params, req *http.Request) (int, string) {
	if len(actors) == 0 {
		return 412, "This Host has not yet been configured"
	}

	name := params["name"]
	if _, ok := counters[name]; ok {
		fmt.Printf("fetching local sync state for %s:\n  %s\n", name, counters[name].Serialize())

	} else {
		fmt.Printf("no counter named %s, spwaning a new one\n", name)
		counters[name] = counter.SpawnCounter(name, actors, thisHostNum)

	}

	return 200, counters[name].Serialize()
}

func setSync(params martini.Params, req *http.Request) (int, string) {
	if len(actors) == 0 {
		return 412, "This Host has not yet been configured"
	}

	name := params["name"]

	_, ok := counters[name]
	if !ok {
		fmt.Printf("Counter %s not found. Creating a new one.\n", name)
		counters[name] = counter.SpawnCounter(name, actors, thisHostNum)

	}
	ctr, _ := counters[name]

	syncMsg := msg.BuildSyncMsg(req.Body)
	fmt.Printf("syncing with the following remote state:\n  %+v\n", syncMsg)

	ctr.UpdateFromMsg(syncMsg)

	return 200, "OK"
}

func main() {

	counters = make(map[string]*counter.Counter)
	r := martini.Classic()

	r.Post(`/config`, setConfig)
	r.Post(`/counter/:name`, addToCounter)
	r.Get(`/counter/:name/value`, getLocalCounterValue)
	r.Get(`/counter/:name/consistent_value`, getGlobalCounterValue)

	r.Get(`/counter/:name/getsync`, getSync)
	r.Post(`/counter/:name/setsync`, setSync)

	r.RunOnAddr(":7777")
}
