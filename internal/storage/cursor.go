package storage

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// Cursor pagination (§10.2.1): opaque URL-safe base64 of the last-seen row
// id; listing is keyset (`id < cursor`) ordered by id DESC.

func EncodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func DecodeCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil // 0 means "no cursor" — start from newest.
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return id, nil
}

// ClampLimit enforces the documented limit bounds (default 50, max 200).
func ClampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// BucketSeconds computes the downsampling bucket width (§10.1.4):
// range/maxPoints rounded up to the collection interval.
func BucketSeconds(rangeSeconds, maxPoints, intervalSeconds int) int {
	if maxPoints < 1 {
		maxPoints = 1
	}
	b := rangeSeconds / maxPoints
	if b < intervalSeconds {
		b = intervalSeconds
	}
	return b
}
