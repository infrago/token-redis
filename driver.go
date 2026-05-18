package tokenredis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/infrago/base"
	"github.com/infrago/token"
	"github.com/infrago/token/expire"
	"github.com/infrago/token/payloadcodec"
	"github.com/redis/go-redis/v9"
)

type redisDriver struct {
	mutex sync.Mutex

	addr     string
	username string
	password string
	db       int
	prefix   string
	timeout  time.Duration
	codec    string

	client *redis.Client
}

func init() {
	token.RegisterDriver("redis", &redisDriver{
		addr:    "127.0.0.1:6379",
		db:      0,
		prefix:  "infrago:token:",
		timeout: 5 * time.Second,
		codec:   payloadcodec.Default,
	})
}

func (d *redisDriver) Configure(setting Map) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if v, ok := setting["redis_addr"].(string); ok && strings.TrimSpace(v) != "" {
		d.addr = strings.TrimSpace(v)
	}
	if v, ok := setting["addr"].(string); ok && strings.TrimSpace(v) != "" {
		d.addr = strings.TrimSpace(v)
	}
	if v, ok := setting["redis_username"].(string); ok {
		d.username = strings.TrimSpace(v)
	}
	if v, ok := setting["username"].(string); ok {
		d.username = strings.TrimSpace(v)
	}
	if v, ok := setting["redis_password"].(string); ok {
		d.password = v
	}
	if v, ok := setting["password"].(string); ok {
		d.password = v
	}
	if v, ok := setting["redis_db"].(int); ok {
		d.db = v
	}
	if v, ok := setting["db"].(int); ok {
		d.db = v
	}
	if v, ok := setting["redis_db"].(int64); ok {
		d.db = int(v)
	}
	if v, ok := setting["db"].(int64); ok {
		d.db = int(v)
	}
	if v, ok := setting["redis_db"].(string); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			d.db = n
		}
	}
	if v, ok := setting["db"].(string); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			d.db = n
		}
	}
	if v, ok := setting["redis_prefix"].(string); ok && strings.TrimSpace(v) != "" {
		d.prefix = strings.TrimSpace(v)
	}
	if v, ok := setting["prefix"].(string); ok && strings.TrimSpace(v) != "" {
		d.prefix = strings.TrimSpace(v)
	}
	if timeout, ok := parseDurationSetting(setting["redis_timeout"]); ok {
		d.timeout = timeout
	}
	if timeout, ok := parseDurationSetting(setting["timeout"]); ok {
		d.timeout = timeout
	}
	if codec := payloadcodec.FromSetting(setting); codec != "" {
		d.codec = codec
	}
}

func (d *redisDriver) Open() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.client != nil {
		return nil
	}
	client := redis.NewClient(&redis.Options{
		Addr:     d.addr,
		Username: d.username,
		Password: d.password,
		DB:       d.db,
	})
	ctx, cancel := contextWithTimeout(d.timeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return err
	}
	d.client = client
	return nil
}

func (d *redisDriver) Close() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.client == nil {
		return nil
	}
	err := d.client.Close()
	d.client = nil
	return err
}

func (d *redisDriver) SavePayload(tokenID string, payload Map, exp int64) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	if expire.ExpiredUnix(exp) {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	bts, err := payloadcodec.Marshal(d.payloadCodec(), payload)
	if err != nil {
		return err
	}
	ctx, cancel := d.context()
	defer cancel()
	key := d.keyPayload(tokenID)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, bts, ttl).Err()
	}
	return client.Set(ctx, key, bts, 0).Err()
}

func (d *redisDriver) LoadPayload(tokenID string) (Map, bool, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil, false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return nil, false, err
	}
	ctx, cancel := d.context()
	defer cancel()
	raw, err := client.Get(ctx, d.keyPayload(tokenID)).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	out := Map{}
	if err := payloadcodec.Unmarshal(d.payloadCodec(), []byte(raw), &out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (d *redisDriver) DeletePayload(tokenID string) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	ctx, cancel := d.context()
	defer cancel()
	return client.Del(ctx, d.keyPayload(tokenID)).Err()
}

