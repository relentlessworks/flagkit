package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// FlagType defines how a flag is evaluated.
type FlagType string

const (
	FlagTypeBoolean    FlagType = "boolean"
	FlagTypePercentage FlagType = "percentage"
	FlagTypeVariant    FlagType = "variant"
)

// Flag represents a feature flag.
type Flag struct {
	Handle      string    `json:"handle"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        FlagType  `json:"type"`
	Enabled     bool      `json:"enabled"`
	Percentage  int       `json:"percentage,omitempty"`
	Variants    []string  `json:"variants,omitempty"`
	DefaultVar  string    `json:"default_variant,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EvaluateResult holds the outcome of flag evaluation.
type EvaluateResult struct {
	Handle  string `json:"handle"`
	Enabled bool   `json:"enabled"`
	Variant string `json:"variant,omitempty"`
	Reason  string `json:"reason"`
}

var handleCounter uint64

// GenerateHandle creates a short, unique handle like flag_k7m2q.
func GenerateHandle() string {
	n := atomic.AddUint64(&handleCounter, 1)
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(n >> uint(i*8))
		}
	}
	enc := base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").EncodeToString(b)
	return fmt.Sprintf("flag_%s", strings.ToLower(enc[:5]))
}

// Validate checks that a flag's fields are valid for its type.
func (f *Flag) Validate() error {
	if f.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch f.Type {
	case FlagTypeBoolean:
	case FlagTypePercentage:
		if f.Percentage < 0 || f.Percentage > 100 {
			return fmt.Errorf("percentage must be 0-100")
		}
	case FlagTypeVariant:
		if len(f.Variants) == 0 {
			return fmt.Errorf("variants list cannot be empty for variant type")
		}
		if f.DefaultVar != "" {
			found := false
			for _, v := range f.Variants {
				if v == f.DefaultVar {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("default_variant must be one of the variants")
			}
		}
	default:
		return fmt.Errorf("invalid flag type: %s (must be boolean, percentage, or variant)", f.Type)
	}
	return nil
}

// Evaluate determines the flag state for a given context key.
func (f *Flag) Evaluate(contextKey string) EvaluateResult {
	if !f.Enabled {
		return EvaluateResult{
			Handle:  f.Handle,
			Enabled: false,
			Reason:  "flag is disabled",
		}
	}

	switch f.Type {
	case FlagTypeBoolean:
		return EvaluateResult{
			Handle:  f.Handle,
			Enabled: true,
			Reason:  "boolean flag is enabled",
		}

	case FlagTypePercentage:
		if f.Percentage >= 100 {
			return EvaluateResult{
				Handle:  f.Handle,
				Enabled: true,
				Reason:  "percentage is 100%",
			}
		}
		if f.Percentage <= 0 {
			return EvaluateResult{
				Handle:  f.Handle,
				Enabled: false,
				Reason:  "percentage is 0%",
			}
		}
		bucket := hashBucket(contextKey + f.Handle)
		if bucket < f.Percentage {
			return EvaluateResult{
				Handle:  f.Handle,
				Enabled: true,
				Reason:  fmt.Sprintf("context in %d%% rollout", f.Percentage),
			}
		}
		return EvaluateResult{
			Handle:  f.Handle,
			Enabled: false,
			Reason:  fmt.Sprintf("context not in %d%% rollout", f.Percentage),
		}

	case FlagTypeVariant:
		if len(f.Variants) == 0 {
			return EvaluateResult{
				Handle:  f.Handle,
				Enabled: false,
				Reason:  "no variants configured",
			}
		}
		bucket := hashBucket(contextKey + f.Handle)
		idx := bucket % len(f.Variants)
		return EvaluateResult{
			Handle:  f.Handle,
			Enabled: true,
			Variant: f.Variants[idx],
			Reason:  fmt.Sprintf("variant selected: %s", f.Variants[idx]),
		}

	default:
		return EvaluateResult{
			Handle:  f.Handle,
			Enabled: false,
			Reason:  "unknown flag type",
		}
	}
}

// hashBucket produces a 0-99 value from a string using FNV-1a.
func hashBucket(s string) int {
	if s == "" {
		s = "default"
	}
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return int(h % 100)
}
