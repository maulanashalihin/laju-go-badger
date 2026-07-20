package services

import (
	"context"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/maulanashalihin/laju-go/app/models"
	"github.com/maulanashalihin/laju-go/app/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuthTestDB(t *testing.T) *repositories.Repository {
	t.Helper()
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return repositories.NewRepository(db)
}

func newAuthService(t *testing.T, q *repositories.Repository) *AuthService {
	t.Helper()
	return NewAuthService(q, AuthServiceConfig{
		SessionSecret:      "test-secret-32-chars-long-for-testing!!",
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		GoogleRedirectURL:  "http://localhost:8080/auth/google/callback",
	})
}

func TestRegister(t *testing.T) {
	q := setupAuthTestDB(t)
	svc := newAuthService(t, q)

	// seed for duplicate test
	_, err := svc.Register("Existing", "dup@example.com", "pass123")
	require.NoError(t, err)

	tests := []struct {
		name    string
		email   string
		pass    string
		wantErr error
	}{
		{"success", "new@example.com", "password123", nil},
		{"duplicate email", "dup@example.com", "password456", ErrUserAlreadyExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := svc.Register("User", tt.email, tt.pass)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, user.ID)
			assert.Equal(t, tt.email, user.Email)
			assert.Equal(t, models.RoleUser, user.Role)
			assert.False(t, user.EmailVerified)
			assert.NotEqual(t, tt.pass, user.Password, "password must be hashed")
			assert.NotEmpty(t, user.Password)
		})
	}
}

func TestLogin(t *testing.T) {
	q := setupAuthTestDB(t)
	svc := newAuthService(t, q)

	// seed normal user
	_, err := svc.Register("Login User", "normal@example.com", "correct-password")
	require.NoError(t, err)

	// seed OAuth-only user (no password)
	err = q.CreateUser(context.Background(), &models.User{
		Email:    "oauth@example.com",
		Name:     "OAuth User",
		GoogleID: "google-oauth-1",
		Role:     models.RoleUser,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		email   string
		pass    string
		wantErr error
	}{
		{"success", "normal@example.com", "correct-password", nil},
		{"wrong password", "normal@example.com", "wrong", ErrInvalidCredentials},
		{"user not found", "nobody@example.com", "any", ErrInvalidCredentials},
		{"oauth-only user", "oauth@example.com", "any", ErrInvalidCredentials},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := svc.Login(tt.email, tt.pass)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.email, user.Email)
			assert.NotEmpty(t, user.ID)
		})
	}
}

func TestGetUserByID(t *testing.T) {
	q := setupAuthTestDB(t)
	svc := newAuthService(t, q)

	created, err := svc.Register("Found User", "found@example.com", "pass123")
	require.NoError(t, err)

	tests := []struct {
		name    string
		userID  string
		wantErr error
	}{
		{"found", created.ID, nil},
		{"not found", "01HNONEXISTENT", repositories.ErrUserNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := svc.GetUserByID(tt.userID)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, created.ID, user.ID)
			assert.Equal(t, "Found User", user.Name)
		})
	}
}

func TestPassword(t *testing.T) {
	t.Run("hash produces different salts", func(t *testing.T) {
		h1, _ := HashPassword("same-password")
		h2, _ := HashPassword("same-password")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("check correct password", func(t *testing.T) {
		hash, _ := HashPassword("correct")
		assert.True(t, CheckPassword("correct", hash))
	})

	t.Run("check wrong password", func(t *testing.T) {
		hash, _ := HashPassword("correct")
		assert.False(t, CheckPassword("wrong", hash))
	})
}

func TestOAuth(t *testing.T) {
	q := setupAuthTestDB(t)
	svc := newAuthService(t, q)

	t.Run("get config returns scopes", func(t *testing.T) {
		cfg := svc.GetOAuthConfig()
		assert.NotNil(t, cfg)
		assert.Equal(t, []string{"email", "profile"}, cfg.Scopes)
	})

	t.Run("get URL contains state and client ID", func(t *testing.T) {
		url := svc.GetOAuthURL("test-state")
		assert.Contains(t, url, "state=test-state")
		assert.Contains(t, url, "client_id=test-client-id")
	})

	t.Run("get OAuth URL format", func(t *testing.T) {
		url := svc.GetOAuthURL("test-state")
		assert.Contains(t, url, "accounts.google.com")
	})
}
