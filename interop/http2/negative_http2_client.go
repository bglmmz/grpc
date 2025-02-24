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
 *
 * Client used to test http2 error edge cases like GOAWAYs and RST_STREAMs
 *
 * Documentation:
 *	 https://github.com/grpc/grpc/blob/master/doc/negative-http2-interop-test-descriptions.md
 */

package main

import (
	"flag"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/bglmmz/grpc"
	"github.com/bglmmz/grpc/codes"
	"github.com/bglmmz/grpc/grpclog"
	"github.com/bglmmz/grpc/interop"
	testpb "github.com/bglmmz/grpc/interop/grpc_testing"
	"github.com/bglmmz/grpc/status"
	"golang.org/x/net/context"
)

var (
	serverHost = flag.String("server_host", "127.0.0.1", "The server host name")
	serverPort = flag.Int("server_port", 8080, "The server port number")
	testCase   = flag.String("test_case", "goaway",
		`Configure different test cases. Valid options are:
        goaway : client sends two requests, the server will send a goaway in between;
        rst_after_header : server will send rst_stream after it sends headers;
        rst_during_data : server will send rst_stream while sending data;
        rst_after_data : server will send rst_stream after sending data;
        ping : server will send pings between each http2 frame;
        max_streams : server will ensure that the max_concurrent_streams limit is upheld;`)
	largeReqSize  = 271828
	largeRespSize = 314159
)

func largeSimpleRequest() *testpb.SimpleRequest {
	pl := interop.ClientNewPayload(testpb.PayloadType_COMPRESSABLE, largeReqSize)
	return &testpb.SimpleRequest{
		ResponseType: testpb.PayloadType_COMPRESSABLE,
		ResponseSize: int32(largeRespSize),
		Payload:      pl,
	}
}

// sends two unary calls. The server asserts that the calls use different connections.
func goaway(tc testpb.TestServiceClient) {
	interop.DoLargeUnaryCall(tc)
	// sleep to ensure that the client has time to recv the GOAWAY.
	// TODO(ncteisen): make this less hacky.
	time.Sleep(1 * time.Second)
	interop.DoLargeUnaryCall(tc)
}

func rstAfterHeader(tc testpb.TestServiceClient) {
	req := largeSimpleRequest()
	reply, err := tc.UnaryCall(context.Background(), req)
	if reply != nil {
		grpclog.Fatalf("Client received reply despite server sending rst stream after header")
	}
	if status.Code(err) != codes.Internal {
		grpclog.Fatalf("%v.UnaryCall() = _, %v, want _, %v", tc, status.Code(err), codes.Internal)
	}
}

func rstDuringData(tc testpb.TestServiceClient) {
	req := largeSimpleRequest()
	reply, err := tc.UnaryCall(context.Background(), req)
	if reply != nil {
		grpclog.Fatalf("Client received reply despite server sending rst stream during data")
	}
	if status.Code(err) != codes.Unknown {
		grpclog.Fatalf("%v.UnaryCall() = _, %v, want _, %v", tc, status.Code(err), codes.Unknown)
	}
}

func rstAfterData(tc testpb.TestServiceClient) {
	req := largeSimpleRequest()
	reply, err := tc.UnaryCall(context.Background(), req)
	if reply != nil {
		grpclog.Fatalf("Client received reply despite server sending rst stream after data")
	}
	if status.Code(err) != codes.Internal {
		grpclog.Fatalf("%v.UnaryCall() = _, %v, want _, %v", tc, status.Code(err), codes.Internal)
	}
}

func ping(tc testpb.TestServiceClient) {
	// The server will assert that every ping it sends was ACK-ed by the client.
	interop.DoLargeUnaryCall(tc)
}

func maxStreams(tc testpb.TestServiceClient) {
	interop.DoLargeUnaryCall(tc)
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			interop.DoLargeUnaryCall(tc)
		}()
	}
	wg.Wait()
}

func main() {
	flag.Parse()
	serverAddr := net.JoinHostPort(*serverHost, strconv.Itoa(*serverPort))
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithInsecure())
	conn, err := grpc.Dial(serverAddr, opts...)
	if err != nil {
		grpclog.Fatalf("Fail to dial: %v", err)
	}
	defer conn.Close()
	tc := testpb.NewTestServiceClient(conn)
	switch *testCase {
	case "goaway":
		goaway(tc)
		grpclog.Infoln("goaway done")
	case "rst_after_header":
		rstAfterHeader(tc)
		grpclog.Infoln("rst_after_header done")
	case "rst_during_data":
		rstDuringData(tc)
		grpclog.Infoln("rst_during_data done")
	case "rst_after_data":
		rstAfterData(tc)
		grpclog.Infoln("rst_after_data done")
	case "ping":
		ping(tc)
		grpclog.Infoln("ping done")
	case "max_streams":
		maxStreams(tc)
		grpclog.Infoln("max_streams done")
	default:
		grpclog.Fatal("Unsupported test case: ", *testCase)
	}
}
