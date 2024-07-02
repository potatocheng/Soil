package sync

import (
	"testing"
	"time"
)

func TestRWMutex(t *testing.T) {
	sm := &SafeMap[string, string]{
		data: make(map[string]string),
	}

	go func() {
		val, loaded := sm.LoadOrStore("key1", "value1")
		t.Logf("goroutine1, loaded %t, [val = %s]", loaded, val)
	}()

	go func() {
		val, loaded := sm.LoadOrStore("key1", "value2")
		t.Logf("goroutine2, loaded %t, [val = %s]", loaded, val)
	}()

	time.Sleep(time.Second)
}
