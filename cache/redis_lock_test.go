package cache

import (
	"Soil/cache/internal/errs"
	"Soil/cache/mocks"
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestRedisLock_TryLock(t *testing.T) {
	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller) redis.Cmdable
		key      string
		wantLock *Lock
		wantErr  error
	}{
		{
			name: "tryLock SetNX failed",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewBoolCmd(context.Background())
				status.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().SetNX(context.Background(), "key", gomock.Any(), time.Minute).
					Return(status)
				return cmd
			},
			key:     "key",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "tryLock SetNX failed, mock key exist",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewBoolCmd(context.Background())
				status.SetVal(false)
				cmd.EXPECT().SetNX(context.Background(), "key", gomock.Any(), time.Minute).
					Return(status)
				return cmd
			},
			key:     "key",
			wantErr: errs.ErrFailedToPreemptLock,
		},
		{
			name: "tryLock SetNX successful",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewBoolCmd(context.Background())
				status.SetVal(true)
				cmd.EXPECT().SetNX(context.Background(), "key", gomock.Any(), time.Minute).
					Return(status)
				return cmd
			},
			key: "key",
			wantLock: &Lock{
				key: "key",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisLock := NewRedisLock(tc.mock(ctrl))
			lock, err := redisLock.TryLock(context.Background(), tc.key, time.Minute)
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantLock.key, lock.key)
			assert.NotEmpty(t, lock.uuid)
		})
	}
}

func TestLock_Unlock(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) redis.Cmdable
		key       string
		value     string
		wantError error
	}{
		{
			name: "call eval error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewCmd(context.Background())
				status.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().Eval(context.Background(), luaUnlock, []string{"key"}, []any{"value"}).
					Return(status)
				return cmd
			},
			key:       "key",
			value:     "value",
			wantError: context.DeadlineExceeded,
		},
		{
			name: "lock not hold",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewCmd(context.Background())
				status.SetVal(int64(0))
				cmd.EXPECT().Eval(context.Background(), luaUnlock, []string{"key"}, []any{"value"}).
					Return(status)
				return cmd
			},
			key:       "key",
			value:     "value",
			wantError: errs.ErrLockNotHold,
		},
		{
			name: "unlocked",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				status := redis.NewCmd(context.Background())
				status.SetVal(int64(1))
				cmd.EXPECT().Eval(context.Background(), luaUnlock, []string{"key"}, []any{"value"}).
					Return(status)
				return cmd
			},
			key:   "key",
			value: "value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			lock := &Lock{
				key:    tc.key,
				uuid:   tc.value,
				client: tc.mock(ctrl),
			}
			err := lock.Unlock(context.Background())
			assert.Equal(t, tc.wantError, err)
			if err != nil {
				return
			}
		})
	}
}

func TestLock_Refresh(t *testing.T) {
	testCases := []struct {
		name      string
		mock      func(ctrl *gomock.Controller) redis.Cmdable
		lock      *Lock
		wantError error
	}{
		{
			name: "call eval fail",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				ret := redis.NewCmd(context.Background())
				ret.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().Eval(context.Background(), luaRefresh, []string{"key"}, []any{"value", float64(60)}).
					Return(ret)
				return cmd
			},
			lock: &Lock{
				key:        "key",
				uuid:       "value",
				expiration: time.Minute,
			},
			wantError: context.DeadlineExceeded,
		},
		{
			name: "lock is other",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				ret := redis.NewCmd(context.Background())
				ret.SetVal(int64(0))
				cmd.EXPECT().Eval(context.Background(), luaRefresh, []string{"key"}, []any{"value", float64(60)}).
					Return(ret)
				return cmd
			},
			lock: &Lock{
				key:        "key",
				uuid:       "value",
				expiration: time.Minute,
			},
			wantError: errs.ErrLockNotHold,
		},
		{
			name: "refresh successful",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				ret := redis.NewCmd(context.Background())
				ret.SetVal(int64(1))
				cmd.EXPECT().Eval(context.Background(), luaRefresh, []string{"key"}, []any{"value", float64(60)}).
					Return(ret)
				return cmd
			},
			lock: &Lock{
				key:        "key",
				uuid:       "value",
				expiration: time.Minute,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			tc.lock.client = tc.mock(ctrl)
			err := tc.lock.Refresh(context.Background())
			assert.Equal(t, err, tc.wantError)
			if err != nil {
				return
			}
		})
	}
}

func ExampleLock_Refresh() {
	var lock *Lock
	errChan := make(chan error)
	timeoutChan := make(chan struct{})
	stopChan := make(chan struct{})
	// 使用一个协程专门用来续约
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		timeoutRetry := 0
		for {
			select {
			case <-ticker.C:
				// 定时续约
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				err := lock.Refresh(ctx)
				cancel()
				if errors.Is(err, context.DeadlineExceeded) {
					// redis命令运行超时，也需要续约
					timeoutChan <- struct{}{}
				}
				if err != nil {
					// 续约出现问题通知业务
					errChan <- err
					return
				}
				timeoutRetry = 0
			case <-stopChan:
				return
			case <-timeoutChan:
				timeoutRetry++
				if timeoutRetry > 10 {
					errChan <- context.DeadlineExceeded
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
				err := lock.Refresh(ctx)
				cancel()
				if errors.Is(err, context.DeadlineExceeded) {
					// redis命令运行超时，也需要续约
					timeoutChan <- struct{}{}
				}
				if err != nil {
					// 续约出现问题通知业务
					errChan <- err
					return
				}
			}
		}
	}()

	// 业务可以用循环处理
	for i := 0; i < 10; i++ {
		select {
		case <-errChan:
			break
		default:
			// 业务代码
		}
	}

	// 业务不可以用循环处理, 需要把业务拆分成几段
	select {
	case <-errChan:
		break
	default:
		// 业务代码 step1
	}

	select {
	case <-errChan:
		break
	default:
		// 业务代码 step2
	}

	select {
	case <-errChan:
		break
	default:
		// 业务代码 step3
	}

	//业务处理完成后，不需要续约了要关闭协程
	stopChan <- struct{}{}
}

func TestRedisLock_Lock(t *testing.T) {

}
