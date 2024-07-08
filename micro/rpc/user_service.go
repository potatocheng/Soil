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
	Msg string
	Err error
}

func (u *UserServiceServer) GetUserById(ctx context.Context, in *GetUserByIdReq) (*GetUserByIdResp, error) {
	log.Println(in)
	return &GetUserByIdResp{Msg: u.Msg}, u.Err
}

func (u *UserServiceServer) Name() string {
	return "user-service"
}
