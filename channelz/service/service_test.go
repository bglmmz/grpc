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

package service

import (
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	channelzpb "github.com/bglmmz/grpc/channelz/grpc_channelz_v1"
	"github.com/bglmmz/grpc/connectivity"
	"github.com/bglmmz/grpc/credentials"
	"github.com/bglmmz/grpc/internal/channelz"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"
)

func init() {
	channelz.TurnOn()
}

type protoToSocketOptFunc func([]*channelzpb.SocketOption) *channelz.SocketOptionData

// protoToSocketOpt is used in function socketProtoToStruct to extract socket option
// data from unmarshaled proto message.
// It is only defined under linux, non-appengine environment on x86 architecture.
var protoToSocketOpt protoToSocketOptFunc

// emptyTime is used for detecting unset value of time.Time type.
// For go1.7 and earlier, ptypes.Timestamp will fill in the loc field of time.Time
// with &utcLoc. However zero value of a time.Time type value loc field is nil.
// This behavior will make reflect.DeepEqual fail upon unset time.Time field,
// and cause false positive fatal error.
var emptyTime time.Time

type dummyChannel struct {
	state                    connectivity.State
	target                   string
	callsStarted             int64
	callsSucceeded           int64
	callsFailed              int64
	lastCallStartedTimestamp time.Time
}

func (d *dummyChannel) ChannelzMetric() *channelz.ChannelInternalMetric {
	return &channelz.ChannelInternalMetric{
		State:                    d.state,
		Target:                   d.target,
		CallsStarted:             d.callsStarted,
		CallsSucceeded:           d.callsSucceeded,
		CallsFailed:              d.callsFailed,
		LastCallStartedTimestamp: d.lastCallStartedTimestamp,
	}
}

type dummyServer struct {
	callsStarted             int64
	callsSucceeded           int64
	callsFailed              int64
	lastCallStartedTimestamp time.Time
}

func (d *dummyServer) ChannelzMetric() *channelz.ServerInternalMetric {
	return &channelz.ServerInternalMetric{
		CallsStarted:             d.callsStarted,
		CallsSucceeded:           d.callsSucceeded,
		CallsFailed:              d.callsFailed,
		LastCallStartedTimestamp: d.lastCallStartedTimestamp,
	}
}

type dummySocket struct {
	streamsStarted                   int64
	streamsSucceeded                 int64
	streamsFailed                    int64
	messagesSent                     int64
	messagesReceived                 int64
	keepAlivesSent                   int64
	lastLocalStreamCreatedTimestamp  time.Time
	lastRemoteStreamCreatedTimestamp time.Time
	lastMessageSentTimestamp         time.Time
	lastMessageReceivedTimestamp     time.Time
	localFlowControlWindow           int64
	remoteFlowControlWindow          int64
	socketOptions                    *channelz.SocketOptionData
	localAddr                        net.Addr
	remoteAddr                       net.Addr
	security                         credentials.ChannelzSecurityValue
	remoteName                       string
}

func (d *dummySocket) ChannelzMetric() *channelz.SocketInternalMetric {
	return &channelz.SocketInternalMetric{
		StreamsStarted:                   d.streamsStarted,
		StreamsSucceeded:                 d.streamsSucceeded,
		StreamsFailed:                    d.streamsFailed,
		MessagesSent:                     d.messagesSent,
		MessagesReceived:                 d.messagesReceived,
		KeepAlivesSent:                   d.keepAlivesSent,
		LastLocalStreamCreatedTimestamp:  d.lastLocalStreamCreatedTimestamp,
		LastRemoteStreamCreatedTimestamp: d.lastRemoteStreamCreatedTimestamp,
		LastMessageSentTimestamp:         d.lastMessageSentTimestamp,
		LastMessageReceivedTimestamp:     d.lastMessageReceivedTimestamp,
		LocalFlowControlWindow:           d.localFlowControlWindow,
		RemoteFlowControlWindow:          d.remoteFlowControlWindow,
		SocketOptions:                    d.socketOptions,
		LocalAddr:                        d.localAddr,
		RemoteAddr:                       d.remoteAddr,
		Security:                         d.security,
		RemoteName:                       d.remoteName,
	}
}

