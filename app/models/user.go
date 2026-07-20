package models

import (
	"time"
)

type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Avatar        string    `json:"avatar"`
	Password      string    `json:"-"` // Hashed password, never return in JSON (empty for OAuth users)
	Role          UserRole  `json:"role"`
	GoogleID      string    `json:"-"` // OAuth provider ID (empty for email/password users)
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName returns the logical collection name for User.
func (User) TableName() string {
	return "users"
}
