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

// This file is the implementation of a gRPC server using HTTP/2 which
// uses the standard Go http2 Server implementation (via the
// http.Handler interface), rather than speaking low-level HTTP/2
// frames itself. It is the implementation of *grpc.Server.ServeHTTP.

package transport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bglmmz/grpc/codes"
	"github.com/bglmmz/grpc/credentials"
	"github.com/bglmmz/grpc/metadata"
	"github.com/bglmmz/grpc/peer"
	"github.com/bglmmz/grpc/stats"
	"github.com/bglmmz/grpc/status"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"golang.org/x/net/http2"
)

// NewServerHandlerTransport returns a ServerTransport handling gRPC
// from inside an http.Handler. It requires that the http Server
// supports HTTP/2.
func NewServerHandlerTransport(w http.ResponseWriter, r *http.Request, stats stats.Handler) (ServerTransport, error) {
	if r.ProtoMajor != 2 {
		return nil, errors.New("gRPC requires HTTP/2")
	}
	if r.Method != "POST" {
		return nil, errors.New("invalid gRPC request method")
	}
	contentType := r.Header.Get("Content-Type")
	// TODO: do we assume contentType is lowercase? we did before
	contentSubtype, validContentType := contentSubtype(contentType)
	if !validContentType {
		return nil, errors.New("invalid gRPC request content-type")
	}
	if _, ok := w.(http.Flusher); !ok {
		return nil, errors.New("gRPC requires a ResponseWriter supporting http.Flusher")
	}
	if _, ok := w.(http.CloseNotifier); !ok {
		return nil, errors.New("gRPC requires a ResponseWriter supporting http.CloseNotifier")
	}

	st := &serverHandlerTransport{
		rw:             w,
		req:            r,
		closedCh:       make(chan struct{}),
		writes:         make(chan func()),
		contentType:    contentType,
		contentSubtype: contentSubtype,
		stats:          stats,
	}

	if v := r.Header.Get("grpc-timeout"); v != "" {
		to, err := decodeTimeout(v)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "malformed time-out: %v", err)
		}
		st.timeoutSet = true
		st.timeout = to
	}

	metakv := []string{"content-type", contentType}
	if r.Host != "" {
		metakv = append(metakv, ":authority", r.Host)
	}
	for k, vv := range r.Header {
		k = strings.ToLower(k)
		if isReservedHeader(k) && !isWhitelistedHeader(k) {
			continue
		}
		for _, v := range vv {
			v, err := decodeMetadataHeader(k, v)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "malformed binary metadata: %v", err)
			}
			metakv = append(metakv, k, v)
		}
	}
	st.headerMD = metadata.Pairs(metakv...)

	return st, nil
}

// serverHandlerTransport is an implementation of ServerTransport
// which replies to exactly one gRPC request (exactly one HTTP request),
// using the net/http.Handler interface. This http.Handler is guaranteed
// at this point to be speaking over HTTP/2, so it's able to speak valid
// gRPC.
type serverHandlerTransport struct {
	rw               http.ResponseWriter
	req              *http.Request
	timeoutSet       bool
	timeout          time.Duration
	didCommonHeaders bool

	headerMD metadata.MD

	closeOnce sync.Once
	closedCh  chan struct{} // closed on Close

	// writes is a channel of code to run serialized in the
	// ServeHTTP (HandleStreams) goroutine. The channel is closed
	// when WriteStatus is called.
	writes chan func()

	// block concurrent WriteStatus calls
	// e.g. grpc/(*serverStream).SendMsg/RecvMsg
	writeStatusMu sync.Mutex

	// we just mirror the request content-type
	contentType string
	// we store both contentType and contentSubtype so we don't keep recreating them
	// TODO make sure this is consistent across handler_server and http2_server
	contentSubtype string

	stats stats.Handler
}

func (ht *serverHandlerTransport) Close() error {
	ht.closeOnce.Do(ht.closeCloseChanOnce)
	return nil
}

func (ht *serverHandlerTransport) closeCloseChanOnce() { close(ht.closedCh) }

func (ht *serverHandlerTransport) RemoteAddr() net.Addr { return strAddr(ht.req.RemoteAddr) }

// strAddr is a net.Addr backed by either a TCP "ip:port" string, or
// the empty string if unknown.
type strAddr string

