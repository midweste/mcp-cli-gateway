package gateway

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/config"
)

// PromptHash returns the first 12 hex chars of the SHA256 of a prompt.
func PromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h)[:12]
}

// ParseDuration parses a --last style duration string (e.g., "1h", "2d", "30m").
// Returns 0 for empty input (meaning "lifetime").
func ParseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	s = strings.TrimSpace(strings.ToLower(s))

	if len(s) < 2 {
		// Try parsing as hours
		if v, err := parseFloat(s); err == nil {
			return time.Duration(v * float64(time.Hour))
		}
		return 0
	}

	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	v, err := parseFloat(numStr)
	if err != nil {
		return 0
	}

	switch suffix {
	case 'h':
		return time.Duration(v * float64(time.Hour))
	case 'd':
		return time.Duration(v * 24 * float64(time.Hour))
	case 'm':
		return time.Duration(v * float64(time.Minute))
	default:
		// Default to hours
		if vFull, err := parseFloat(s); err == nil {
			return time.Duration(vFull * float64(time.Hour))
		}
		return 0
	}
}

func parseFloat(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid number: %s", s)
	}
	return v, nil
}

// pickBucketAlternative finds an unused alias in the bucket, preferring higher
// index ("smarter") over lower. Returns "" if nothing available.
// Used by both single-dispatch (findBucketAlternative) and batch assignment.
func pickBucketAlternative(bucket []string, requested string, unavailable map[string]bool) string {
	reqIdx := indexOf(bucket, requested)
	if reqIdx < 0 {
		return ""
	}

	// Try smarter (higher index) first
	for i, m := range bucket {
		if i > reqIdx && !unavailable[m] {
			return m
		}
	}
	// Then lesser (lower index)
	for i, m := range bucket {
		if i < reqIdx && !unavailable[m] {
			return m
		}
	}
	return ""
}

// indexOf returns the index of item in slice, or -1.
func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

// FindBucketForModel returns the bucket containing the given alias, or nil.
func FindBucketForModel(cfg *config.Config, alias string) []string {
	for _, bucket := range cfg.ModelBuckets {
		for _, m := range bucket {
			if m == alias {
				return bucket
			}
		}
	}
	return nil
}