func channelProtoToStruct(c *channelzpb.Channel) *dummyChannel {
	dc := &dummyChannel{}
	pdata := c.GetData()
	switch pdata.GetState().GetState() {
	case channelzpb.ChannelConnectivityState_UNKNOWN:
		// TODO: what should we set here?
	case channelzpb.ChannelConnectivityState_IDLE:
		dc.state = connectivity.Idle
	case channelzpb.ChannelConnectivityState_CONNECTING:
		dc.state = connectivity.Connecting
	case channelzpb.ChannelConnectivityState_READY:
		dc.state = connectivity.Ready
	case channelzpb.ChannelConnectivityState_TRANSIENT_FAILURE:
		dc.state = connectivity.TransientFailure
	case channelzpb.ChannelConnectivityState_SHUTDOWN:
		dc.state = connectivity.Shutdown
	}
	dc.target = pdata.GetTarget()
	dc.callsStarted = pdata.CallsStarted
	dc.callsSucceeded = pdata.CallsSucceeded
	dc.callsFailed = pdata.CallsFailed
	if t, err := ptypes.Timestamp(pdata.GetLastCallStartedTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			dc.lastCallStartedTimestamp = t
		}
	}
	return dc
}

func serverProtoToStruct(s *channelzpb.Server) *dummyServer {
	ds := &dummyServer{}
	pdata := s.GetData()
	ds.callsStarted = pdata.CallsStarted
	ds.callsSucceeded = pdata.CallsSucceeded
	ds.callsFailed = pdata.CallsFailed
	if t, err := ptypes.Timestamp(pdata.GetLastCallStartedTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			ds.lastCallStartedTimestamp = t
		}
	}
	return ds
}

func socketProtoToStruct(s *channelzpb.Socket) *dummySocket {
	ds := &dummySocket{}
	pdata := s.GetData()
	ds.streamsStarted = pdata.GetStreamsStarted()
	ds.streamsSucceeded = pdata.GetStreamsSucceeded()
	ds.streamsFailed = pdata.GetStreamsFailed()
	ds.messagesSent = pdata.GetMessagesSent()
	ds.messagesReceived = pdata.GetMessagesReceived()
	ds.keepAlivesSent = pdata.GetKeepAlivesSent()
	if t, err := ptypes.Timestamp(pdata.GetLastLocalStreamCreatedTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			ds.lastLocalStreamCreatedTimestamp = t
		}
	}
	if t, err := ptypes.Timestamp(pdata.GetLastRemoteStreamCreatedTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			ds.lastRemoteStreamCreatedTimestamp = t
		}
	}
	if t, err := ptypes.Timestamp(pdata.GetLastMessageSentTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			ds.lastMessageSentTimestamp = t
		}
	}
	if t, err := ptypes.Timestamp(pdata.GetLastMessageReceivedTimestamp()); err == nil {
		if !t.Equal(emptyTime) {
			ds.lastMessageReceivedTimestamp = t
		}
	}
	if v := pdata.GetLocalFlowControlWindow(); v != nil {
		ds.localFlowControlWindow = v.Value
	}
	if v := pdata.GetRemoteFlowControlWindow(); v != nil {
		ds.remoteFlowControlWindow = v.Value
	}
	if v := pdata.GetOption(); v != nil && protoToSocketOpt != nil {
		ds.socketOptions = protoToSocketOpt(v)
	}
	if v := s.GetSecurity(); v != nil {
		ds.security = protoToSecurity(v)
	}
	if local := s.GetLocal(); local != nil {
		ds.localAddr = protoToAddr(local)
	}
	if remote := s.GetRemote(); remote != nil {
		ds.remoteAddr = protoToAddr(remote)
	}
	ds.remoteName = s.GetRemoteName()
	return ds
}

func protoToSecurity(protoSecurity *channelzpb.Security) credentials.ChannelzSecurityValue {
	switch v := protoSecurity.Model.(type) {
	case *channelzpb.Security_Tls_:
		return &credentials.TLSChannelzSecurityValue{StandardName: v.Tls.GetStandardName(), LocalCertificate: v.Tls.GetLocalCertificate(), RemoteCertificate: v.Tls.GetRemoteCertificate()}
	case *channelzpb.Security_Other:
		sv := &credentials.OtherChannelzSecurityValue{Name: v.Other.GetName()}
		var x ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(v.Other.GetValue(), &x); err == nil {
			sv.Value = x.Message
		}
		return sv
	}
	return nil
}

func protoToAddr(a *channelzpb.Address) net.Addr {
	switch v := a.Address.(type) {
	case *channelzpb.Address_TcpipAddress:
		if port := v.TcpipAddress.GetPort(); port != 0 {
			return &net.TCPAddr{IP: v.TcpipAddress.GetIpAddress(), Port: int(port)}
		}
		return &net.IPAddr{IP: v.TcpipAddress.GetIpAddress()}
	case *channelzpb.Address_UdsAddress_:
		return &net.UnixAddr{Name: v.UdsAddress.GetFilename(), Net: "unix"}
	case *channelzpb.Address_OtherAddress_:
		// TODO:
	}
	return nil
}

