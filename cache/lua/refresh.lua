if redis.call('GET', KEYS[1]) == ARGV[1] then
    -- 是实例自己加的锁，可以延长加锁时间
    return redis.call('EXPIRE', KEYS[1], ARGV[2])
else
    -- 不是自己的所，不能解锁
    return 0
end