func (a strAddr) Network() string {
	if a != "" {
		// Per the documentation on net/http.Request.RemoteAddr, if this is
		// set, it's set to the IP:port of the peer (hence, TCP):
		// https://golang.org/pkg/net/http/#Request
		//
		// If we want to support Unix sockets later, we can
		// add our own grpc-specific convention within the
		// grpc codebase to set RemoteAddr to a different
		// format, or probably better: we can attach it to the
		// context and use that from serverHandlerTransport.RemoteAddr.
		return "tcp"
	}
	return ""
}

func (a strAddr) String() string { return string(a) }

// do runs fn in the ServeHTTP goroutine.
func (ht *serverHandlerTransport) do(fn func()) error {
	// Avoid a panic writing to closed channel. Imperfect but maybe good enough.
	select {
	case <-ht.closedCh:
		return ErrConnClosing
	default:
		select {
		case ht.writes <- fn:
			return nil
		case <-ht.closedCh:
			return ErrConnClosing
		}
	}
}

func (ht *serverHandlerTransport) WriteStatus(s *Stream, st *status.Status) error {
	ht.writeStatusMu.Lock()
	defer ht.writeStatusMu.Unlock()

	err := ht.do(func() {
		ht.writeCommonHeaders(s)

		// And flush, in case no header or body has been sent yet.
		// This forces a separation of headers and trailers if this is the
		// first call (for example, in end2end tests's TestNoService).
		ht.rw.(http.Flusher).Flush()

		h := ht.rw.Header()
		h.Set("Grpc-Status", fmt.Sprintf("%d", st.Code()))
		if m := st.Message(); m != "" {
			h.Set("Grpc-Message", encodeGrpcMessage(m))
		}

		if p := st.Proto(); p != nil && len(p.Details) > 0 {
			stBytes, err := proto.Marshal(p)
			if err != nil {
				// TODO: return error instead, when callers are able to handle it.
				panic(err)
			}

			h.Set("Grpc-Status-Details-Bin", encodeBinHeader(stBytes))
		}

		if md := s.Trailer(); len(md) > 0 {
			for k, vv := range md {
				// Clients don't tolerate reading restricted headers after some non restricted ones were sent.
				if isReservedHeader(k) {
					continue
				}
				for _, v := range vv {
					// http2 ResponseWriter mechanism to send undeclared Trailers after
					// the headers have possibly been written.
					h.Add(http2.TrailerPrefix+k, encodeMetadataHeader(k, v))
				}
			}
		}
	})

	if err == nil { // transport has not been closed
		if ht.stats != nil {
			ht.stats.HandleRPC(s.Context(), &stats.OutTrailer{})
		}
		close(ht.writes)
	}
	ht.Close()
	return err
}

// writeCommonHeaders sets common headers on the first write
// call (Write, WriteHeader, or WriteStatus).
func (ht *serverHandlerTransport) writeCommonHeaders(s *Stream) {
	if ht.didCommonHeaders {
		return
	}
	ht.didCommonHeaders = true

	h := ht.rw.Header()
	h["Date"] = nil // suppress Date to make tests happy; TODO: restore
	h.Set("Content-Type", ht.contentType)

	// Predeclare trailers we'll set later in WriteStatus (after the body).
	// This is a SHOULD in the HTTP RFC, and the way you add (known)
	// Trailers per the net/http.ResponseWriter contract.
	// See https://golang.org/pkg/net/http/#ResponseWriter
	// and https://golang.org/pkg/net/http/#example_ResponseWriter_trailers
	h.Add("Trailer", "Grpc-Status")
	h.Add("Trailer", "Grpc-Message")
	h.Add("Trailer", "Grpc-Status-Details-Bin")

	if s.sendCompress != "" {
		h.Set("Grpc-Encoding", s.sendCompress)
	}
}

func (ht *serverHandlerTransport) Write(s *Stream, hdr []byte, data []byte, opts *Options) error {
	return ht.do(func() {
		ht.writeCommonHeaders(s)
		ht.rw.Write(hdr)
		ht.rw.Write(data)
		ht.rw.(http.Flusher).Flush()
	})
}

func (ht *serverHandlerTransport) WriteHeader(s *Stream, md metadata.MD) error {
	err := ht.do(func() {
		ht.writeCommonHeaders(s)
		h := ht.rw.Header()
		for k, vv := range md {
			// Clients don't tolerate reading restricted headers after some non restricted ones were sent.
			if isReservedHeader(k) {
				continue
			}
			for _, v := range vv {
				v = encodeMetadataHeader(k, v)
				h.Add(k, v)
			}
		}
		ht.rw.WriteHeader(200)
		ht.rw.(http.Flusher).Flush()
	})

	if err == nil {
		if ht.stats != nil {
			ht.stats.HandleRPC(s.Context(), &stats.OutHeader{})
		}
	}
	return err
}