func (d *redisDriver) RevokeToken(token string, exp int64) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if expire.ExpiredUnix(exp) {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	ctx, cancel := d.context()
	defer cancel()
	key := d.keyRevokeToken(token)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, "1", ttl).Err()
	}
	return client.Set(ctx, key, "1", 0).Err()
}

func (d *redisDriver) RevokeTokenID(tokenID string, exp int64) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	if expire.ExpiredUnix(exp) {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	ctx, cancel := d.context()
	defer cancel()
	key := d.keyRevokeTokenID(tokenID)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, "1", ttl).Err()
	}
	return client.Set(ctx, key, "1", 0).Err()
}

func (d *redisDriver) RevokedToken(token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return false, err
	}
	ctx, cancel := d.context()
	defer cancel()
	n, err := client.Exists(ctx, d.keyRevokeToken(token)).Result()
	return n > 0, err
}

func (d *redisDriver) RevokedTokenID(tokenID string) (bool, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return false, err
	}
	ctx, cancel := d.context()
	defer cancel()
	n, err := client.Exists(ctx, d.keyRevokeTokenID(tokenID)).Result()
	return n > 0, err
}

func (d *redisDriver) ensureClient() (*redis.Client, error) {
	d.mutex.Lock()
	client := d.client
	d.mutex.Unlock()
	if client != nil {
		return client, nil
	}
	if err := d.Open(); err != nil {
		return nil, err
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.client, nil
}

func (d *redisDriver) keyPayload(tokenID string) string {
	return d.prefix + "payload:" + tokenID
}

func (d *redisDriver) keyRevokeToken(token string) string {
	return d.prefix + "revoke:token:" + hashToken(token)
}

func (d *redisDriver) keyRevokeTokenID(tokenID string) string {
	return d.prefix + "revoke:tokenid:" + tokenID
}

func (d *redisDriver) expireDuration(exp int64) time.Duration {
	return expire.DurationUntilUnix(exp)
}

func (d *redisDriver) payloadCodec() string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return payloadcodec.Normalize(d.codec)
}

func (d *redisDriver) context() (context.Context, context.CancelFunc) {
	d.mutex.Lock()
	timeout := d.timeout
	d.mutex.Unlock()
	return contextWithTimeout(timeout)
}

func contextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), timeout)
}

func parseDurationSetting(v Any) (time.Duration, bool) {
	switch vv := v.(type) {
	case time.Duration:
		return vv, true
	case string:
		vv = strings.TrimSpace(vv)
		if vv == "" {
			return 0, false
		}
		duration, err := time.ParseDuration(vv)
		if err == nil {
			return duration, true
		}
		seconds, err := strconv.ParseFloat(vv, 64)
		if err != nil {
			return 0, false
		}
		return time.Duration(seconds * float64(time.Second)), true
	case int:
		return time.Duration(vv) * time.Second, true
	case int8:
		return time.Duration(vv) * time.Second, true
	case int16:
		return time.Duration(vv) * time.Second, true
	case int32:
		return time.Duration(vv) * time.Second, true
	case int64:
		return time.Duration(vv) * time.Second, true
	case uint:
		return time.Duration(vv) * time.Second, true
	case uint8:
		return time.Duration(vv) * time.Second, true
	case uint16:
		return time.Duration(vv) * time.Second, true
	case uint32:
		return time.Duration(vv) * time.Second, true
	case uint64:
		return time.Duration(vv) * time.Second, true
	case float32:
		return time.Duration(float64(vv) * float64(time.Second)), true
	case float64:
		return time.Duration(vv * float64(time.Second)), true
	default:
		return 0, false
	}
}

func hashToken(token string) string {
	sum := sha1.Sum([]byte(token))
	return hex.EncodeToString(sum[:])
}
