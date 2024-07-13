package rpc

import (
	"Soil/micro/proto/gen"
	"Soil/micro/rpc/compressor/gzipCompressor"
	"Soil/micro/rpc/serialize/protobuf"
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestClientAndServer(t *testing.T) {
	server := NewServer("tcp", "127.0.0.1:8080")

	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()
	time.Sleep(time.Second * 3)

	client, err := NewClient("127.0.0.1:8080")
	require.NoError(t, err)
	clientUserService := &UserService{}
	err = client.InitClientProxy(clientUserService)
	require.NoError(t, err)

	service := &UserServiceServer{}
	server.RegisterService(service)

	testCases := []struct {
		name   string
		before func()

		wantErr    error
		wantResult *GetUserByIdResp
	}{
		{
			name: "no error",
			before: func() {
				service.Msg = "Hello World"
			},
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
		{
			name: "exist error, no data",
			before: func() {
				service.Msg = ""
				service.Err = errors.New("this is a error")
			},
			wantResult: &GetUserByIdResp{},
			wantErr:    errors.New("this is a error"),
		},
		{
			name: "both",
			before: func() {
				service.Msg = "Hello World"
				service.Err = errors.New("this is a error")
			},
			wantErr: errors.New("this is a error"),
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before()
			resp, er := clientUserService.GetUserById(context.Background(), &GetUserByIdReq{Id: 123})
			assert.Equal(t, er, tc.wantErr)
			assert.Equal(t, tc.wantResult, resp)
		})
	}
}

func TestClientAndServerProto(t *testing.T) {
	server := NewServer("tcp", "127.0.0.1:8080")
	service := &UserServiceServer{}
	server.RegisterService(service)
	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()
	time.Sleep(time.Second * 3)

	client, err := NewClient("127.0.0.1:8080", ClientWithSerializer(&protobuf.Serializer{}))
	require.NoError(t, err)
	clientUserService := &UserService{}
	err = client.InitClientProxy(clientUserService)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		before func()

		wantErr    error
		wantResult *GetUserByIdResp
	}{
		{
			name: "no error",
			before: func() {
				service.Msg = "Hello World"
			},
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
		{
			name: "exist error, no data",
			before: func() {
				service.Msg = ""
				service.Err = errors.New("this is a error")
			},
			wantResult: &GetUserByIdResp{},
			wantErr:    errors.New("this is a error"),
		},
		{
			name: "both",
			before: func() {
				service.Msg = "Hello World"
				service.Err = errors.New("this is a error")
			},
			wantErr: errors.New("this is a error"),
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before()
			resp, er := clientUserService.GetUserByIdProto(context.Background(), &gen.GetUserByIdReq{Id: 123})
			assert.Equal(t, er, tc.wantErr)
			assert.Equal(t, tc.wantResult.Msg, resp.Msg)
		})
	}
}

func TestOneWay(t *testing.T) {
	server := NewServer("tcp", "127.0.0.1:8080")
	service := &UserServiceServer{}
	server.RegisterService(service)
	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()
	time.Sleep(time.Second * 3)

	client, err := NewClient("127.0.0.1:8080", ClientWithSerializer(&protobuf.Serializer{}))
	require.NoError(t, err)
	clientUserService := &UserService{}
	err = client.InitClientProxy(clientUserService)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		before func()

		wantErr    error
		wantResult *GetUserByIdResp
	}{
		{
			name: "both",
			before: func() {
				service.Msg = "Hello World"
				service.Err = errors.New("this is a error")
			},
			wantErr: errors.New("mirco: one-way调用，不必处理结果"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before()
			ctx := CtxWithOneWay(context.Background())
			_, err = clientUserService.GetUserByIdProto(ctx, &gen.GetUserByIdReq{Id: 123})
			fmt.Println(err)
			assert.Equal(t, err, tc.wantErr)
		})
	}
}

func TestClientAndServer_Compressor(t *testing.T) {
	server := NewServer("tcp", "127.0.0.1:8080")

	go func() {
		err := server.Start()
		require.NoError(t, err)
	}()
	time.Sleep(time.Second * 3)

	client, err := NewClient("127.0.0.1:8080", ClientWithCompressor(&gzipCompressor.Compressor{}))
	require.NoError(t, err)
	clientUserService := &UserService{}
	err = client.InitClientProxy(clientUserService)
	require.NoError(t, err)

	service := &UserServiceServer{}
	server.RegisterService(service)

	testCases := []struct {
		name   string
		before func()

		wantErr    error
		wantResult *GetUserByIdResp
	}{
		{
			name: "no error",
			before: func() {
				service.Msg = "Hello World"
			},
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
		{
			name: "exist error, no data",
			before: func() {
				service.Msg = ""
				service.Err = errors.New("this is a error")
			},
			wantResult: &GetUserByIdResp{},
			wantErr:    errors.New("this is a error"),
		},
		{
			name: "both",
			before: func() {
				service.Msg = "Hello World"
				service.Err = errors.New("this is a error")
			},
			wantErr: errors.New("this is a error"),
			wantResult: &GetUserByIdResp{
				Msg: "Hello World",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before()
			resp, er := clientUserService.GetUserById(context.Background(), &GetUserByIdReq{Id: 123})
			assert.Equal(t, er, tc.wantErr)
			assert.Equal(t, tc.wantResult, resp)
		})
	}
}
