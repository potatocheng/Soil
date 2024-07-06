package net

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	// 开启一个服务器
	serv := NewServer("tcp", ":8088")
	go func() {
		err := serv.Start()
		if err != nil {
			t.Log(err)
		}
	}()

	time.Sleep(time.Second * 3)

	cli := NewClient("tcp", "127.0.0.1:8088")
	res, err := cli.Communicate("Hello")
	require.NoError(t, err)
	log.Printf(res)
	assert.Equal(t, res, "Hello")
}