func convertSocketRefSliceToMap(sktRefs []*channelzpb.SocketRef) map[int64]string {
	m := make(map[int64]string)
	for _, sr := range sktRefs {
		m[sr.SocketId] = sr.Name
	}
	return m
}

type OtherSecurityValue struct {
	LocalCertificate  []byte `protobuf:"bytes,1,opt,name=local_certificate,json=localCertificate,proto3" json:"local_certificate,omitempty"`
	RemoteCertificate []byte `protobuf:"bytes,2,opt,name=remote_certificate,json=remoteCertificate,proto3" json:"remote_certificate,omitempty"`
}

func (m *OtherSecurityValue) Reset()         { *m = OtherSecurityValue{} }
func (m *OtherSecurityValue) String() string { return proto.CompactTextString(m) }
func (*OtherSecurityValue) ProtoMessage()    {}

func init() {
	// Ad-hoc registering the proto type here to facilitate UnmarshalAny of OtherSecurityValue.
	proto.RegisterType((*OtherSecurityValue)(nil), "grpc.credentials.OtherChannelzSecurityValue")
}

func TestGetTopChannels(t *testing.T) {
	tcs := []*dummyChannel{
		{
			state:                    connectivity.Connecting,
			target:                   "test.channelz:1234",
			callsStarted:             6,
			callsSucceeded:           2,
			callsFailed:              3,
			lastCallStartedTimestamp: time.Now().UTC(),
		},
		{
			state:                    connectivity.Connecting,
			target:                   "test.channelz:1234",
			callsStarted:             1,
			callsSucceeded:           2,
			callsFailed:              3,
			lastCallStartedTimestamp: time.Now().UTC(),
		},
		{
			state:          connectivity.Shutdown,
			target:         "test.channelz:8888",
			callsStarted:   0,
			callsSucceeded: 0,
			callsFailed:    0,
		},
		{},
	}
	channelz.NewChannelzStorage()
	for _, c := range tcs {
		channelz.RegisterChannel(c, 0, "")
	}
	s := newCZServer()
	resp, _ := s.GetTopChannels(context.Background(), &channelzpb.GetTopChannelsRequest{StartChannelId: 0})
	if !resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want true, got %v", resp.GetEnd())
	}
	for i, c := range resp.GetChannel() {
		if !reflect.DeepEqual(channelProtoToStruct(c), tcs[i]) {
			t.Fatalf("dummyChannel: %d, want: %#v, got: %#v", i, tcs[i], channelProtoToStruct(c))
		}
	}
	for i := 0; i < 50; i++ {
		channelz.RegisterChannel(tcs[0], 0, "")
	}
	resp, _ = s.GetTopChannels(context.Background(), &channelzpb.GetTopChannelsRequest{StartChannelId: 0})
	if resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want false, got %v", resp.GetEnd())
	}
}

func TestGetServers(t *testing.T) {
	ss := []*dummyServer{
		{
			callsStarted:             6,
			callsSucceeded:           2,
			callsFailed:              3,
			lastCallStartedTimestamp: time.Now().UTC(),
		},
		{
			callsStarted:             1,
			callsSucceeded:           2,
			callsFailed:              3,
			lastCallStartedTimestamp: time.Now().UTC(),
		},
		{
			callsStarted:             1,
			callsSucceeded:           0,
			callsFailed:              0,
			lastCallStartedTimestamp: time.Now().UTC(),
		},
	}
	channelz.NewChannelzStorage()
	for _, s := range ss {
		channelz.RegisterServer(s, "")
	}
	svr := newCZServer()
	resp, _ := svr.GetServers(context.Background(), &channelzpb.GetServersRequest{StartServerId: 0})
	if !resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want true, got %v", resp.GetEnd())
	}
	for i, s := range resp.GetServer() {
		if !reflect.DeepEqual(serverProtoToStruct(s), ss[i]) {
			t.Fatalf("dummyServer: %d, want: %#v, got: %#v", i, ss[i], serverProtoToStruct(s))
		}
	}
	for i := 0; i < 50; i++ {
		channelz.RegisterServer(ss[0], "")
	}
	resp, _ = svr.GetServers(context.Background(), &channelzpb.GetServersRequest{StartServerId: 0})
	if resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want false, got %v", resp.GetEnd())
	}
}

