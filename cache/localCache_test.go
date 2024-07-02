package cache

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func traverse(cache *BuildInMapCache) {
	for p := cache.head; p != cache.tail.next; p = p.next {
		if p != cache.head {
			fmt.Print(", ")
		}
		fmt.Print("<", p.key, ", ", p.value, ">")
	}
	fmt.Println()
}

func TestBuildInMapCache_Set(t *testing.T) {
	cache := NewBuildInMapCache(10, 60)
	err := cache.Set(context.Background(), "key1", 1, 0)
	require.NoError(t, err)
	traverse(cache)

	err = cache.Set(context.Background(), "key2", 2, 0)
	require.NoError(t, err)
	traverse(cache)

	err = cache.Set(context.Background(), "key3", 3, 0)
	require.NoError(t, err)
	traverse(cache)

	time.Sleep(time.Second * 6)

	res, err := cache.Get(context.Background(), "key1")
	require.NoError(t, err)
	require.Equal(t, res, 2)
}
