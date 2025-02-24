/*
 *
 * Copyright 2018 gRPC authors.
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
package health_test

import (
	"testing"

	"github.com/bglmmz/grpc"
	"github.com/bglmmz/grpc/health"
	pb "github.com/bglmmz/grpc/health/grpc_health_v1"
)

// Make sure the service implementation complies with the proto definition.
func TestRegister(t *testing.T) {
	s := grpc.NewServer()
	pb.RegisterHealthServer(s, health.NewServer())
	s.Stop()
}