func TestGetServerSockets(t *testing.T) {
	channelz.NewChannelzStorage()
	svrID := channelz.RegisterServer(&dummyServer{}, "")
	refNames := []string{"listen socket 1", "normal socket 1", "normal socket 2"}
	ids := make([]int64, 3)
	ids[0] = channelz.RegisterListenSocket(&dummySocket{}, svrID, refNames[0])
	ids[1] = channelz.RegisterNormalSocket(&dummySocket{}, svrID, refNames[1])
	ids[2] = channelz.RegisterNormalSocket(&dummySocket{}, svrID, refNames[2])
	svr := newCZServer()
	resp, _ := svr.GetServerSockets(context.Background(), &channelzpb.GetServerSocketsRequest{ServerId: svrID, StartSocketId: 0})
	if !resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want: true, got: %v", resp.GetEnd())
	}
	// GetServerSockets only return normal sockets.
	want := map[int64]string{
		ids[1]: refNames[1],
		ids[2]: refNames[2],
	}
	if !reflect.DeepEqual(convertSocketRefSliceToMap(resp.GetSocketRef()), want) {
		t.Fatalf("GetServerSockets want: %#v, got: %#v", want, resp.GetSocketRef())
	}

	for i := 0; i < 50; i++ {
		channelz.RegisterNormalSocket(&dummySocket{}, svrID, "")
	}
	resp, _ = svr.GetServerSockets(context.Background(), &channelzpb.GetServerSocketsRequest{ServerId: svrID, StartSocketId: 0})
	if resp.GetEnd() {
		t.Fatalf("resp.GetEnd() want false, got %v", resp.GetEnd())
	}
}

func TestGetChannel(t *testing.T) {
	channelz.NewChannelzStorage()
	refNames := []string{"top channel 1", "nested channel 1", "nested channel 2", "nested channel 3"}
	ids := make([]int64, 4)
	ids[0] = channelz.RegisterChannel(&dummyChannel{}, 0, refNames[0])
	ids[1] = channelz.RegisterChannel(&dummyChannel{}, ids[0], refNames[1])
	ids[2] = channelz.RegisterSubChannel(&dummyChannel{}, ids[0], refNames[2])
	ids[3] = channelz.RegisterChannel(&dummyChannel{}, ids[1], refNames[3])
	svr := newCZServer()
	resp, _ := svr.GetChannel(context.Background(), &channelzpb.GetChannelRequest{ChannelId: ids[0]})
	metrics := resp.GetChannel()
	subChans := metrics.GetSubchannelRef()
	if len(subChans) != 1 || subChans[0].GetName() != refNames[2] || subChans[0].GetSubchannelId() != ids[2] {
		t.Fatalf("GetSubChannelRef() want %#v, got %#v", []*channelzpb.SubchannelRef{{SubchannelId: ids[2], Name: refNames[2]}}, subChans)
	}
	nestedChans := metrics.GetChannelRef()
	if len(nestedChans) != 1 || nestedChans[0].GetName() != refNames[1] || nestedChans[0].GetChannelId() != ids[1] {
		t.Fatalf("GetChannelRef() want %#v, got %#v", []*channelzpb.ChannelRef{{ChannelId: ids[1], Name: refNames[1]}}, nestedChans)
	}

	resp, _ = svr.GetChannel(context.Background(), &channelzpb.GetChannelRequest{ChannelId: ids[1]})
	metrics = resp.GetChannel()
	nestedChans = metrics.GetChannelRef()
	if len(nestedChans) != 1 || nestedChans[0].GetName() != refNames[3] || nestedChans[0].GetChannelId() != ids[3] {
		t.Fatalf("GetChannelRef() want %#v, got %#v", []*channelzpb.ChannelRef{{ChannelId: ids[3], Name: refNames[3]}}, nestedChans)
	}
}

func TestGetSubChannel(t *testing.T) {
	channelz.NewChannelzStorage()
	refNames := []string{"top channel 1", "sub channel 1", "socket 1", "socket 2"}
	ids := make([]int64, 4)
	ids[0] = channelz.RegisterChannel(&dummyChannel{}, 0, refNames[0])
	ids[1] = channelz.RegisterSubChannel(&dummyChannel{}, ids[0], refNames[1])
	ids[2] = channelz.RegisterNormalSocket(&dummySocket{}, ids[1], refNames[2])
	ids[3] = channelz.RegisterNormalSocket(&dummySocket{}, ids[1], refNames[3])
	svr := newCZServer()
	resp, _ := svr.GetSubchannel(context.Background(), &channelzpb.GetSubchannelRequest{SubchannelId: ids[1]})
	metrics := resp.GetSubchannel()
	want := map[int64]string{
		ids[2]: refNames[2],
		ids[3]: refNames[3],
	}
	if !reflect.DeepEqual(convertSocketRefSliceToMap(metrics.GetSocketRef()), want) {
		t.Fatalf("GetSocketRef() want %#v: got: %#v", want, metrics.GetSocketRef())
	}
}

