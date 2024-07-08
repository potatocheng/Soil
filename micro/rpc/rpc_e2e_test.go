package rpc

import (
	"context"
	"errors"
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

	clientUserService := &UserService{}
	err := InitClientProxy("127.0.0.1:8080", clientUserService)
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
