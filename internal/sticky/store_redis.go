package sticky

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key schema:
//
//	maxx:v1:sticky:t<tenantID>:c<clientType>:p<projectID>:v<policyVer>:k<baseKey>
//
// clientType / baseKey are URL-escaped to keep ":" inside model names or
// session IDs from breaking the schema. Value is the provider ID as ASCII
// decimal. TTL is set on SET so Redis auto-expires entries.
const (
	redisStickyPrefix = "maxx:v1:sticky"
	redisStickyKeyFmt = redisStickyPrefix + ":t%d:c%s:p%d:v%s:k%s"
)

type redisStore struct {
	rdb *redis.Client
}

// NewRedisStore wraps a go-redis client into a sticky Store. The client's
// lifecycle is owned by the caller (typically shared with the coordinator's
// connection pool).
func NewRedisStore(rdb *redis.Client) Store {
	return &redisStore{rdb: rdb}
}

func keyName(k Key) string {
	return fmt.Sprintf(
		redisStickyKeyFmt,
		k.TenantID,
		url.QueryEscape(k.ClientType),
		k.ProjectID,
		url.QueryEscape(k.PolicyVer),
		url.QueryEscape(k.BaseKey),
	)
}

func (s *redisStore) Get(ctx context.Context, key Key) (uint64, bool, error) {
	v, err := s.rdb.Get(ctx, keyName(key)).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	id, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid sticky value %q: %w", v, err)
	}
	return id, true, nil
}

func (s *redisStore) Set(ctx context.Context, key Key, providerID uint64, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	return s.rdb.Set(ctx, keyName(key), strconv.FormatUint(providerID, 10), ttl).Err()
}

func (s *redisStore) Delete(ctx context.Context, key Key) error {
	return s.rdb.Del(ctx, keyName(key)).Err()
}
