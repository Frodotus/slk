package main

import (
	"testing"
	"time"
)

// TestUserResolverFresh is the headline of the identity-TTL feature: the
// resolver re-fetches a cached identity once it ages past the TTL, and a
// zero TTL preserves the old "cache forever" behavior.
func TestUserResolverFresh(t *testing.T) {
	now := time.Now().Unix()
	day := int64(24 * 60 * 60)

	// TTL disabled (0): everything is fresh, nothing ever re-resolves.
	off := &userResolver{ttl: 0}
	if !off.fresh(0) || !off.fresh(now-365*day) {
		t.Error("ttl=0 must treat all identities as fresh (no refresh)")
	}

	r := &userResolver{ttl: 7 * 24 * time.Hour}
	if !r.fresh(now) {
		t.Error("just-synced identity must be fresh")
	}
	if !r.fresh(now - 6*day) {
		t.Error("6 days old must be fresh under a 7d TTL")
	}
	if r.fresh(now - 8*day) {
		t.Error("8 days old must be stale under a 7d TTL")
	}
	// updated_at=0 (never stamped) reads as ancient -> stale, so unstamped
	// rows refresh on next sight.
	if r.fresh(0) {
		t.Error("unstamped (updated_at=0) identity must be stale")
	}
}
