// This is a simple pubsub implementation. Messages can be of any type, and all
// channels can only have one publisher. The publishing routine naively loops
// through all subscribed channels and pushes the message to them, with the
// message being dropped for any channels which block. Users worried about any
// subscribing channels blocking should use buffered channels for them.
// This implementation has the following properties:
//
// * Publishing routine naively loops through subsribed channels. Any which
//   block will have the message not sent to them. It is recommended you give
//   subscribed channels a small buffer.
//
// * A channel can be subscribed to before it is published to, and vice-versa
//
// * Channels are destroyed by calling ClosePubCh. All subscribed channels will
//   be automatically closed when this happens
//
package pubsub

import (
	"log"
	"sync"
	"time"
)

type pubSub struct {
	addDst chan chan interface{}
	remDst chan chan interface{}
	src    chan interface{}
	dsts   map[chan interface{}]struct{}

	// These are only accessed/modified by the PubSubMux managing this pubSub

	srcGotten bool
}

func newPubSub() *pubSub {
	p := &pubSub{
		addDst: make(chan chan interface{}),
		remDst: make(chan chan interface{}),
		src:    make(chan interface{}),
		dsts:   map[chan interface{}]struct{}{},
	}
	go p.spin()
	return p
}

func (p *pubSub) spin() {
outerloop:
	for {
		select {
		case ch := <-p.addDst:
			p.dsts[ch] = struct{}{}
		case ch := <-p.remDst:
			delete(p.dsts, ch)
		case msg, ok := <-p.src:
			if !ok {
				break outerloop
			}
			for ch := range p.dsts {
				select {
				case ch <- msg:
				default:
					log.Printf("pubSub dropping message to channel %v", msg, ch)
				}
			}
		}
	}

	for ch := range p.dsts {
		close(ch)
	}
}

// All publishing and subscribing happens through channels, but this struct is
// necessary so subscribers can find the publisher go-routines. Can be used by
// multiple routines at once.
type PubSubMux struct {
	mux        map[string]*pubSub
	muxLock    sync.Mutex
	pubTimeout time.Duration
}

func NewPubSubMux() *PubSubMux {
	return &PubSubMux{
		mux: map[string]*pubSub{},
	}
}

// Returns a new channel which can be published to for the given channel.
//
// Also returns a boolean which will be true if GetPubCh has been called on this
// channel already (although this gets reset when the channel gets cleaned up)
func (pmux *PubSubMux) GetPubCh(channel string) (chan<- interface{}, bool) {
	pmux.muxLock.Lock()
	defer pmux.muxLock.Unlock()

	p, ok := pmux.mux[channel]
	if !ok {
		p = newPubSub()
		pmux.mux[channel] = p
	}

	srcGotten := p.srcGotten
	p.srcGotten = true
	return p.src, srcGotten
}

// Calls close on the publish channel. Will close all the subscribed channels
// and cleanup the routine managing the channel. Returns whether there was
// actually a channel of the given name in existence
func (pmux *PubSubMux) ClosePubCh(channel string) bool {
	pmux.muxLock.Lock()
	defer pmux.muxLock.Unlock()

	p, ok := pmux.mux[channel]
	if !ok {
		return false
	}

	close(p.src)
	delete(pmux.mux, channel)
	return true
}

// Subscribes the given subscription channel to the given channel. Creates the
// routine for that channel if it didn't already exist
func (pmux *PubSubMux) AddSubCh(channel string, ch chan interface{}) {
	pmux.muxLock.Lock()
	defer pmux.muxLock.Unlock()

	p, ok := pmux.mux[channel]
	if !ok {
		p = newPubSub()
		pmux.mux[channel] = p
	}
	select {
	case p.addDst <- ch:
	case <-time.After(1 * time.Second):
		log.Printf("pubSub timedout writing to addDst")
	}
}

// Unsubsribes the given subscription channel from the given channel. Does NOT
// create the routine for that channel if it didn't already exist
func (pmux *PubSubMux) RemSubCh(channel string, ch chan interface{}) {
	pmux.muxLock.Lock()
	defer pmux.muxLock.Unlock()

	p, ok := pmux.mux[channel]
	if !ok {
		return
	}
	select {
	case p.remDst <- ch:
	case <-time.After(1 * time.Second):
		log.Printf("pubSub timedout writing to remDst")
	}
}
