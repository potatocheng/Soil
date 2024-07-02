package errs

import "github.com/pkg/errors"

var (
	ErrRepeatClose          = errors.New("cache: 重复关闭")
	ErrFailedToRefreshCache = errors.New("刷新缓存失败")
	ErrFailedToPreemptLock  = errors.New("redis-lock: 抢锁失败")
	ErrLockNotHold          = errors.New("redis-lock: 你没有持有锁")
)

func NewErrKeyNotFound(key string) error {
	return errors.Errorf("cache：键[%s]不存在\n", key)
}

func NewErrRedisSetFailed(msg string) error {
	return errors.Errorf("cache：写入 redis 失败，返回信息是：%s\n", msg)
}
