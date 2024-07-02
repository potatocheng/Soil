package cache

import "time"

type RetryStrategy interface {
	Next() (time.Duration, bool)
}

type FixedRetryStrategy struct {
	Interval time.Duration
	MaxCnt   int
	Cnt      int
}

func (f *FixedRetryStrategy) Next() (time.Duration, bool) {
	if f.Cnt >= f.MaxCnt {
		return 0, false
	}
	f.Cnt++
	return f.Interval, true
}
