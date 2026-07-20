package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/maulanashalihin/laju-go/app/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestUser(t *testing.T, q *Repository, ctx context.Context, email, name string) *models.User {
	t.Helper()
	user := &models.User{
		Email:    email,
		Name:     name,
		Password: "dummyhashdummyhashdummyhashdummyhash",
		Role:     models.RoleUser,
	}
	err := q.CreateUser(ctx, user)
	require.NoError(t, err)
	require.NotEmpty(t, user.ID)
	return user
}

func TestCreateUserAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "alice@example.com", "Alice")

	got, err := q.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, got.Email)
	assert.Equal(t, user.Name, got.Name)
	assert.Equal(t, models.RoleUser, got.Role)
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	createTestUser(t, q, ctx, "dup@example.com", "First")

	err := q.CreateUser(ctx, &models.User{
		Email:    "dup@example.com",
		Name:     "Second",
		Password: "hashed",
		Role:     models.RoleUser,
	})
	assert.ErrorIs(t, err, ErrUserAlreadyExists)
}

func TestGetUserByEmail(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	createTestUser(t, q, ctx, "bob@example.com", "Bob")

	got, err := q.GetUserByEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, "Bob", got.Name)

	_, err = q.GetUserByEmail(ctx, "nobody@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestGetUserByGoogleID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := &models.User{
		Email:         "google@example.com",
		Name:          "Google User",
		GoogleID:      "google-123",
		Avatar:        "https://example.com/avatar.jpg",
		EmailVerified: true,
		Role:          models.RoleUser,
	}
	err := q.CreateUserWithGoogleID(ctx, user)
	require.NoError(t, err)

	got, err := q.GetUserByGoogleID(ctx, "google-123")
	require.NoError(t, err)
	assert.Equal(t, user.Name, got.Name)

	_, err = q.GetUserByGoogleID(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUpdateUser(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "update@example.com", "Original")

	user.Name = "Updated"
	user.Avatar = "/storage/avatar.jpg"
	err := q.UpdateUser(ctx, user)
	require.NoError(t, err)

	got, err := q.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "/storage/avatar.jpg", got.Avatar)
}

func TestUpdateUserLinksGoogleID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "link@example.com", "Link")
	user.GoogleID = "google-link-1"
	require.NoError(t, q.UpdateUser(ctx, user))

	got, err := q.GetUserByGoogleID(ctx, "google-link-1")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
}

func TestUpdateUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	err := q.UpdateUser(ctx, &models.User{ID: "nonexistent", Name: "Ghost"})
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestDeleteUser(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "delete@example.com", "Delete Me")

	err := q.DeleteUser(ctx, user.ID)
	require.NoError(t, err)

	_, err = q.GetUserByID(ctx, user.ID)
	assert.ErrorIs(t, err, ErrUserNotFound)

	// Email index must also be gone so re-creating with same email works.
	err = q.CreateUser(ctx, &models.User{
		Email: "delete@example.com", Name: "Reborn",
		Password: "x", Role: models.RoleUser,
	})
	require.NoError(t, err)
}

func TestDeleteUserNotFound(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	err := q.DeleteUser(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestSetUserRoleAdmin(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "role@example.com", "Role Test")

	err := q.SetUserRoleAdmin(ctx, user.ID)
	require.NoError(t, err)

	got, err := q.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.RoleAdmin, got.Role)
}

func TestCreateAndGetSession(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "session@example.com", "Session User")

	sess := &Session{
		ID:        "test-session-id",
		UserID:    user.ID,
		Data:      fmt.Sprintf(`{"user_id":"%s","email":"session@example.com","role":"user"}`, user.ID),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err := q.CreateSession(ctx, sess)
	require.NoError(t, err)

	got, err := q.GetSessionByID(ctx, "test-session-id")
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)
	assert.Equal(t, sess.UserID, got.UserID)
}

func TestGetSessionExpired(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "expired@example.com", "Expired")

	sess := &Session{
		ID:        "expired-session",
		UserID:    user.ID,
		Data:      `{}`,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	require.NoError(t, q.CreateSession(ctx, sess))

	// GetSessionByID does not check expiry — the session store layer does.
	got, err := q.GetSessionByID(ctx, "expired-session")
	require.NoError(t, err)
	assert.Equal(t, "expired-session", got.ID)
	assert.Equal(t, user.ID, got.UserID)
}

