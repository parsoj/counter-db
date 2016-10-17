# Distributed Counter Database

## Description
   
   A Distributed database for increment-only counters
   

## Setup Requirements

    This app is written in Go-lang version 1.6. All libraries are included in the challenge directory, and the only other setup requirement is the environment variable "GOPATH" is set to point to the "challenge" directory. 
    To run the app:
    $ go run $GOPATH/src/db/server/server.go
    
    Alternatively, you can just use the included Dockerfile

## API 

### Supported calls
#### Configure -- POST to /config

    Accepts a congfiguration message of the form:
    {"actors": ["1.2.3.4", "1.2.3.5", "1.2.3.6"]} 
    
    Representing all of the hosts that are a part of the DB. 
    Each host will parse this list of actors, and determine its own 
    actor number based on the ordering of the list. So, all hosts must receive the 
    same config, with the same ip addresses in the same order. 

#### Add -- POST to /counter/<name>

    Increments the named counter on this host, and trigger a sync of that counter to other hosts. If neighboring hosts are currently unreachable for a sync, then the synchro process will use exponential backoff up to a maximum number of retries 

#### Get Local Counter Value -- GET from /counter/<name>/value

    Returns the local counter value for the named counter. This should be fast, but it will not neccesarily reflect the consistent state of the database.

#### Get Global Counter Value -- GET from /counter/<name>/consistent_value

    Functions similarly to "get local counter value", but blocks on dispatching sync requests to all neighboring hosts. If all hosts do not return a 200 response, then this request will keep blocking

#### Get Sync Data -- GET from /counter/<name>/getsync

    A "private" API call for getting sync data from a host. This is used as part of 
    regular polling that all hosts do to keep synchronized to their neighbors


#### Post Sync Data -- POST to /counter/<name>/setsync

    A "private" API call for pushing sync data to a host. This is used by any host that receives a counter update and wants to tell its neighbors
  

## High-Level Algorithm

    As was hinted in the problem document, perfect consistency is hard given the constraints required. 
    The CAP theorem states that we can't have perfect:
    * consistency
    * availability
    * partition tolerance
    All at the same time. In the problem document, requirements for partition tolerance and availability are laid out, so we are going to have to sacrifice on consistency to make this work. 
    
    Because this is an "increment only" counter, we can take some shortcuts in keeping things synchronized in a stable fashion. Namely, I don't need to implement PAXOS here, since updates are always "conflict free". i.e. you can always just compare messages to your existing state, and take the maximum counter value of the two. 
    
    To get more into details, I'll talk about the implementation for a single synchronized counter.
    So, for each counter stored in the DB:
    Each host keeps its own "partial-counter" which holds only the counter values that have been directly added through its API. The total value for a given distributed counter is thus the sum of all of these partial counters across all hosts. 
    To maintain availability, each host also keeps a copy all other neighboring partial-counters. Every host periodically polls all other hosts to try to keep up to date values for it's neighbors partial counters. This is where we can take a shortcut, and not worry about out-of-date update messages squashing our current values. Because the counter is increment only, every host can ignore partial-counter updates that are not greater than the current copy it has. 
    
    When a host receives an API call to add to its counter, it will attempt to notify all its neighboring counters. 
    
    So, in summary, each host keeps a partial-counter, as well as copies of all its neighboring host's partial-counters. It continuously polls neighboring hosts for updates/increments to their partial-counters, and tries to notify those hosts when it receives an update/increment itself. 
    
    I did both continuous polling, and pushing-on-update to help improve consistency a bit. Pushing on update helps synchronize updates more quickly than polling might. However continuous polling can help with the case where a host pushes out its update to only some of its neighbors before dying/getting partitioned out. With continous polling, if a host can get its update out to at least one other host (that doen't also die or get partitioned out) then that update will still eventually get synchronized to the remaining neighbors it it connected to. 
    
## Testing Performed

    This was tested with "blockade" - a docker-based utility for testing network failures and partitions:
    https://github.com/dcm-oss/blockade


## What I left out
### Service Discovery

  A killed process on one host will need a new configuration if restarted. I did not implement any sort of config-synchronization behavior to allow a new host to be added to an existing fleet without another config being sent. 
  However, if a host is killed, then replaced and given the same config and IP, then it will synchronize with the other hosts and effectively replace the dead host. 
  If a new IP address is desired for a new host, all hosts will need to receive the new config
  
### Persistence to disk

   Using an SQLITE database or file-based storage would have allowed a host to persist to disk, and restart itself from data stored on disk. This would be important in the case of process death on a partitioned host that has unsynchronized data. However since the overall synchronization algorithm is implemented - I left this out for the sake of simplicity and time. 

  
  
  


## TODO
COMMENTS

BIN directory stuff