func (ht *serverHandlerTransport) HandleStreams(startStream func(*Stream), traceCtx func(context.Context, string) context.Context) {
	// With this transport type there will be exactly 1 stream: this HTTP request.

	ctx := contextFromRequest(ht.req)
	var cancel context.CancelFunc
	if ht.timeoutSet {
		ctx, cancel = context.WithTimeout(ctx, ht.timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	// requestOver is closed when either the request's context is done
	// or the status has been written via WriteStatus.
	requestOver := make(chan struct{})

	// clientGone receives a single value if peer is gone, either
	// because the underlying connection is dead or because the
	// peer sends an http2 RST_STREAM.
	clientGone := ht.rw.(http.CloseNotifier).CloseNotify()
	go func() {
		select {
		case <-requestOver:
		case <-ht.closedCh:
		case <-clientGone:
		}
		cancel()
		ht.Close()
	}()

	req := ht.req

	s := &Stream{
		id:             0, // irrelevant
		requestRead:    func(int) {},
		cancel:         cancel,
		buf:            newRecvBuffer(),
		st:             ht,
		method:         req.URL.Path,
		recvCompress:   req.Header.Get("grpc-encoding"),
		contentSubtype: ht.contentSubtype,
	}
	pr := &peer.Peer{
		Addr: ht.RemoteAddr(),
	}
	if req.TLS != nil {
		pr.AuthInfo = credentials.TLSInfo{State: *cloneConnectionState(req.TLS)}
	}
	ctx = metadata.NewIncomingContext(ctx, ht.headerMD)
	s.ctx = peer.NewContext(ctx, pr)
	if ht.stats != nil {
		s.ctx = ht.stats.TagRPC(s.ctx, &stats.RPCTagInfo{FullMethodName: s.method})
		inHeader := &stats.InHeader{
			FullMethod:  s.method,
			RemoteAddr:  ht.RemoteAddr(),
			Compression: s.recvCompress,
		}
		ht.stats.HandleRPC(s.ctx, inHeader)
	}
	s.trReader = &transportReader{
		reader:        &recvBufferReader{ctx: s.ctx, ctxDone: s.ctx.Done(), recv: s.buf},
		windowHandler: func(int) {},
	}

	// readerDone is closed when the Body.Read-ing goroutine exits.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)

		// TODO: minimize garbage, optimize recvBuffer code/ownership
		const readSize = 8196
		for buf := make([]byte, readSize); ; {
			n, err := req.Body.Read(buf)
			if n > 0 {
				s.buf.put(recvMsg{data: buf[:n:n]})
				buf = buf[n:]
			}
			if err != nil {
				s.buf.put(recvMsg{err: mapRecvMsgError(err)})
				return
			}
			if len(buf) == 0 {
				buf = make([]byte, readSize)
			}
		}
	}()

	// startStream is provided by the *grpc.Server's serveStreams.
	// It starts a goroutine serving s and exits immediately.
	// The goroutine that is started is the one that then calls
	// into ht, calling WriteHeader, Write, WriteStatus, Close, etc.
	startStream(s)

	ht.runStream()
	close(requestOver)

	// Wait for reading goroutine to finish.
	req.Body.Close()
	<-readerDone
}

func (ht *serverHandlerTransport) runStream() {
	for {
		select {
		case fn, ok := <-ht.writes:
			if !ok {
				return
			}
			fn()
		case <-ht.closedCh:
			return
		}
	}
}

func (ht *serverHandlerTransport) IncrMsgSent() {}

func (ht *serverHandlerTransport) IncrMsgRecv() {}

func (ht *serverHandlerTransport) Drain() {
	panic("Drain() is not implemented")
}

// mapRecvMsgError returns the non-nil err into the appropriate
// error value as expected by callers of *grpc.parser.recvMsg.
// In particular, in can only be:
//   * io.EOF
//   * io.ErrUnexpectedEOF
//   * of type transport.ConnectionError
//   * an error from the status package
func mapRecvMsgError(err error) error {
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return err
	}
	if se, ok := err.(http2.StreamError); ok {
		if code, ok := http2ErrConvTab[se.Code]; ok {
			return status.Error(code, se.Error())
		}
	}
	if strings.Contains(err.Error(), "body closed by handler") {
		return status.Error(codes.Canceled, err.Error())
	}
	return connectionErrorf(true, err, err.Error())
}
