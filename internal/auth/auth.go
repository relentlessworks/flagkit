package auth

import (
	"fmt"
	"time"

	"github.com/relentlessworks/flagkit/internal/model"
	"github.com/relentlessworks/flagkit/internal/store"
)

// Auth handles OTP-based authentication.
type Auth struct {
	store *store.Store
}

// New creates a new Auth instance.
func New(s *store.Store) *Auth {
	return &Auth{store: s}
}

// RequestOTP generates and stores an OTP for the given email.
// In dev mode (no SMTP), the code is returned for logging.
func (a *Auth) RequestOTP(email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	code := model.GenerateOTPCode()
	otp := &model.OTP{
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	if err := a.store.SaveOTP(otp); err != nil {
		return "", fmt.Errorf("save OTP: %w", err)
	}

	return code, nil
}

// VerifyOTP validates the OTP and returns a token.
// If the email doesn't have a workspace yet, one is created.
func (a *Auth) VerifyOTP(email, code string) (*model.Token, error) {
	if email == "" || code == "" {
		return nil, fmt.Errorf("email and code are required")
	}

	otp, err := a.store.GetOTP(email)
	if err != nil {
		return nil, fmt.Errorf("no OTP requested for this email")
	}

	if time.Now().After(otp.ExpiresAt) {
		a.store.DeleteOTP(email)
		return nil, fmt.Errorf("OTP has expired")
	}

	if otp.Code != code {
		return nil, fmt.Errorf("invalid OTP code")
	}

	a.store.DeleteOTP(email)

	// Find or create workspace for this email
	// For simplicity, each email gets its own workspace
	wsHandle := ""
	for handle, ws := range a.store.Data().Workspaces {
		_ = handle
		// Check if this email already has a token (existing user)
		_ = ws
	}

	// Create a new workspace if none exists for this email
	// We use email-based workspace lookup
	wsHandle = a.findOrCreateWorkspace(email)

	token := &model.Token{
		Token:     model.GenerateToken(),
		Workspace: wsHandle,
		Email:     email,
		CreatedAt: time.Now(),
	}

	if err := a.store.SaveToken(token); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	return token, nil
}

// ValidateToken checks if a token is valid and returns the associated workspace handle.
func (a *Auth) ValidateToken(token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("missing token")
	}

	t, err := a.store.GetToken(token)
	if err != nil {
		return "", fmt.Errorf("invalid token")
	}

	return t.Workspace, nil
}

// findOrCreateWorkspace finds a workspace by email or creates a new one.
func (a *Auth) findOrCreateWorkspace(email string) string {
	// Check existing tokens for this email
	for _, t := range a.store.Data().Tokens {
		if t.Email == email {
			return t.Workspace
		}
	}

	// Create new workspace
	wsHandle := model.GenerateWorkspaceHandle()
	ws := &model.Workspace{
		Handle:    wsHandle,
		Name:      email,
		Plan:      "free",
		CreatedAt: time.Now(),
	}
	_ = a.store.CreateWorkspace(ws)
	return wsHandle
}
