package tokenredis

import (
	"testing"
	"time"

	. "github.com/infrago/base"
	"github.com/infrago/token/expire"
)

func TestExpiredUnix(t *testing.T) {
	if !expire.ExpiredUnix(time.Now().Add(-time.Second).Unix()) {
		t.Fatalf("expected past unix timestamp to be expired")
	}
	if expire.ExpiredUnix(time.Now().Unix()) {
		t.Fatalf("current unix second should remain valid")
	}
}

func TestExpireDurationKeepsCurrentSecond(t *testing.T) {
	driver := &redisDriver{}

	ttl := driver.expireDuration(time.Now().Unix())
	if ttl <= 0 {
		t.Fatalf("expected positive ttl for current unix second, got %v", ttl)
	}
}

func TestConfigureTimeout(t *testing.T) {
	driver := &redisDriver{timeout: 5 * time.Second}

	driver.Configure(Map{"timeout": "250ms"})
	if driver.timeout != 250*time.Millisecond {
		t.Fatalf("expected string duration timeout, got %v", driver.timeout)
	}

	driver.Configure(Map{"redis_timeout": int64(2)})
	if driver.timeout != 2*time.Second {
		t.Fatalf("expected numeric second timeout, got %v", driver.timeout)
	}
}

func TestContextWithTimeoutCanBeDisabled(t *testing.T) {
	driver := &redisDriver{timeout: 0}

	ctx, cancel := driver.context()
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Fatalf("expected no deadline when timeout is disabled")
	}
}

func TestConfigurePayloadCodec(t *testing.T) {
	driver := &redisDriver{}

	driver.Configure(Map{"codec": "ignored", "driver_codec": "custom"})
	if driver.payloadCodec() != "custom" {
		t.Fatalf("expected custom payload codec, got %q", driver.payloadCodec())
	}
}
