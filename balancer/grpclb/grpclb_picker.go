/*
 *
 * Copyright 2017 gRPC authors.
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

package grpclb

import (
	"sync"
	"sync/atomic"

	"github.com/bglmmz/grpc/balancer"
	lbpb "github.com/bglmmz/grpc/balancer/grpclb/grpc_lb_v1"
	"github.com/bglmmz/grpc/codes"
	"github.com/bglmmz/grpc/status"
	"golang.org/x/net/context"
)

// rpcStats is same as lbmpb.ClientStats, except that numCallsDropped is a map
// instead of a slice.
type rpcStats struct {
	// Only access the following fields atomically.
	numCallsStarted                        int64
	numCallsFinished                       int64
	numCallsFinishedWithClientFailedToSend int64
	numCallsFinishedKnownReceived          int64

	mu sync.Mutex
	// map load_balance_token -> num_calls_dropped
	numCallsDropped map[string]int64
}

func newRPCStats() *rpcStats {
	return &rpcStats{
		numCallsDropped: make(map[string]int64),
	}
}

// toClientStats converts rpcStats to lbpb.ClientStats, and clears rpcStats.
func (s *rpcStats) toClientStats() *lbpb.ClientStats {
	stats := &lbpb.ClientStats{
		NumCallsStarted:                        atomic.SwapInt64(&s.numCallsStarted, 0),
		NumCallsFinished:                       atomic.SwapInt64(&s.numCallsFinished, 0),
		NumCallsFinishedWithClientFailedToSend: atomic.SwapInt64(&s.numCallsFinishedWithClientFailedToSend, 0),
		NumCallsFinishedKnownReceived:          atomic.SwapInt64(&s.numCallsFinishedKnownReceived, 0),
	}
	s.mu.Lock()
	dropped := s.numCallsDropped
	s.numCallsDropped = make(map[string]int64)
	s.mu.Unlock()
	for token, count := range dropped {
		stats.CallsFinishedWithDrop = append(stats.CallsFinishedWithDrop, &lbpb.ClientStatsPerToken{
			LoadBalanceToken: token,
			NumCalls:         count,
		})
	}
	return stats
}

func (s *rpcStats) drop(token string) {
	atomic.AddInt64(&s.numCallsStarted, 1)
	s.mu.Lock()
	s.numCallsDropped[token]++
	s.mu.Unlock()
	atomic.AddInt64(&s.numCallsFinished, 1)
}

func (s *rpcStats) failedToSend() {
	atomic.AddInt64(&s.numCallsStarted, 1)
	atomic.AddInt64(&s.numCallsFinishedWithClientFailedToSend, 1)
	atomic.AddInt64(&s.numCallsFinished, 1)
}

func (s *rpcStats) knownReceived() {
	atomic.AddInt64(&s.numCallsStarted, 1)
	atomic.AddInt64(&s.numCallsFinishedKnownReceived, 1)
	atomic.AddInt64(&s.numCallsFinished, 1)
}

type errPicker struct {
	// Pick always returns this err.
	err error
}

func (p *errPicker) Pick(ctx context.Context, opts balancer.PickOptions) (balancer.SubConn, func(balancer.DoneInfo), error) {
	return nil, nil, p.err
}

// rrPicker does roundrobin on subConns. It's typically used when there's no
// response from remote balancer, and grpclb falls back to the resolved
// backends.
//
// It guaranteed that len(subConns) > 0.
type rrPicker struct {
	mu           sync.Mutex
	subConns     []balancer.SubConn // The subConns that were READY when taking the snapshot.
	subConnsNext int
}

func (p *rrPicker) Pick(ctx context.Context, opts balancer.PickOptions) (balancer.SubConn, func(balancer.DoneInfo), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sc := p.subConns[p.subConnsNext]
	p.subConnsNext = (p.subConnsNext + 1) % len(p.subConns)
	return sc, nil, nil
}

// lbPicker does two layers of picks:
//
// First layer: roundrobin on all servers in serverList, including drops and backends.
// - If it picks a drop, the RPC will fail as being dropped.
// - If it picks a backend, do a second layer pick to pick the real backend.
//
// Second layer: roundrobin on all READY backends.
//
// It's guaranteed that len(serverList) > 0.
type lbPicker struct {
	mu             sync.Mutex
	serverList     []*lbpb.Server
	serverListNext int
	subConns       []balancer.SubConn // The subConns that were READY when taking the snapshot.
	subConnsNext   int

	stats *rpcStats
}

func (p *lbPicker) Pick(ctx context.Context, opts balancer.PickOptions) (balancer.SubConn, func(balancer.DoneInfo), error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Layer one roundrobin on serverList.
	s := p.serverList[p.serverListNext]
	p.serverListNext = (p.serverListNext + 1) % len(p.serverList)

	// If it's a drop, return an error and fail the RPC.
	if s.Drop {
		p.stats.drop(s.LoadBalanceToken)
		return nil, nil, status.Errorf(codes.Unavailable, "request dropped by grpclb")
	}

	// If not a drop but there's no ready subConns.
	if len(p.subConns) <= 0 {
		return nil, nil, balancer.ErrNoSubConnAvailable
	}

	// Return the next ready subConn in the list, also collect rpc stats.
	sc := p.subConns[p.subConnsNext]
	p.subConnsNext = (p.subConnsNext + 1) % len(p.subConns)
	done := func(info balancer.DoneInfo) {
		if !info.BytesSent {
			p.stats.failedToSend()
		} else if info.BytesReceived {
			p.stats.knownReceived()
		}
	}
	return sc, done, nil
}
