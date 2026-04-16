package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"hermes-agent/go/internal/config"
	"hermes-agent/go/internal/security"
	"hermes-agent/go/internal/store"
)

// Service manages local-user authentication and JWT issuance.
type Service struct {
	cfg       config.Config
	store     *store.Store
	jwtSecret []byte
}

// NewService creates the auth service and ensures a signing secret exists.
func NewService(cfg config.Config, st *store.Store) (*Service, error) {
	secret, err := loadOrCreateSecret(cfg.JWTSecretPath())
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, store: st, jwtSecret: secret}, nil
}

func loadOrCreateSecret(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		return raw, nil
	}
	secret, err := security.RandomSecret()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return nil, err
	}
	return []byte(secret), nil
}

// InitAdmin creates the initial local administrator account.
func (s *Service) InitAdmin(ctx context.Context, username, password string) error {
	if len(password) < s.cfg.Security.MinPasswordLength {
		return fmt.Errorf("password length must be at least %d", s.cfg.Security.MinPasswordLength)
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	if err := s.store.CreateUser(ctx, username, hash, "admin"); err != nil {
		return err
	}
	return s.store.WriteAudit(ctx, username, "init_admin", "administrator created")
}

// Login validates local credentials and returns a signed JWT on success.
func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.store.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("invalid username or password")
		}
		return "", err
	}
	now := time.Now().UTC()
	if user.LockedUntil.Valid && user.LockedUntil.Time.After(now) {
		return "", fmt.Errorf("account is temporarily locked until %s", user.LockedUntil.Time.Format(time.RFC3339))
	}
	if err := security.CheckPassword(user.PasswordHash, password); err != nil {
		failed := user.FailedAttempts + 1
		var lockUntil *time.Time
		if failed >= s.cfg.Security.MaxLoginAttempts {
			until := now.Add(time.Duration(s.cfg.Security.LoginWindowMinute) * time.Minute)
			lockUntil = &until
			failed = 0
		}
		_ = s.store.UpdateLoginFailure(ctx, username, failed, lockUntil)
		_ = s.store.WriteAudit(ctx, username, "login_failed", "invalid credentials")
		return "", fmt.Errorf("invalid username or password")
	}
	if err := s.store.ResetLoginFailures(ctx, username); err != nil {
		return "", err
	}
	token, err := security.SignJWT(s.jwtSecret, s.cfg.JWTIssuer, user.Username, user.Role, s.cfg.JWTExpiry())
	if err != nil {
		return "", err
	}
	_ = s.store.WriteAudit(ctx, username, "login_success", "token issued")
	return token, nil
}

// ParseToken validates and parses a signed JWT.
func (s *Service) ParseToken(token string) (*security.Claims, error) {
	return security.ParseJWT(s.jwtSecret, token)
}
