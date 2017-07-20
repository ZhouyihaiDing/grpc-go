/*
 *
 * Copyright 2016 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package grpc

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/naming"
	"google.golang.org/grpc/grpclog"
)

type testWatcher struct {
	// the channel to receives name resolution updates
	update chan *naming.Update
	// the side channel to get to know how many updates in a batch
	side chan int
	// the channel to notifiy update injector that the update reading is done
	readDone chan int
}

func (w *testWatcher) Next() (updates []*naming.Update, err error) {
	n := <-w.side
	if n == 0 {
		return nil, fmt.Errorf("w.side is closed")
	}
	for i := 0; i < n; i++ {
		u := <-w.update
		if u != nil {
			updates = append(updates, u)
		}
	}
	w.readDone <- 0
	return
}

func (w *testWatcher) Close() {
}

// Inject naming resolution updates to the testWatcher.
func (w *testWatcher) inject(updates []*naming.Update) {
	w.side <- len(updates)
	for _, u := range updates {
		w.update <- u
	}
	<-w.readDone
}

type testNameResolver struct {
	w    *testWatcher
	addr string
}

func (r *testNameResolver) Resolve(target string) (naming.Watcher, error) {
	r.w = &testWatcher{
		update:   make(chan *naming.Update, 1),
		side:     make(chan int, 1),
		readDone: make(chan int),
	}
	r.w.side <- 1
	r.w.update <- &naming.Update{
		Op:   naming.Add,
		Addr: r.addr,
	}
	go func() {
		<-r.w.readDone
	}()
	return r.w, nil
}

func startServers(t *testing.T, numServers int, maxStreams uint32) ([]*server, *testNameResolver) {
	var servers []*server
	for i := 0; i < numServers; i++ {
		s := newTestServer()
		servers = append(servers, s)
		go s.start(t, 0, maxStreams)
		s.wait(t, 2*time.Second)
	}
	// Point to server[0]
	addr := "localhost:" + servers[0].port
	return servers, &testNameResolver{
		addr: addr,
	}
}

func TestNameDiscovery(t *testing.T) {
	// Start 2 servers on 2 ports.
	numServers := 2
	servers, r := startServers(t, numServers, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	req := "port"
	var reply string
	if err := Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err == nil || ErrorDesc(err) != servers[0].port {
		t.Fatalf("grpc.Invoke(_, _, _, _, _) = %v, want %s", err, servers[0].port)
	}
	// Inject the name resolution change to remove servers[0] and add servers[1].
	var updates []*naming.Update
	updates = append(updates, &naming.Update{
		Op:   naming.Delete,
		Addr: "localhost:" + servers[0].port,
	})
	updates = append(updates, &naming.Update{
		Op:   naming.Add,
		Addr: "localhost:" + servers[1].port,
	})
	r.w.inject(updates)
	// Loop until the rpcs in flight talks to servers[1].
	for {
		if err := Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err != nil && ErrorDesc(err) == servers[1].port {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cc.Close()
	for i := 0; i < numServers; i++ {
		servers[i].stop()
	}
}

func TestEmptyAddrs(t *testing.T) {
	servers, r := startServers(t, 1, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	var reply string
	if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc); err != nil || reply != expectedResponse {
		t.Fatalf("grpc.Invoke(_, _, _, _, _) = %v, reply = %q, want %q, <nil>", err, reply, expectedResponse)
	}
	// Inject name resolution change to remove the server so that there is no address
	// available after that.
	u := &naming.Update{
		Op:   naming.Delete,
		Addr: "localhost:" + servers[0].port,
	}
	r.w.inject([]*naming.Update{u})
	// Loop until the above updates apply.
	for {
		time.Sleep(10 * time.Millisecond)
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
		if err := Invoke(ctx, "/foo/bar", &expectedRequest, &reply, cc); err != nil {
			break
		}
	}
	cc.Close()
	servers[0].stop()
}

func TestRoundRobin(t *testing.T) {
	// Start 3 servers on 3 ports.
	numServers := 3
	servers, r := startServers(t, numServers, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	// Add servers[1] to the service discovery.
	fmt.Println("I")
	u := &naming.Update{
		Op:   naming.Add,
		Addr: "localhost:" + servers[1].port,
	}
	r.w.inject([]*naming.Update{u})
	req := "port"
	var reply string
	// Loop until servers[1] is up
	for {
		fmt.Println("Invoke:")
		var err1 error
		if err1 = Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err1 != nil && ErrorDesc(err1) == servers[1].port {
			break
		}
		fmt.Println(err1, ErrorDesc(err1), " ", servers[1].port)
		time.Sleep(1 * time.Second)
	}
	// Add server2[2] to the service discovery.
	fmt.Println("II")
	u = &naming.Update{
		Op:   naming.Add,
		Addr: "localhost:" + servers[2].port,
	}
	r.w.inject([]*naming.Update{u})
	// Loop until both servers[2] are up.
	for {
		if err := Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err != nil && ErrorDesc(err) == servers[2].port {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Check the incoming RPCs served in a round-robin manner.
	fmt.Println("III")
	for i := 0; i < 10; i++ {
		if err := Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err == nil || ErrorDesc(err) != servers[i%numServers].port {
			t.Fatalf("Index %d: Invoke(_, _, _, _, _) = %v, want %s", i, err, servers[i%numServers].port)
		}
	}
	cc.Close()
	for i := 0; i < numServers; i++ {
		servers[i].stop()
	}
}

func TestCloseWithPendingRPC(t *testing.T) {
	servers, r := startServers(t, 1, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}

	var reply string
	if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); err != nil {
		t.Fatalf("grpc.Invoke(_, _, _, _, _) = %v, want %s", err, servers[0].port)
	}
	// Remove the server.
	updates := []*naming.Update{{
		Op:   naming.Delete,
		Addr: "localhost:" + servers[0].port,
	}}
	r.w.inject(updates)
	// Loop until the above update applies.
	for {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
		if err := Invoke(ctx, "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); Code(err) == codes.DeadlineExceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Issue 2 RPCs which should be completed with error status once cc is closed.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var reply string
		if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); err == nil {
			t.Errorf("grpc.Invoke(_, _, _, _, _) = %v, want not nil", err)
		}
	}()
	go func() {
		defer wg.Done()
		var reply string
		time.Sleep(5 * time.Millisecond)
		if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); err == nil {
			t.Errorf("grpc.Invoke(_, _, _, _, _) = %v, want not nil", err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	cc.Close()
	wg.Wait()
	servers[0].stop()

}

func TestGetOnWaitChannel(t *testing.T) {
	servers, r := startServers(t, 1, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	// Remove all servers so that all upcoming RPCs will block on waitCh.
	updates := []*naming.Update{{
		Op:   naming.Delete,
		Addr: "localhost:" + servers[0].port,
	}}
	r.w.inject(updates)
	for {
		var reply string
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Millisecond)
		if err := Invoke(ctx, "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); Code(err) == codes.DeadlineExceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var reply string
		if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); err != nil {
			t.Errorf("grpc.Invoke(_, _, _, _, _) = %v, want <nil>", err)
		}
	}()
	// Add a connected server to get the above RPC through.
	updates = []*naming.Update{{
		Op:   naming.Add,
		Addr: "localhost:" + servers[0].port,
	}}
	r.w.inject(updates)
	// Wait until the above RPC succeeds.
	wg.Wait()
	cc.Close()
	servers[0].stop()
}

func TestOneServerDown(t *testing.T) {
	// Start 2 servers.
	numServers := 2
	servers, r := startServers(t, numServers, math.MaxUint32)
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	// Add servers[1] to the service discovery.
	var updates []*naming.Update
	updates = append(updates, &naming.Update{
		Op:   naming.Add,
		Addr: "localhost:" + servers[1].port,
	})
	r.w.inject(updates)
	req := "port"
	var reply string
	// Loop until servers[1] is up
	for {
		if err := Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err != nil && ErrorDesc(err) == servers[1].port {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	var wg sync.WaitGroup
	numRPC := 100
	sleepDuration := 10 * time.Millisecond
	wg.Add(1)
	go func() {
		time.Sleep(sleepDuration)
		// After sleepDuration, kill server[0].
		servers[0].stop()
		wg.Done()
	}()

	// All non-failfast RPCs should not block because there's at least one connection available.
	for i := 0; i < numRPC; i++ {
		wg.Add(1)
		go func() {
			time.Sleep(sleepDuration)
			// After sleepDuration, invoke RPC.
			// server[0] is killed around the same time to make it racy between balancer and gRPC internals.
			Invoke(context.Background(), "/foo/bar", &req, &reply, cc, FailFast(false))
			wg.Done()
		}()
	}
	wg.Wait()
	cc.Close()
	for i := 0; i < numServers; i++ {
		servers[i].stop()
	}
}

func TestOneAddressRemoval(t *testing.T) {
	// Start 2 servers.
	fmt.Println("begin")
	numServers := 2
	servers, r := startServers(t, numServers, math.MaxUint32)
	fmt.Println("Dialllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllll")
	cc, err := Dial("foo.bar.com", WithBalancer(RoundRobin(r)), WithBlock(), WithInsecure(), WithCodec(testCodec{}))
	if err != nil {
		t.Fatalf("Failed to create ClientConn: %v", err)
	}
	fmt.Println("before updateeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	// Add servers[1] to the service discovery.
	var updates []*naming.Update
	updates = append(updates, &naming.Update{
		Op:   naming.Add,
		Addr: "localhost:" + servers[1].port,
	})
	r.w.inject(updates)
	req := "port"
	var reply string
	// Loop until servers[1] is up
	for {
		var err1 error
		fmt.Println("Inovokeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
		if err1 = Invoke(context.Background(), "/foo/bar", &req, &reply, cc); err1 != nil && ErrorDesc(err1) == servers[1].port {
			break
		}
		fmt.Println(err1, ErrorDesc(err1), servers[1].port)
		time.Sleep(1 * time.Second)
	}
	fmt.Println("end err1 ======================================================")

	var wg sync.WaitGroup
	numRPC := 100
	sleepDuration := 10 * time.Millisecond
	wg.Add(1)
	go func() {
		time.Sleep(sleepDuration)
		// After sleepDuration, delete server[0].
		var updates []*naming.Update
		updates = append(updates, &naming.Update{
			Op:   naming.Delete,
			Addr: "localhost:" + servers[0].port,
		})
		r.w.inject(updates)
		wg.Done()
	}()

	// All non-failfast RPCs should not fail because there's at least one connection available.
	for i := 0; i < numRPC; i++ {
		wg.Add(1)
		go func() {
			var reply string
			time.Sleep(sleepDuration)
			// After sleepDuration, invoke RPC.
			// server[0] is removed around the same time to make it racy between balancer and gRPC internals.
			if err := Invoke(context.Background(), "/foo/bar", &expectedRequest, &reply, cc, FailFast(false)); err != nil {
				t.Errorf("grpc.Invoke(_, _, _, _, _) = %v, want not nil", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	fmt.Println("after wait")
	cc.Close()
	for i := 0; i < numServers; i++ {
		servers[i].stop()
	}
}

type firstFind struct {
	rr			*roundRobin
}

// FirstFind returns a Balancer is a simple balancer for multi-addresses in one addrConn. It uses FirstFindMD as
// Address.Metadata so that lbWatcher knows its firstfind(addrConns has multi address) or roundrobin.
func FirstFind(r naming.Resolver) Balancer {
	return &firstFind{rr: &roundRobin{r: r}}
}

// The only difference is change Metadata type to FirstFindMD
func (ff *firstFind) watchAddrUpdates() error {
	updates, err := ff.rr.w.Next()
	if err != nil {
		grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
		return err
	}
	ff.rr.mu.Lock()
	defer ff.rr.mu.Unlock()
	for _, update := range updates {
		addr := Address{
			Addr:     update.Addr,
			Metadata: FirstFindMD{update.Metadata},
		}
		switch update.Op {
		case naming.Add:
			var exist bool
			for _, v := range ff.rr.addrs {
				if addr == v.addr {
					exist = true
					grpclog.Infoln("grpc: The name resolver wanted to add an existing address: ", addr)
					break
				}
			}
			if exist {
				continue
			}
			ff.rr.addrs = append(ff.rr.addrs, &addrInfo{addr: addr})
		case naming.Delete:
			for i, v := range ff.rr.addrs {
				if addr == v.addr {
					copy(ff.rr.addrs[i:], ff.rr.addrs[i+1:])
					ff.rr.addrs = ff.rr.addrs[:len(ff.rr.addrs)-1]
					break
				}
			}
		default:
			grpclog.Errorln("Unknown update.Op ", update.Op)
		}
	}
	// Make a copy of rr.addrs and write it onto rr.addrCh so that gRPC internals gets notified.
	open := make([]Address, len(ff.rr.addrs))
	for i, v := range ff.rr.addrs {
		open[i] = v.addr
	}
	if ff.rr.done {
		return ErrClientConnClosing
	}
	select {
	case <-ff.rr.addrCh:
	default:
	}
	ff.rr.addrCh <- open
	return nil
}

// The only difference is using ff.watchAddrUpdates() to use findFirstMD
func (ff *firstFind) Start(target string, config BalancerConfig) error {
	ff.rr.mu.Lock()
	defer ff.rr.mu.Unlock()
	if ff.rr.done {
		return ErrClientConnClosing
	}
	if ff.rr.r == nil {
		// If there is no name resolver installed, it is not needed to
		// do name resolution. In this case, target is added into rr.addrs
		// as the only address available and rr.addrCh stays nil.
		ff.rr.addrs = append(ff.rr.addrs, &addrInfo{addr: Address{Addr: target}})
		return nil
	}
	w, err := ff.rr.r.Resolve(target)
	if err != nil {
		return err
	}
	ff.rr.w = w
	ff.rr.addrCh = make(chan []Address, 1)
	go func() {
		for {
			if err := ff.watchAddrUpdates(); err != nil {
				return
			}
		}
	}()
	return nil
}

// Up sets the connected state of addr and sends notification if there are pending
// Get() calls.
func (ff *firstFind) Up(addr Address) func(error) {
	return ff.rr.Up(addr)
}

// down unsets the connected state of addr.
func (ff *firstFind) down(addr Address, err error) {
	ff.rr.mu.Lock()
	defer ff.rr.mu.Unlock()
	for _, a := range ff.rr.addrs {
		if addr == a.addr {
			a.connected = false
			break
		}
	}
}

// Get returns the next addr in the rotation.
func (ff *firstFind) Get(ctx context.Context, opts BalancerGetOptions) (addr Address, put func(), err error) {
	addr, put, err = ff.rr.Get(ctx, opts)
	return
	/*var ch chan struct{}
	ff.rr.mu.Lock()
	if ff.rr.done {
		ff.rr.mu.Unlock()
		err = ErrClientConnClosing
		return
	}

	if len(ff.rr.addrs) > 0 {
		next := 0
		for {
			a := ff.rr.addrs[next]
			next = (next + 1) % len(ff.rr.addrs)
			if a.connected {
				addr = a.addr
				ff.rr.mu.Unlock()
				return
			}
			if next == 0 {
				// Has iterated all the possible address but none is connected.
				break
			}
		}
	}
	if !opts.BlockingWait {
		if len(ff.rr.addrs) == 0 {
			ff.rr.mu.Unlock()
			err = Errorf(codes.Unavailable, "there is no address available")
			return
		}
		// Returns the next addr on rr.addrs for failfast RPCs.
		addr = ff.rr.addrs[0].addr
		ff.rr.mu.Unlock()
		return
	}
	// Wait on rr.waitCh for non-failfast RPCs.
	if ff.rr.waitCh == nil {
		ch = make(chan struct{})
		ff.rr.waitCh = ch
	} else {
		ch = ff.rr.waitCh
	}
	ff.rr.mu.Unlock()
	for {
		fmt.Println(len(ff.rr.addrs), ff.rr.addrs)
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-ch:
			ff.rr.mu.Lock()
			if ff.rr.done {
				ff.rr.mu.Unlock()
				err = ErrClientConnClosing
				return
			}

			if len(ff.rr.addrs) > 0 {
				next := 0
				for {
					a := ff.rr.addrs[next]
					next = (next + 1) % len(ff.rr.addrs)
					if a.connected {
						addr = a.addr
						ff.rr.mu.Unlock()
						return
					}
					if next == 0 {
						// Has iterated all the possible address but none is connected.
						break
					}
				}
			}
		// The newly added addr got removed by Down() again.
			if ff.rr.waitCh == nil {
				ch = make(chan struct{})
				ff.rr.waitCh = ch
			} else {
				ch = ff.rr.waitCh
			}
			ff.rr.mu.Unlock()
		}
	}
	*/
}

func (ff *firstFind) Notify() <-chan []Address {
	return ff.rr.addrCh
}

func (ff *firstFind) Close() error {
	return ff.rr.Close()
}
