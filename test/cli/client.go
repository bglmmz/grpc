package main

import (
	"context"
	"fmt"
	"github.com/bglmmz/grpc"
	"github.com/bglmmz/grpc/credentials"
	"github.com/bglmmz/grpc/grpclog"
	"github.com/bglmmz/grpc/test/hello"
	"golang.org/x/net/trace"
	"net/http"
	"time"
)

func main() {
	// single cert
	//creds, err := credentials.NewClientTLSFromFile("E:/gopath/projects/grpc/test/single-cert/ca.crt",
	//	"peer0.org3.example.com")

	// double cert
	/*	creds, err := credentials.NewClientTLSFromFileForTwoWay(
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui\\CS.crt",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/CS.key",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/CE.crt",
		"D:\\golang\\Hyperledger-TWGC\\grpc\\test\\dahui/CE.key",
		"0.0.0.0")*/
	creds, err := credentials.NewClientTLSFromFileForTwoWay(
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\client_sign.crt",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\client_sign.key",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\client_cipher.crt",
		"D:\\golang\\bglmmz-grpc\\grpc\\test\\gm_cert\\client_cipher.key",
		"0.0.0.0")
	if err != nil {
		fmt.Println("1", err)
		return
	}

	// 开启trace
	go startTrace()

	grpcOptions := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	//grpcOptions = append(grpcOptions, grpc.WithInsecure())

	ctx, _ := context.WithTimeout(context.Background(), time.Minute*10)
	conn, err := grpc.DialContext(ctx, "localhost:6262", grpcOptions...)
	if err != nil {
		fmt.Println("dial error:", err)
		return
	}

	cli := hello.NewCommunicateClient(conn)
	ret, err := cli.Speak(context.Background(), &hello.Content{Detail: "aaa"})
	if err != nil {
		fmt.Println("speak error:", err)
		return
	}
	fmt.Println("speak ok:", ret.Detail)
}

func startTrace() {
	trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
		return true, true
	}
	go http.ListenAndServe(":50051", nil)
	grpclog.Println("Trace listen on 50051")
}