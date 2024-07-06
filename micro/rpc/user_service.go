package rpc

import (
	"context"
	"log"
)

type UserService struct {
	GetUserById func(ctx context.Context, in *GetUserByIdReq) (*GetUserByIdResp, error)
}

func (u UserService) Name() string {
	return "user-service"
}

type GetUserByIdReq struct {
	Id int
}

type GetUserByIdResp struct {
	Msg string
}

type UserServiceServer struct {
}

func (u *UserServiceServer) GetUserById(ctx context.Context, in *GetUserByIdReq) (*GetUserByIdResp, error) {
	log.Println(in)
	return &GetUserByIdResp{Msg: "Hello World"}, nil
}

func (u *UserServiceServer) Name() string {
	return "user-service"
}
