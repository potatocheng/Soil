if redis.call('get', KEYS[1]) == ARGV[1] then
    -- 是实例自己加的锁，可以解锁, 即删掉这个键值对
    return redis.call('del', KEYS[1])
else
    -- 不是自己的所，不能解锁
    return 0
end