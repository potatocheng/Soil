package cache

import (
	"Soil/cache/internal/errs"
	"context"
	"sync"
	"time"
)

type item struct {
	key      string
	value    any
	deadline time.Time
	size     uint32
	prev     *item
	next     *item
}

func (i *item) deadlineBefore(t time.Time) bool {
	return !i.deadline.IsZero() && i.deadline.Before(t)
}

func initItem(key string, value any) *item {
	return &item{
		key:   key,
		value: value,
	}
}

type BuildInMapCache struct {
	data     map[string]*item
	rwMutex  sync.RWMutex
	close    chan struct{}
	head     *item
	tail     *item
	size     uint32
	capacity uint32

	// onEvicted 实现CDC(change data capture), 将数据的修改结果捕获
	onEvicted func(k string, v any)
}

type BuildInMapCacheOption func(cache *BuildInMapCache)

// NewBuildInMapCache capacity指的是设置的内存大小，单位是字节
func NewBuildInMapCache(interval time.Duration, capacity uint32, ops ...BuildInMapCacheOption) *BuildInMapCache {
	res := &BuildInMapCache{
		data:      make(map[string]*item, 100),
		close:     make(chan struct{}),
		capacity:  capacity,
		head:      initItem("head", nil),
		tail:      initItem("tail", nil),
		onEvicted: func(k string, v any) {},
	}

	// 初始化双向链表
	res.head.next = res.tail
	res.tail.prev = res.head

	for _, op := range ops {
		op(res)
	}

	// 这个goroutine负责每隔一段时间遍历缓存,将过期缓存删除掉,
	// 但是考虑到性能在缓存数量很多的情况下不可能遍历全部缓存
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case t := <-ticker.C:
				res.rwMutex.Lock()
				cnt := 0
				// 每次遍历map顺序是随机的，那么每个缓存都可能能遍历到
				for k, v := range res.data {
					if cnt > 1000 {
						break
					}
					if v.deadlineBefore(t) {
						res.delete(k)
					}
					cnt++
				}
				res.rwMutex.Unlock()
			case <-res.close:
				close(res.close)
				return
			}
		}
	}()

	return res
}

// Set expiration如果为0表示不设置超时时间
func (b *BuildInMapCache) Set(ctx context.Context, key string, val any,
	expiration time.Duration) error {
	b.rwMutex.Lock()
	defer b.rwMutex.Unlock()

	keySize, err := Of(key)
	if err != nil {
		return err
	}

	valSize, err := Of(val)
	if err != nil {
		return err
	}

	pairSize := keySize + valSize
	// 先判断缓冲中有没有node
	node, ok := b.data[key]
	if ok {
		// 缓存已经有node，只需要移动node双向链表最前面
		node.value = val
		b.moveToHead(node)
		err := b.set(key, val, expiration, pairSize)
		if err != nil {
			return err
		}
	} else {
		// 判断缓存满没满
		if b.size+uint32(pairSize) > b.capacity {
			// 满了就删除缓存
			margin := b.capacity - b.size
			// 从LRU队列从后往前遍历知道margin > pairSize
			for p := b.tail.prev; margin < uint32(pairSize); p = p.prev {
				margin += p.size
				n := b.removeTail()
				delete(b.data, n.key)
				b.size -= p.size
			}
		}
		// 没满就直接加入
		err = b.set(key, val, expiration, pairSize)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *BuildInMapCache) set(key string, val any, expiration time.Duration, pairSize uint32) error {
	var dl time.Time
	if expiration > 0 {
		dl = time.Now().Add(expiration)
	}

	node := &item{
		key:      key,
		value:    val,
		deadline: dl,
		size:     pairSize,
	}
	b.data[key] = node
	b.addToHead(b.data[key])
	b.size += pairSize
	return nil
}

// Get 在get数据时，如果数据过期会删除数据
func (b *BuildInMapCache) Get(ctx context.Context, key string) (any, error) {
	/*b.rwMutex.RLock()
	value, ok := b.data[key]
	b.rwMutex.RUnlock()
	// 这个方案不行，因为本来缓存过期了但是从这里到加写锁之间用户给重新set了expiration
	// 那么就会删除这个缓存
	if !ok {
		return nil, errs.NewErrKeyNotFound(key)
	}

	b.rwMutex.Lock()
	defer b.rwMutex.Unlock()
	if !value.deadline.IsZero() && value.deadline.Before(time.Now()) {
		delete(b.data, key)
		return nil, errs.NewErrKeyNotFound(key)
	}

	return value.value, nil*/
	b.rwMutex.RLock()
	node, ok := b.data[key]
	b.rwMutex.RUnlock()
	if !ok {
		return nil, errs.NewErrKeyNotFound(key)
	}
	//这里采用double-check防止注释方案里的问题
	now := time.Now()
	if node.deadlineBefore(now) {
		b.rwMutex.Lock()
		defer b.rwMutex.Unlock()
		node, ok = b.data[key]
		if !ok {
			return nil, errs.NewErrKeyNotFound(key)
		}
		if node.deadlineBefore(now) {
			b.delete(key)
			return nil, errs.NewErrKeyNotFound(key)
		}
		// 调整缓存顺序
		b.moveToHead(node)
	}

	return node.value, nil
}

func (b *BuildInMapCache) Delete(ctx context.Context, key string) error {
	b.rwMutex.Lock()
	defer b.rwMutex.Unlock()
	_, ok := b.data[key]
	if !ok {
		return errs.NewErrKeyNotFound(key)
	}
	b.delete(key)

	return nil
}

func (b *BuildInMapCache) delete(key string) {
	i, ok := b.data[key]
	if !ok {
		return
	}
	delete(b.data, key)
	b.onEvicted(key, i.value)
}

func (b *BuildInMapCache) Close() error {
	select {
	case b.close <- struct{}{}:
	default:
		return errs.ErrRepeatClose
	}

	return nil
}

func BuildInMapCacheWithEvictedCallback(onEvicted func(k string, v any)) BuildInMapCacheOption {
	return func(cache *BuildInMapCache) {
		cache.onEvicted = onEvicted
	}
}

func (b *BuildInMapCache) addToHead(node *item) {
	node.next = b.head.next
	node.prev = b.head
	b.head.next.prev = node
	b.head.next = node
}

func (b *BuildInMapCache) removeNode(node *item) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (b *BuildInMapCache) moveToHead(node *item) {
	b.removeNode(node)
	b.addToHead(node)
}
func (b *BuildInMapCache) removeTail() *item {
	node := b.tail.prev
	b.removeNode(node)
	return node
}
