package cache

// BloomFilterCache 使用布隆过滤器缓解缓存穿透问题
// 布隆过滤器只能告诉我们一个元素绝对不在集合内或可能在集合内
type BloomFilterCache struct {
	ReadThroughCache
}
