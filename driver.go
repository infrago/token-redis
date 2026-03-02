package tokenredis

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
	"github.com/infrago/token"
	"github.com/redis/go-redis/v9"
)

type redisDriver struct {
	mutex sync.Mutex

	addr     string
	username string
	password string
	db       int
	prefix   string

	client *redis.Client
}

func init() {
	token.RegisterDriver("redis", &redisDriver{
		addr:   "127.0.0.1:6379",
		db:     0,
		prefix: "infrago:token:",
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
	if err := client.Ping(context.Background()).Err(); err != nil {
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

func (d *redisDriver) SavePayload(_ *infra.Meta, tokenID string, payload Map, exp int64) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	bts, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx := context.Background()
	key := d.keyPayload(tokenID)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, bts, ttl).Err()
	}
	return client.Set(ctx, key, bts, 0).Err()
}

func (d *redisDriver) LoadPayload(_ *infra.Meta, tokenID string) (Map, bool, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil, false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return nil, false, err
	}
	ctx := context.Background()
	raw, err := client.Get(ctx, d.keyPayload(tokenID)).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	out := Map{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (d *redisDriver) DeletePayload(_ *infra.Meta, tokenID string) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	return client.Del(context.Background(), d.keyPayload(tokenID)).Err()
}

func (d *redisDriver) RevokeToken(_ *infra.Meta, token string, exp int64) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	key := d.keyRevokeToken(token)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, "1", ttl).Err()
	}
	return client.Set(ctx, key, "1", 0).Err()
}

func (d *redisDriver) RevokeTokenID(_ *infra.Meta, tokenID string, exp int64) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	key := d.keyRevokeTokenID(tokenID)
	if ttl := d.expireDuration(exp); ttl > 0 {
		return client.Set(ctx, key, "1", ttl).Err()
	}
	return client.Set(ctx, key, "1", 0).Err()
}

func (d *redisDriver) RevokedToken(_ *infra.Meta, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return false, err
	}
	n, err := client.Exists(context.Background(), d.keyRevokeToken(token)).Result()
	return n > 0, err
}

func (d *redisDriver) RevokedTokenID(_ *infra.Meta, tokenID string) (bool, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return false, nil
	}
	client, err := d.ensureClient()
	if err != nil {
		return false, err
	}
	n, err := client.Exists(context.Background(), d.keyRevokeTokenID(tokenID)).Result()
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
	if exp <= 0 {
		return 0
	}
	delta := time.Until(time.Unix(exp, 0))
	if delta <= 0 {
		return time.Second
	}
	return delta
}

func hashToken(token string) string {
	sum := sha1.Sum([]byte(token))
	return hex.EncodeToString(sum[:])
}

