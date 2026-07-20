package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gofiber/fiber/v2"
	"github.com/maulanashalihin/laju-go/app/models"
	"github.com/maulanashalihin/laju-go/app/repositories"
	"github.com/maulanashalihin/laju-go/app/services"
	"github.com/maulanashalihin/laju-go/app/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestApp(t *testing.T) (*fiber.App, *repositories.Repository) {
	t.Helper()

	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	querier := repositories.NewRepository(db)
	store := session.New(querier, nil, 24*time.Hour)
	authSvc := services.NewAuthService(querier, services.AuthServiceConfig{
		SessionSecret: "test-secret-32-chars-long-for-testing!!",
	})
	inertiaSvc := services.NewInertiaService(services.NewAssetService("", "", false), store)

	app := fiber.New()
	h := NewAuthHandler(authSvc, store, inertiaSvc)
	app.Get("/login", h.ShowLoginForm)
	app.Get("/register", h.ShowRegisterForm)
	app.Post("/register", h.Register)
	app.Post("/login", h.Login)
	app.Get("/logout", h.Logout)

	return app, querier
}

func hashPW(t *testing.T, pw string) string {
	t.Helper()
	h, err := services.HashPassword(pw)
	require.NoError(t, err)
	return h
}

func TestShowForms(t *testing.T) {
	app, _ := setupTestApp(t)

	tests := []struct {
		path string
	}{
		{"/login"},
		{"/register"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestRegisterEndpoint(t *testing.T) {
	app, querier := setupTestApp(t)

	ctx := context.Background()
	err := querier.CreateUser(ctx, &models.User{
		Email:    "dup@example.com",
		Name:     "Existing",
		Password: hashPW(t, "x"),
		Role:     models.RoleUser,
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		body         string
		wantStatus   int
		wantLocation string
		wantSession  bool
	}{
		{
			"success", `{"name":"T","email":"a@b.com","password":"pass123"}`,
			http.StatusSeeOther, "/app", true,
		},
		{
			"empty fields", `{"name":"","email":"","password":""}`,
			http.StatusSeeOther, "/register", false,
		},
		{
			"duplicate email", `{"name":"T","email":"dup@example.com","password":"x"}`,
			http.StatusSeeOther, "/register", false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantLocation, resp.Header.Get("Location"))

			if tt.wantSession {
				hasSession := false
				for _, c := range resp.Header["Set-Cookie"] {
					if strings.HasPrefix(c, "session_id=") {
						hasSession = true
						break
					}
				}
				assert.True(t, hasSession, "response should set session cookie")
			}
		})
	}
}

func TestLoginEndpoint(t *testing.T) {
	app, querier := setupTestApp(t)

	ctx := context.Background()
	err := querier.CreateUser(ctx, &models.User{
		Email:    "user@example.com",
		Name:     "Test User",
		Password: hashPW(t, "pass123"),
		Role:     models.RoleUser,
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		body         string
		wantStatus   int
		wantLocation string
	}{
		{"success", `{"email":"user@example.com","password":"pass123"}`, http.StatusSeeOther, "/app"},
		{"wrong password", `{"email":"user@example.com","password":"wrong"}`, http.StatusSeeOther, "/login"},
		{"unknown user", `{"email":"nobody@example.com","password":"any"}`, http.StatusSeeOther, "/login"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, tt.wantLocation, resp.Header.Get("Location"))
		})
	}
}

func TestLogout(t *testing.T) {
	app, querier := setupTestApp(t)
	ctx := context.Background()

	err := querier.CreateUser(ctx, &models.User{
		Email:    "logout@example.com",
		Name:     "Logout User",
		Password: hashPW(t, "pw"),
		Role:     models.RoleUser,
	})
	require.NoError(t, err)

	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"logout@example.com","password":"pw"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, loginResp.StatusCode)

	var sessionCookie string
	for _, c := range loginResp.Header["Set-Cookie"] {
		if strings.HasPrefix(c, "session_id=") {
			sessionCookie = strings.Split(c, ";")[0]
			break
		}
	}
	require.NotEmpty(t, sessionCookie)

	logoutReq := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logoutReq.Header.Set("Cookie", sessionCookie)
	logoutResp, err := app.Test(logoutReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, logoutResp.StatusCode)
	assert.Equal(t, "/login", logoutResp.Header.Get("Location"))
}
