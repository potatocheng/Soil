package cache

import (
	"Soil/cache/mocks"
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestRedisCache_Set(t *testing.T) {
	testCases := []struct {
		name       string
		mock       func(ctrl *gomock.Controller) redis.Cmdable
		key        string
		value      string
		expiration time.Duration
		wantError  error
	}{
		{
			name: "set",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				statusCmd := redis.NewStatusCmd(context.Background())
				statusCmd.SetVal("OK")
				cmd.EXPECT().Set(context.Background(), "key1", "value1", time.Second).Return(statusCmd)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Second,
		},
		{
			name: "set",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				statusCmd := redis.NewStatusCmd(context.Background())
				statusCmd.SetErr(errors.New("error"))
				cmd.EXPECT().Set(context.Background(), "key1", "value1", time.Second).Return(statusCmd)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Second,
		},
		{
			name: "set error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				statusCmd := redis.NewStatusCmd(context.Background())
				statusCmd.SetErr(errors.New("error"))
				cmd.EXPECT().Set(context.Background(), "key1", "value1", time.Second).Return(statusCmd)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Second,
			wantError:  errors.New("error"),
		},
		{
			name: "timeout",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				statusCmd := redis.NewStatusCmd(context.Background())
				statusCmd.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().Set(context.Background(), "key1", "value1", time.Second).Return(statusCmd)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Second,
			wantError:  context.DeadlineExceeded,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			c := NewRedisCache(tc.mock(ctrl))
			err := c.Set(context.Background(), tc.key, tc.value, tc.expiration)
			assert.Equal(t, tc.wantError, err)
		})
	}
}

func TestRedisCache_Get(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) redis.Cmdable
		key       string
		value     string
		wantError error
	}{
		{
			name: "get",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewStringCmd(context.Background())
				status.SetVal("value1")
				cmd.EXPECT().Get(context.Background(), "key1").Return(status)

				return cmd
			},
			key:   "key1",
			value: "value1",
		},
		{
			name: "get non-existent key",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewStringCmd(context.Background())
				status.SetVal("(nil)")
				cmd.EXPECT().Get(context.Background(), "nonexisting").Return(status)

				return cmd
			},
			key:   "nonexisting",
			value: "(nil)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cache := NewRedisCache(tc.mock(ctrl))
			val, err := cache.Get(context.Background(), tc.key)
			assert.Equal(t, tc.wantError, err)
			if err != nil {
				return
			}
			assert.Equal(t, tc.value, val)
		})
	}
}

func TestRedisCache_Delete(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) redis.Cmdable
		key       string
		wantValue string
		wantErr   error
	}{
		{
			name: "delete",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewIntCmd(context.Background())
				status.SetVal(1)
				cmd.EXPECT().Del(context.Background(), "key1").Return(status)
				return cmd
			},
			key: "key1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cache := NewRedisCache(tc.mock(ctrl))
			err := cache.Delete(context.Background(), tc.key)
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
		})
	}
}
