package repositories

import "time"

// Session is the persistence shape stored under key "session:<id>".
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Data      string    `json:"data"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PasswordReset is the persistence shape stored under key "pwreset:<token>".
type PasswordReset struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
