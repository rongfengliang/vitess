// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package barnacle

import (
	"math/rand"
	"sync"
	"time"

	log "github.com/golang/glog"
)

var (
	RETRY_DELAY = time.Duration(1 * time.Second)
)

type GetAddressesFunc func() ([]string, error)

// Balancer is a simple round-robin load balancer.
// It allows you to temporarily mark down nodes that
// are non-functional.
type Balancer struct {
	mu           sync.Mutex
	addressNodes []*addressStatus
	index        int
	getAddresses GetAddressesFunc
}

type addressStatus struct {
	Address  string
	timeDown time.Time
	balancer *Balancer
}

// NewBalancer creates a Balancer. getAddreses is the function
// it will use to refresh the list of addresses if one of the
// nodes has been marked down. The list of addresses is shuffled.
func NewBalancer(getAddresses GetAddressesFunc) *Balancer {
	blc := new(Balancer)
	blc.getAddresses = getAddresses
	blc.refresh()
	return blc
}

// Get returns a single address that was not recently marked down.
// If it finds an address that was down for longer than RETRY_DELAY,
// it refreshes the list of addresses and returns the next available
// node.
func (blc *Balancer) Get() (address string) {
	blc.mu.Lock()
	defer blc.mu.Unlock()
	if len(blc.addressNodes) == 0 {
		return ""
	}
	for _ = range blc.addressNodes {
		blc.index = (blc.index + 1) % len(blc.addressNodes)
		addrNode := blc.addressNodes[blc.index]
		if addrNode.timeDown.IsZero() {
			return addrNode.Address
		}
		if time.Now().Sub(addrNode.timeDown) > RETRY_DELAY {
			addrNode.timeDown = time.Time{}
			blc.refresh()
			return addrNode.Address
		}
	}
	return ""
}

// MarkDown marks the specified address down. Such addresses
// will not be used by Balancer for the duration of RETRY_DELAY.
func (blc *Balancer) MarkDown(address string) {
	blc.mu.Lock()
	defer blc.mu.Unlock()
	if index := findAddrNode(blc.addressNodes, address); index != -1 {
		blc.addressNodes[index].timeDown = time.Now()
	}
}

// Refresh forces a refresh. All mark down flags for nodes are
// cleared. The address order is shuffled after the update.
func (blc *Balancer) Refresh() {
	blc.mu.Lock()
	defer blc.mu.Unlock()
	blc.refresh()
	for _, addrNode := range blc.addressNodes {
		addrNode.timeDown = time.Time{}
	}
}

func (blc *Balancer) refresh() {
	addresses, err := blc.getAddresses()
	if err != nil {
		log.Errorf("%v", err)
		return
	}
	// Add new addressNodes
	for _, address := range addresses {
		if index := findAddrNode(blc.addressNodes, address); index == -1 {
			addrNode := &addressStatus{Address: address, balancer: blc}
			blc.addressNodes = append(blc.addressNodes, addrNode)
		}
	}
	// Remove those that went away
	i := 0
	for i < len(blc.addressNodes) {
		if index := findAddress(addresses, blc.addressNodes[i].Address); index == -1 {
			blc.addressNodes = delAddrNode(blc.addressNodes, i)
			continue
		}
		i++
	}
	shuffle(blc.addressNodes)
}

func findAddrNode(addressNodes []*addressStatus, address string) (index int) {
	for i, addrNode := range addressNodes {
		if address == addrNode.Address {
			return i
		}
	}
	return -1
}

func findAddress(addresses []string, address string) (index int) {
	for i, addr := range addresses {
		if address == addr {
			return i
		}
	}
	return -1
}

func delAddrNode(addressNodes []*addressStatus, index int) []*addressStatus {
	copy(addressNodes[index:len(addressNodes)-1], addressNodes[index+1:])
	return addressNodes[:len(addressNodes)-1]
}

// shuffle uses the Fisher-Yates algorithm.
func shuffle(addressNodes []*addressStatus) {
	index := 0
	for i := len(addressNodes) - 1; i > 0; i-- {
		index = int(rand.Int63()) % (i + 1)
		addressNodes[i], addressNodes[index] = addressNodes[index], addressNodes[i]
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
