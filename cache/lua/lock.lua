local val = redis.call("GET", KEYS[1])
if  val == ARGV[1] then
    -- 锁已经存在且锁是该实例的,重置过期时间
    redis.call("EXPIRE", KEYS[1], ARGV[2])
    return "OK"
elseif val == false then
    -- 锁并不存在, 加锁
    return redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[2])
elseif val ~= ARGV[1] then
    -- 锁存在，但是锁被别人拿着
    return ""
end