package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

// Workspace represents a tenant in the system.
type Workspace struct {
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
}

// Token represents an auth token for a workspace.
type Token struct {
	Token       string    `json:"token"`
	Workspace   string    `json:"workspace_handle"`
	Email       string    `json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}

// OTP represents a one-time password for auth.
type OTP struct {
	Email     string    `json:"email"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID        int       `json:"id"`
	Action    string    `json:"action"`
	Handle    string    `json:"handle,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Email     string    `json:"email,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// GenerateWorkspaceHandle creates a short handle like ws_abc12.
func GenerateWorkspaceHandle() string {
	b := make([]byte, 5)
	rand.Read(b)
	enc := base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").EncodeToString(b)
	return fmt.Sprintf("ws_%s", strings.ToLower(enc[:5]))
}

// GenerateToken creates a random auth token.
func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("fk_%s", base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").EncodeToString(b))
}

// GenerateOTPCode creates a 6-digit OTP code.
func GenerateOTPCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	code := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return fmt.Sprintf("%06d", code%1000000)
}
