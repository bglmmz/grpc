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

//go:generate ./regenerate.sh

// Package health provides some utility functions to health-check a server. The implementation
// is based on protobuf. Users need to write their own implementations if other IDLs are used.
package health

import (
	"sync"

	"github.com/bglmmz/grpc/codes"
	healthpb "github.com/bglmmz/grpc/health/grpc_health_v1"
	"github.com/bglmmz/grpc/status"
	"golang.org/x/net/context"
)

// Server implements `service Health`.
type Server struct {
	mu sync.Mutex
	// statusMap stores the serving status of the services this Server monitors.
	statusMap map[string]healthpb.HealthCheckResponse_ServingStatus
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{
		statusMap: make(map[string]healthpb.HealthCheckResponse_ServingStatus),
	}
}

// Check implements `service Health`.
func (s *Server) Check(ctx context.Context, in *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.Service == "" {
		// check the server overall health status.
		return &healthpb.HealthCheckResponse{
			Status: healthpb.HealthCheckResponse_SERVING,
		}, nil
	}
	if status, ok := s.statusMap[in.Service]; ok {
		return &healthpb.HealthCheckResponse{
			Status: status,
		}, nil
	}
	return nil, status.Error(codes.NotFound, "unknown service")
}

// Watch implements `service Health`.
func (s *Server) Watch(in *healthpb.HealthCheckRequest, stream healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "Watching is not supported")
}

// SetServingStatus is called when need to reset the serving status of a service
// or insert a new service entry into the statusMap.
func (s *Server) SetServingStatus(service string, status healthpb.HealthCheckResponse_ServingStatus) {
	s.mu.Lock()
	s.statusMap[service] = status
	s.mu.Unlock()
}
