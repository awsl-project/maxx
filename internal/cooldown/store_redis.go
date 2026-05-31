package cooldown

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key schema 见 docs/multi-instance-rfc.md。所有 cooldown key 都加
// "maxx:v1:" 前缀,方便后续 schema 演进且不和其他系统冲突。
const (
	redisCooldownPrefix = "maxx:v1:cooldown"
	redisCooldownKeyFmt = redisCooldownPrefix + ":p:%d:c:%s:m:%s"
	redisCooldownGenFmt = redisCooldownPrefix + ":gen:p:%d"
	wildcardToken       = "*"
)

// redisCooldownStore 是 CooldownStore 的 Redis 实现。
//
// SetIfLater 用 Lua 脚本保证"读已有 until + 比较 + 写"的原子性;
// 否则两个实例并发 SetIfLater 同一个 key 可能丢失更晚的那个。
//
// value 编码:until 的 unix milliseconds(纯数字),解析快、传输小。
// TTL:until - now,redis 自动过期,无需 Manager 做 expired sweep。
type redisCooldownStore struct {
	rdb *redis.Client
}

// NewRedisCooldownStore wraps a go-redis client into a CooldownStore.
// 调用方负责传入的 client 的生命周期(通常和 coordinator 共享一个底层连接池
// 即可,这里不调用 Close)。
func NewRedisCooldownStore(rdb *redis.Client) CooldownStore {
	return &redisCooldownStore{rdb: rdb}
}

// keyName 把 CooldownKey 编码成 Redis key。空 clientType/model 用 "*"
// 表示通配。
//
// clientType / model 用 url.QueryEscape 转义,这样含 ":" 的值
// (如 "gpt-4:turbo" 这种模型名)不会破坏 parseKey 的分段逻辑。
// `*` (wildcardToken) 不被 url 编码,保持作为通配符。
func keyName(k CooldownKey) string {
	c := encodeKeyPart(k.ClientType)
	m := encodeKeyPart(k.Model)
	return fmt.Sprintf(redisCooldownKeyFmt, k.ProviderID, c, m)
}

func encodeKeyPart(v string) string {
	if v == "" {
		return wildcardToken
	}
	return url.QueryEscape(v)
}

func decodeKeyPart(v string) (string, bool) {
	if v == wildcardToken {
		return "", true
	}
	out, err := url.QueryUnescape(v)
	if err != nil {
		return "", false
	}
	return out, true
}

// parseKey 把 Redis key 解析回 CooldownKey。仅在 ListByProvider 中使用。
func parseKey(raw string) (CooldownKey, bool) {
	// 期望:maxx:v1:cooldown:p:<id>:c:<client>:m:<model>
	// clientType/model 经 url.QueryEscape 转义,所以分段安全。
	parts := strings.Split(raw, ":")
	if len(parts) != 9 || parts[0] != "maxx" || parts[1] != "v1" || parts[2] != "cooldown" {
		return CooldownKey{}, false
	}
	if parts[3] != "p" || parts[5] != "c" || parts[7] != "m" {
		return CooldownKey{}, false
	}
	pid, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		return CooldownKey{}, false
	}
	clientType, ok := decodeKeyPart(parts[6])
	if !ok {
		return CooldownKey{}, false
	}
	model, ok := decodeKeyPart(parts[8])
	if !ok {
		return CooldownKey{}, false
	}
	return CooldownKey{ProviderID: pid, ClientType: clientType, Model: model}, true
}

func (s *redisCooldownStore) Get(ctx context.Context, key CooldownKey) (time.Time, bool, error) {
	v, err := s.rdb.Get(ctx, keyName(key)).Result()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid cooldown value %q: %w", v, err)
	}
	t := time.UnixMilli(ms)
	if time.Now().After(t) {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func (s *redisCooldownStore) ListByProvider(ctx context.Context, providerID uint64) ([]CooldownStoreEntry, error) {
	pattern := fmt.Sprintf(redisCooldownKeyFmt, providerID, "*", "*")
	// 用 SCAN 避免 KEYS 在大库阻塞。每 provider 的 cooldown key 一般几个到几十个,
	// 单次扫描即可;再用 MGET 一次性取所有 value,避免 N+1 round trips
	// (尤其是 applyRemoteEvent 路径每次事件都会重 ListByProvider)。
	var (
		cursor uint64
		out    = make([]CooldownStoreEntry, 0)
	)
	now := time.Now()
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			values, err := s.rdb.MGet(ctx, keys...).Result()
			if err != nil {
				return nil, err
			}
			for i, raw := range keys {
				k, ok := parseKey(raw)
				if !ok {
					continue
				}
				v := values[i]
				if v == nil {
					continue // 刚好过期 / 失踪
				}
				str, ok := v.(string)
				if !ok {
					continue
				}
				ms, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					continue
				}
				t := time.UnixMilli(ms)
				if now.After(t) {
					continue
				}
				out = append(out, CooldownStoreEntry{Key: k, Until: t})
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return out, nil
}

func (s *redisCooldownStore) Set(ctx context.Context, key CooldownKey, until time.Time) error {
	ttl := time.Until(until)
	if ttl <= 0 {
		return nil // 已过期,不写入
	}
	return s.rdb.Set(ctx, keyName(key), until.UnixMilli(), ttl).Err()
}

// SetIfLater 用 Lua 保证原子性。逻辑:
//   - 读当前值
//   - 如果不存在 / 比新值严格早 → 写入并返回 1
//   - 否则返回 0
//
// 用 `>` 严格大于而不是 `>=`:两个实例在同一毫秒计算出相同 until
// 是常见情形(time.Now().Add(d) 经常对齐),如果同 until 也被拒绝,
// 第二个实例就完全跳过了 generation bump + publish 步骤,其他实例
// 也无法收到事件 → 状态分歧。允许同值"写入"虽然多一次 SET,但
// 保证了"凡发生 mutation 必产生事件"的不变量。
//
// PEXPIRE 用毫秒级 TTL,避免 redis 自动过期时精度损失影响并发判断。
var setIfLaterScript = redis.NewScript(`
local key = KEYS[1]
local newMs = tonumber(ARGV[1])
local current = redis.call("GET", key)
if current and tonumber(current) > newMs then
    return 0
end
local ttl = newMs - tonumber(ARGV[2])
if ttl <= 0 then
    return 0
end
redis.call("SET", key, newMs, "PX", ttl)
return 1
`)

func (s *redisCooldownStore) SetIfLater(ctx context.Context, key CooldownKey, until time.Time) (bool, error) {
	nowMs := time.Now().UnixMilli()
	res, err := setIfLaterScript.Run(ctx, s.rdb, []string{keyName(key)}, until.UnixMilli(), nowMs).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (s *redisCooldownStore) Delete(ctx context.Context, key CooldownKey) error {
	return s.rdb.Del(ctx, keyName(key)).Err()
}

func (s *redisCooldownStore) DeleteByProvider(ctx context.Context, providerID uint64) error {
	pattern := fmt.Sprintf(redisCooldownKeyFmt, providerID, "*", "*")
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *redisCooldownStore) GetGeneration(ctx context.Context, providerID uint64) (int64, error) {
	v, err := s.rdb.Get(ctx, fmt.Sprintf(redisCooldownGenFmt, providerID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return v, err
}

func (s *redisCooldownStore) BumpGeneration(ctx context.Context, providerID uint64) (int64, error) {
	return s.rdb.Incr(ctx, fmt.Sprintf(redisCooldownGenFmt, providerID)).Result()
}