func TestUpdateSession(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "session-update@example.com", "Update Session")

	sess := &Session{
		ID:        "update-session",
		UserID:    user.ID,
		Data:      `{}`,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, q.CreateSession(ctx, sess))

	sess.Data = `{"role":"admin"}`
	err := q.UpdateSession(ctx, sess)
	require.NoError(t, err)

	got, err := q.GetSessionByID(ctx, "update-session")
	require.NoError(t, err)
	assert.Contains(t, got.Data, "admin")
}

func TestGetSessionsByUserID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "multi-session@example.com", "Multi")

	for i := 0; i < 3; i++ {
		sess := &Session{
			ID:        fmt.Sprintf("multi-session-%d", i),
			UserID:    user.ID,
			Data:      `{}`,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		require.NoError(t, q.CreateSession(ctx, sess))
	}

	sessions, err := q.GetSessionsByUserID(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestDeleteSessionsByUserID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "delete-sessions@example.com", "Delete")

	sess := &Session{
		ID:        "to-delete",
		UserID:    user.ID,
		Data:      `{}`,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, q.CreateSession(ctx, sess))

	require.NoError(t, q.DeleteSessionsByUserID(ctx, user.ID))

	sessions, err := q.GetSessionsByUserID(ctx, user.ID)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestDeleteExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "expired-cleanup@example.com", "Cleanup")

	sess1 := &Session{
		ID:        "expired-1",
		UserID:    user.ID,
		Data:      `{}`,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	require.NoError(t, q.CreateSession(ctx, sess1))

	sess2 := &Session{
		ID:        "active-1",
		UserID:    user.ID,
		Data:      `{}`,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, q.CreateSession(ctx, sess2))

	require.NoError(t, q.DeleteExpiredSessions(ctx))

	sessions, err := q.GetSessionsByUserID(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "active-1", sessions[0].ID)
}

func TestCreateUserWithGoogleID(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := &models.User{
		Email:         "google-new@example.com",
		Name:          "Google New",
		GoogleID:      "google-new-456",
		Avatar:        "https://example.com/pic.jpg",
		EmailVerified: true,
		Role:          models.RoleUser,
	}
	err := q.CreateUserWithGoogleID(ctx, user)
	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)

	got, err := q.GetUserByGoogleID(ctx, "google-new-456")
	require.NoError(t, err)
	assert.Equal(t, "Google New", got.Name)
	assert.Equal(t, "https://example.com/pic.jpg", got.Avatar)
	assert.True(t, got.EmailVerified)
}

func TestUserRecordToModel(t *testing.T) {
	now := time.Now()
	rec := userRecord{
		ID:            "01HTEST",
		Email:         "test@example.com",
		Name:          "Test",
		Password:      "hashed",
		Avatar:        "/storage/avatar.jpg",
		Role:          "user",
		GoogleID:      "g-1",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	mUser := rec.toModel()
	assert.Equal(t, rec.ID, mUser.ID)
	assert.Equal(t, rec.Email, mUser.Email)
	assert.Equal(t, "/storage/avatar.jpg", mUser.Avatar)
	assert.Equal(t, "user", string(mUser.Role))
	assert.Equal(t, "g-1", mUser.GoogleID)
	assert.True(t, mUser.EmailVerified)
}

func TestPasswordResetLifecycle(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "reset@example.com", "Reset")

	token := "reset-token-abc"
	require.NoError(t, q.CreatePasswordReset(ctx, token, user.ID, user.Email, time.Now().Add(1*time.Hour)))

	pr, err := q.GetPasswordReset(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, pr.UserID)
	assert.False(t, pr.Used)

	require.NoError(t, q.MarkPasswordResetUsed(ctx, token))

	_, err = q.GetPasswordReset(ctx, token)
	assert.ErrorIs(t, err, ErrPasswordResetNotFound)
}

func TestPasswordResetExpired(t *testing.T) {
	db := setupTestDB(t)
	q := NewRepository(db)
	ctx := context.Background()

	user := createTestUser(t, q, ctx, "reset-exp@example.com", "Reset Exp")

	token := "expired-reset-token"
	require.NoError(t, q.CreatePasswordReset(ctx, token, user.ID, user.Email, time.Now().Add(-1*time.Hour)))

	_, err := q.GetPasswordReset(ctx, token)
	assert.ErrorIs(t, err, ErrPasswordResetNotFound)

	require.NoError(t, q.DeleteExpiredPasswordResets(ctx))
}
