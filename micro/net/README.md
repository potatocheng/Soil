# micro/net

`micro/net` 是面向生产的轻量级 TCP 通信组件：二进制帧协议、**连接多路复用**、**多 endpoint 负载均衡**、应用层心跳、连接治理、超时取消、优雅 drain 关闭、限流与中间件。

## 协议格式

```
| Magic(2) | Version(1) | MsgType(1) | Flags(1) | Reserved(1) |
| Header Length(4) | Body Length(4) | Request ID(8) | Header | Body |
```

| 字段 | 说明 |
|---|---|
| Magic | `0x4D4E` |
| Version | `1` |
| MsgType | Request / Response / Heartbeat / Ping / Pong |
| Flags | `FlagOneWay` 单向；`FlagError` 错误响应 |
| RequestID | 多路复用关联请求与响应 |

## 架构要点

```
Client.Call
   └─ Balancer.Pick(endpoints)     # round_robin / random / least_active / weighted_rr
         └─ endpoint.streamPool.Get
               └─ stream.Call
                     ├─ writeMu 串行写帧
                     └─ pending[requestID] ← readLoop 分发

Server.handleConn
   ├─ 读请求 → go handleRequest（可并发）
   ├─ writeMu 串行写响应（支持客户端 mux）
   └─ Shutdown: CloseRead 半关闭 → 等 reqWg → 再 Close
```

## 快速开始

### Server

```go
handler := net.HandlerFunc(func(ctx context.Context, req *net.Request) (*net.Response, error) {
    return &net.Response{Body: req.Body}, nil
})

serv := net.NewServer("tcp", ":8080",
    net.WithHandler(handler),
    net.WithMiddleware(loggingMW),
)
go serv.Start()

ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
_ = serv.Shutdown(ctx) // drain 在途请求
```

### Client（单地址）

```go
cli := net.NewClient("tcp", "127.0.0.1:8080",
    net.WithMaxOpenConns(4),
    net.WithRequestTimeout(5*time.Second),
    net.WithHeartbeat(30*time.Second, 3*time.Second),
)
defer cli.Close()

resp, err := cli.Call(ctx, &net.Request{Body: []byte("hello")})
```

### Client（多地址负载均衡）

```go
// 逗号/分号分隔；@weight 可选
cli := net.NewClient("tcp", "10.0.0.1:8080@3,10.0.0.2:8080@1",
    net.WithBalancerPolicy(net.PolicyWeightedRoundRobin),
    net.WithRetry(2, 50*time.Millisecond),
    net.WithEndpointHealth(2, 5*time.Second), // 连续失败熔断
    net.WithHeartbeat(0, 0),
)

// 或追加 endpoint
cli = net.NewClient("tcp", "10.0.0.1:8080",
    net.WithEndpoints("10.0.0.2:8080", "10.0.0.3:8080@2"),
    net.WithBalancerPolicy(net.PolicyLeastActive),
)

// DNS 展开 hostname → 全部 A/AAAA
cli = net.NewClient("tcp", "api.internal:8080",
    net.WithDNSResolve(true),
    net.WithBalancerPolicy(net.PolicyRoundRobin),
)
// 运行时刷新解析结果
_ = cli.Resolve(ctx)

fmt.Println(cli.Endpoints())
for _, st := range cli.EndpointStats() {
    fmt.Println(st.Addr, st.Healthy, st.Pool.Open)
}
```

## 多 Endpoint 说明

| 能力 | 说明 |
|---|---|
| 地址解析 | `a:1,b:2` / `a:1;b:2` / 空白分隔；`host:port@weight` |
| `WithEndpoints` / `WithEndpointList` | 追加地址 |
| `WithDNSResolve` / `WithResolver` | DNS 或多源解析；`Resolve()` 可热更新 |
| 负载策略 | `round_robin`（默认）、`random`、`least_active`、`weighted_round_robin` |
| 健康熔断 | 连续失败达阈值后冷却，期间优先其它节点；全挂时降级再试 |
| 连接隔离 | **每个 endpoint 独立 stream 池**，坏节点不影响好节点连接 |

内置策略：

- **round_robin**：原子轮询  
- **random**：均匀随机  
- **least_active**：`reserved + pool inflight` 最小（选路即预占）  
- **weighted_round_robin**：Nginx 平滑加权  

自定义：`WithBalancer(yourBalancer)` 实现 `Pick([]*endpointNode)` 接口（包内 `Balancer`）。

## 配置项

### Client

| Option | 说明 |
|---|---|
| `WithDialTimeout` | 拨号超时 |
| `WithReadTimeout` / `WithWriteTimeout` | 读写超时 |
| `WithRequestTimeout` | 单次请求超时（与 ctx 取更早） |
| `WithKeepAlive` | TCP keepalive |
| `WithIdleTimeout` | stream 空闲回收 |
| `WithMaxConnLifetime` | stream 最大存活时间 |
| `WithMaxConns` / `WithMaxOpenConns` | **每个 endpoint** 的最大 stream 数 |
| `WithHeartbeat(interval, timeout)` | 应用层 Ping/Pong（interval≤0 关闭） |
| `WithMaxMessageSize` | Header/Body 上限 |
| `WithRetry` | 传输层重试（会换 endpoint） |
| `WithEndpoints` / `WithEndpointList` | 追加后端 |
| `WithBalancer` / `WithBalancerPolicy` | 负载均衡 |
| `WithDNSResolve` / `WithResolver` | 地址解析 |
| `WithEndpointHealth` | 熔断阈值与冷却 |
| `WithLogger` / `WithMetrics` / `WithTLSConfig` | 可观测与 TLS |

### Server

| Option | 说明 |
|---|---|
| `WithHandler` / `WithMiddleware` | 处理器与洋葱中间件 |
| `WithHandlerTimeout` | 单请求处理超时 |
| `WithServerIdleTimeout` 等 | 读/写/空闲超时 |
| `WithMaxConnections` | 最大并发连接 |
| `WithServerMaxMessageSize` | 帧大小上限 |
| `WithRateLimiter` | 限流 |
| `WithServerLogger` / `WithServerMetrics` / `WithServerTLSConfig` | 可观测与 TLS |

## 核心能力

1. **多路复用**：同一 TCP 上并发多个 `Call`，按 `RequestID` 匹配。
2. **多 endpoint LB**：解析 / DNS / 权重 / 熔断 / 热更新。
3. **应用层心跳**：空闲 stream Ping/Pong。
4. **连接池治理**：maxOpen、idle、lifetime、`Stats()` / `EndpointStats()`。
5. **超时与取消**：`context` + `WithRequestTimeout`。
6. **优雅 drain**：`Shutdown` 半关闭读侧，写完在途响应。
7. **错误可判定**：`errors.Is(err, ErrRemote|ErrRateLimited|ErrHandlerPanic)`。
8. **中间件 / 限流 / Metrics / Logger / TLS**。

## 错误处理

```go
resp, err := cli.Call(ctx, req)
if errors.Is(err, net.ErrRateLimited) { /* ... */ }
if errors.Is(err, net.ErrHandlerPanic) { /* ... */ }
if errors.Is(err, net.ErrRemote) { /* 任意远端 FlagError */ }
if errors.Is(err, net.ErrNoEndpoint) { /* 无可用节点 */ }
```