func TestGetSocket(t *testing.T) {
	channelz.NewChannelzStorage()
	ss := []*dummySocket{
		{
			streamsStarted:                   10,
			streamsSucceeded:                 2,
			streamsFailed:                    3,
			messagesSent:                     20,
			messagesReceived:                 10,
			keepAlivesSent:                   2,
			lastLocalStreamCreatedTimestamp:  time.Now().UTC(),
			lastRemoteStreamCreatedTimestamp: time.Now().UTC(),
			lastMessageSentTimestamp:         time.Now().UTC(),
			lastMessageReceivedTimestamp:     time.Now().UTC(),
			localFlowControlWindow:           65536,
			remoteFlowControlWindow:          1024,
			localAddr:                        &net.TCPAddr{IP: net.ParseIP("1.0.0.1"), Port: 10001},
			remoteAddr:                       &net.TCPAddr{IP: net.ParseIP("12.0.0.1"), Port: 10002},
			remoteName:                       "remote.remote",
		},
		{
			streamsStarted:                   10,
			streamsSucceeded:                 2,
			streamsFailed:                    3,
			messagesSent:                     20,
			messagesReceived:                 10,
			keepAlivesSent:                   2,
			lastRemoteStreamCreatedTimestamp: time.Now().UTC(),
			lastMessageSentTimestamp:         time.Now().UTC(),
			lastMessageReceivedTimestamp:     time.Now().UTC(),
			localFlowControlWindow:           65536,
			remoteFlowControlWindow:          1024,
			localAddr:                        &net.UnixAddr{Name: "file.path", Net: "unix"},
			remoteAddr:                       &net.UnixAddr{Name: "another.path", Net: "unix"},
			remoteName:                       "remote.remote",
		},
		{
			streamsStarted:                  5,
			streamsSucceeded:                2,
			streamsFailed:                   3,
			messagesSent:                    20,
			messagesReceived:                10,
			keepAlivesSent:                  2,
			lastLocalStreamCreatedTimestamp: time.Now().UTC(),
			lastMessageSentTimestamp:        time.Now().UTC(),
			lastMessageReceivedTimestamp:    time.Now().UTC(),
			localFlowControlWindow:          65536,
			remoteFlowControlWindow:         10240,
			localAddr:                       &net.IPAddr{IP: net.ParseIP("1.0.0.1")},
			remoteAddr:                      &net.IPAddr{IP: net.ParseIP("9.0.0.1")},
			remoteName:                      "",
		},
		{
			localAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 10001},
		},
		{
			security: &credentials.TLSChannelzSecurityValue{
				StandardName:      "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
				RemoteCertificate: []byte{48, 130, 2, 156, 48, 130, 2, 5, 160},
			},
		},
		{
			security: &credentials.OtherChannelzSecurityValue{
				Name: "XXXX",
			},
		},
		{
			security: &credentials.OtherChannelzSecurityValue{
				Name:  "YYYY",
				Value: &OtherSecurityValue{LocalCertificate: []byte{1, 2, 3}, RemoteCertificate: []byte{4, 5, 6}},
			},
		},
	}
	svr := newCZServer()
	ids := make([]int64, len(ss))
	svrID := channelz.RegisterServer(&dummyServer{}, "")
	for i, s := range ss {
		ids[i] = channelz.RegisterNormalSocket(s, svrID, strconv.Itoa(i))
	}
	for i, s := range ss {
		resp, _ := svr.GetSocket(context.Background(), &channelzpb.GetSocketRequest{SocketId: ids[i]})
		metrics := resp.GetSocket()
		if !reflect.DeepEqual(metrics.GetRef(), &channelzpb.SocketRef{SocketId: ids[i], Name: strconv.Itoa(i)}) || !reflect.DeepEqual(socketProtoToStruct(metrics), s) {
			t.Fatalf("resp.GetSocket() want: metrics.GetRef() = %#v and %#v, got: metrics.GetRef() = %#v and %#v", &channelzpb.SocketRef{SocketId: ids[i], Name: strconv.Itoa(i)}, s, metrics.GetRef(), socketProtoToStruct(metrics))
		}
	}
}
