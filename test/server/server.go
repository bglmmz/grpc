package main

import (
	"context"
	"fmt"
	"github.com/bglmmz/grpc"
	"github.com/bglmmz/grpc/credentials"
	"github.com/bglmmz/grpc/test/hello"
	"net"
)

type CommServer struct {
}

func (comm *CommServer)Speak(ctx context.Context, content *hello.Content) (*hello.Content, error) {
	fmt.Println("receive message :", content.Detail)
	return &hello.Content{Detail: "i am server"}, nil
}

func main() {
	l, err := net.Listen("tcp", "localhost:6262")
	if err != nil {
		panic(err)
	}

	// load tls cert pair  single cert mode
	//creds, err := credentials.NewServerTLSFromFile("E:/gopath/projects/grpc/test/single-cert/server.crt",
	//	"E:/gopath/projects/grpc/test/single-cert/server.key")

	// load tls cert pair double cert mode
	/*	creds, err := credentials.NewServerTLSFromFileForTwoWay(
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui\\SS.crt",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/SS.key",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/SE.crt",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/SE.key")*/
	creds, err := credentials.NewServerTLSFromFileForTwoWay(
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\server_sign.crt",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\server_sign.key",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\server_cipher.crt",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\server_cipher.key")
	if err != nil {
		panic(err)
	}

	grpcOptions := []grpc.ServerOption{grpc.Creds(creds)}
	gprcServer := grpc.NewServer(grpcOptions...)
	hello.RegisterCommunicateServer(gprcServer, &CommServer{})
	fmt.Println("beginning to serve ...")
	if err = gprcServer.Serve(l); err != nil {
		panic(err)
	}
}
