package proto

import (
	"Soil/micro/proto/gen"
	"fmt"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestUserProto(t *testing.T) {
	req := gen.GetUserByIdReq{}
	req.Id = 123
	data, err := proto.Marshal(&req)
	require.NoError(t, err)

	req2 := gen.GetUserByIdReq{}
	err = proto.Unmarshal(data, &req2)
	require.NoError(t, err)
	fmt.Println(&req2)
}
