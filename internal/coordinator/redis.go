package coordinator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// 命名空间前缀,所有 Redis key/channel 都加这个前缀避免和其他系统冲突
const redisNamespace = "maxx:"

// 活实例集合 key:sorted set,score 为心跳过期时间(unix 秒)
const redisInstanceSetKey = redisNamespace + "instances"

// redisCoordinator 是 Redis 实现
type redisCoordinator struct {
	instanceID string
	rdb        *redis.Client
}

type redisEnvelope struct {
	Sender  string `json:"sender"`
	Payload string `json:"payload"` // base64 编码,允许传二进制
}

// NewRedis 从 URL 构造 Redis Coordinator。URL 格式参考 go-redis 的 ParseURL,
// 例如 "redis://:password@host:6379/0"。
// 构造时会做一次 PING 验证连通,失败则返回 error,由调用方决定是否 fallback。
func NewRedis(ctx context.Context, url, instanceID string) (Coordinator, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &redisCoordinator{instanceID: instanceID, rdb: rdb}, nil
}

func (c *redisCoordinator) InstanceID() string { return c.instanceID }

// underlying 暴露 Redis 客户端给同包内的其他子系统(如 cooldown store)。
// 故意不放进 Coordinator 接口,避免污染通用抽象;memory 实现自然 nil。
func (c *redisCoordinator) underlying() *redis.Client { return c.rdb }

// RedisClient 返回 Coordinator 的底层 Redis 客户端,memory 实现返回 nil。
// 用于需要执行 Redis 专属操作的子系统(SetIfLater 的 Lua、SCAN 等)。
func RedisClient(c Coordinator) *redis.Client {
	if r, ok := c.(*redisCoordinator); ok {
		return r.underlying()
	}
	return nil
}

func (c *redisCoordinator) Publish(ctx context.Context, channel string, payload []byte) error {
	env := redisEnvelope{
		Sender:  c.instanceID,
		Payload: base64.StdEncoding.EncodeToString(payload),
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return c.rdb.Publish(ctx, redisNamespace+"pub:"+channel, data).Err()
}

func (c *redisCoordinator) Subscribe(ctx context.Context, channel string) (<-chan Message, error) {
	out := make(chan Message, 64)
	ps := c.rdb.Subscribe(ctx, redisNamespace+"pub:"+channel)
	// 确认订阅成功后再返回,失败则关闭 channel 并报错
	if _, err := ps.Receive(ctx); err != nil {
		_ = ps.Close()
		close(out)
		return out, err
	}

	var once sync.Once
	closeOut := func() { once.Do(func() { close(out) }) }

	go func() {
		defer closeOut()
		defer ps.Close()
		ch := ps.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case raw, ok := <-ch:
				if !ok {
					return
				}
				var env redisEnvelope
				if err := json.Unmarshal([]byte(raw.Payload), &env); err != nil {
					log.Printf("[Coordinator] discard malformed pubsub payload on %s: %v", channel, err)
					continue
				}
				payload, err := base64.StdEncoding.DecodeString(env.Payload)
				if err != nil {
					log.Printf("[Coordinator] discard pubsub payload with bad base64 on %s: %v", channel, err)
					continue
				}
				select {
				case out <- Message{Sender: env.Sender, Payload: payload}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func (c *redisCoordinator) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := c.rdb.Get(ctx, redisNamespace+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	return b, err
}

func (c *redisCoordinator) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl < 0 {
		ttl = 0
	}
	return c.rdb.Set(ctx, redisNamespace+key, value, ttl).Err()
}

func (c *redisCoordinator) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, redisNamespace+key).Err()
}

func (c *redisCoordinator) RegisterInstance(ctx context.Context, ttl time.Duration) error {
	score := float64(time.Now().Add(ttl).Unix())
	return c.rdb.ZAdd(ctx, redisInstanceSetKey, redis.Z{Score: score, Member: c.instanceID}).Err()
}

func (c *redisCoordinator) RefreshInstance(ctx context.Context, ttl time.Duration) error {
	return c.RegisterInstance(ctx, ttl)
}

func (c *redisCoordinator) UnregisterInstance(ctx context.Context) error {
	return c.rdb.ZRem(ctx, redisInstanceSetKey, c.instanceID).Err()
}

func (c *redisCoordinator) ListAliveInstances(ctx context.Context) ([]string, error) {
	now := time.Now().Unix()
	// 清掉已过期的成员(score < now)
	if err := c.rdb.ZRemRangeByScore(ctx, redisInstanceSetKey, "0", strconv.FormatInt(now, 10)).Err(); err != nil {
		return nil, err
	}
	// 剩下的就是活实例
	members, err := c.rdb.ZRange(ctx, redisInstanceSetKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (c *redisCoordinator) Close() error {
	return c.rdb.Close()
}
