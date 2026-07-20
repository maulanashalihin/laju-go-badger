package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/maulanashalihin/laju-go/app/models"
)

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrUserAlreadyExists     = errors.New("user already exists")
	ErrSessionNotFound       = errors.New("session not found")
	ErrPasswordResetNotFound = errors.New("password reset not found")
)

// userRecord is the JSON-serializable persistence shape for a user.
// models.User keeps Password/GoogleID as `json:"-"` so they never leak into
// API responses; this record explicitly serializes every field for storage.
type userRecord struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Password      string    `json:"password,omitempty"`
	Avatar        string    `json:"avatar,omitempty"`
	Role          string    `json:"role"`
	GoogleID      string    `json:"google_id,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (r *userRecord) toModel() *models.User {
	return &models.User{
		ID:            r.ID,
		Email:         r.Email,
		Name:          r.Name,
		Password:      r.Password,
		Avatar:        r.Avatar,
		Role:          models.UserRole(r.Role),
		GoogleID:      r.GoogleID,
		EmailVerified: r.EmailVerified,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func userKey(id string) []byte       { return []byte(keyUserPrefix + id) }
func userEmailKey(email string) []byte { return []byte(keyUserEmailIndex + email) }
func userGoogleKey(gid string) []byte  { return []byte(keyUserGoogleIndex + gid) }
func sessionKey(id string) []byte      { return []byte(keySessionPrefix + id) }
func sessionUserKey(userID, sid string) []byte {
	return []byte(keySessionUserIndex + userID + ":" + sid)
}
func passwordResetKey(token string) []byte { return []byte(keyPasswordReset + token) }

// --- User operations ---

// CreateUser inserts a new email/password user with a fresh ULID.
// Returns ErrUserAlreadyExists if the email is already registered.
func (q *Repository) CreateUser(ctx context.Context, user *models.User) error {
	return q.update(func(txn *badger.Txn) error {
		if _, err := txn.Get(userEmailKey(user.Email)); err == nil {
			return ErrUserAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		id, err := newULID()
		if err != nil {
			return err
		}
		now := time.Now()
		rec := userRecord{
			ID:            id,
			Email:         user.Email,
			Name:          user.Name,
			Password:      user.Password,
			Role:          string(user.Role),
			EmailVerified: user.EmailVerified,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := txn.Set(userKey(id), buf); err != nil {
			return err
		}
		if err := txn.Set(userEmailKey(user.Email), []byte(id)); err != nil {
			return err
		}
		user.ID = id
		user.CreatedAt = now
		user.UpdatedAt = now
		return nil
	})
}

// CreateUserWithGoogleID inserts a new OAuth user with a fresh ULID and a
// google_id index. Returns ErrUserAlreadyExists on email collision.
func (q *Repository) CreateUserWithGoogleID(ctx context.Context, user *models.User) error {
	return q.update(func(txn *badger.Txn) error {
		if _, err := txn.Get(userEmailKey(user.Email)); err == nil {
			return ErrUserAlreadyExists
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if user.GoogleID != "" {
			if _, err := txn.Get(userGoogleKey(user.GoogleID)); err == nil {
				return ErrUserAlreadyExists
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}

		id, err := newULID()
		if err != nil {
			return err
		}
		now := time.Now()
		rec := userRecord{
			ID:            id,
			Email:         user.Email,
			Name:          user.Name,
			Avatar:        user.Avatar,
			GoogleID:      user.GoogleID,
			EmailVerified: user.EmailVerified,
			Role:          string(user.Role),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := txn.Set(userKey(id), buf); err != nil {
			return err
		}
		if err := txn.Set(userEmailKey(user.Email), []byte(id)); err != nil {
			return err
		}
		if user.GoogleID != "" {
			if err := txn.Set(userGoogleKey(user.GoogleID), []byte(id)); err != nil {
				return err
			}
		}
		user.ID = id
		user.CreatedAt = now
		user.UpdatedAt = now
		return nil
	})
}

func (q *Repository) getUserByIDTxn(txn *badger.Txn, id string) (*models.User, error) {
	val, err := txnGet(txn, userKey(id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	var rec userRecord
	if err := json.Unmarshal(val, &rec); err != nil {
		return nil, err
	}
	return rec.toModel(), nil
}

// GetUserByID fetches a user by its ULID.
func (q *Repository) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	var u *models.User
	err := q.view(func(txn *badger.Txn) error {
		var inner error
		u, inner = q.getUserByIDTxn(txn, id)
		return inner
	})
	return u, err
}

// GetUserByEmail fetches a user via the email index.
func (q *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u *models.User
	err := q.view(func(txn *badger.Txn) error {
		idVal, err := txnGet(txn, userEmailKey(email))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var inner error
		u, inner = q.getUserByIDTxn(txn, string(idVal))
		return inner
	})
	return u, err
}

// GetUserByGoogleID fetches a user via the google_id index.
func (q *Repository) GetUserByGoogleID(ctx context.Context, googleID string) (*models.User, error) {
	var u *models.User
	err := q.view(func(txn *badger.Txn) error {
		idVal, err := txnGet(txn, userGoogleKey(googleID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var inner error
		u, inner = q.getUserByIDTxn(txn, string(idVal))
		return inner
	})
	return u, err
}

// UpdateUser persists mutable fields (name, avatar, email_verified, google_id).
// Email is immutable here. The google_id index is reconciled when it changes.
func (q *Repository) UpdateUser(ctx context.Context, user *models.User) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, userKey(user.ID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var rec userRecord
		if err := json.Unmarshal(val, &rec); err != nil {
			return err
		}

		oldGoogleID := rec.GoogleID
		rec.Name = user.Name
		rec.Avatar = user.Avatar
		rec.EmailVerified = user.EmailVerified
		rec.GoogleID = user.GoogleID
		rec.UpdatedAt = time.Now()

		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := txn.Set(userKey(user.ID), buf); err != nil {
			return err
		}

		// Reconcile google_id index only when it actually changed.
		if oldGoogleID != rec.GoogleID {
			if oldGoogleID != "" {
				_ = txn.Delete(userGoogleKey(oldGoogleID))
			}
			if rec.GoogleID != "" {
				if err := txn.Set(userGoogleKey(rec.GoogleID), []byte(user.ID)); err != nil {
					return err
				}
			}
		}
		user.UpdatedAt = rec.UpdatedAt
		return nil
	})
}

// UpdateUserPassword sets the hashed password for a user.
func (q *Repository) UpdateUserPassword(ctx context.Context, id string, hashedPassword string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, userKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var rec userRecord
		if err := json.Unmarshal(val, &rec); err != nil {
			return err
		}
		rec.Password = hashedPassword
		rec.UpdatedAt = time.Now()
		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return txn.Set(userKey(id), buf)
	})
}

// UpdateUserAvatar sets the avatar URL for a user.
func (q *Repository) UpdateUserAvatar(ctx context.Context, id string, avatarURL string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, userKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var rec userRecord
		if err := json.Unmarshal(val, &rec); err != nil {
			return err
		}
		rec.Avatar = avatarURL
		rec.UpdatedAt = time.Now()
		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return txn.Set(userKey(id), buf)
	})
}

// DeleteUser removes a user and its email/google indexes.
func (q *Repository) DeleteUser(ctx context.Context, id string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, userKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var rec userRecord
		if err := json.Unmarshal(val, &rec); err != nil {
			return err
		}
		if err := txn.Delete(userKey(id)); err != nil {
			return err
		}
		_ = txn.Delete(userEmailKey(rec.Email))
		if rec.GoogleID != "" {
			_ = txn.Delete(userGoogleKey(rec.GoogleID))
		}
		return nil
	})
}

// SetUserRoleAdmin promotes a user to the admin role.
func (q *Repository) SetUserRoleAdmin(ctx context.Context, id string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, userKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		var rec userRecord
		if err := json.Unmarshal(val, &rec); err != nil {
			return err
		}
		rec.Role = string(models.RoleAdmin)
		rec.UpdatedAt = time.Now()
		buf, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return txn.Set(userKey(id), buf)
	})
}

// --- Session operations ---

// CreateSession stores a session and its per-user index key.
func (q *Repository) CreateSession(ctx context.Context, session *Session) error {
	now := time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now
	buf, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return q.update(func(txn *badger.Txn) error {
		if err := txn.Set(sessionKey(session.ID), buf); err != nil {
			return err
		}
		return txn.Set(sessionUserKey(session.UserID, session.ID), nil)
	})
}

// GetSessionByID fetches a session by ID. Expiry is NOT checked here — the
// session store layer handles expiry semantics.
func (q *Repository) GetSessionByID(ctx context.Context, id string) (*Session, error) {
	var s *Session
	err := q.view(func(txn *badger.Txn) error {
		val, err := txnGet(txn, sessionKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrSessionNotFound
			}
			return err
		}
		s = &Session{}
		return json.Unmarshal(val, s)
	})
	return s, err
}

// GetSessionsByUserID returns all non-expired sessions for a user.
func (q *Repository) GetSessionsByUserID(ctx context.Context, userID string) ([]*Session, error) {
	var sessions []*Session
	prefix := []byte(keySessionUserIndex + userID + ":")
	err := q.view(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
		defer iter.Close()
		now := time.Now()
		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			key := iter.Item().Key()
			// key = idx:sess:u:<uid>:<sid> — split off sid
			sep := -1
			for i := len(key) - 1; i >= 0; i-- {
				if key[i] == ':' {
					sep = i
					break
				}
			}
			if sep < 0 {
				continue
			}
			sid := string(key[sep+1:])
			val, gerr := txnGet(txn, sessionKey(sid))
			if gerr != nil {
				continue
			}
			var s Session
			if jerr := json.Unmarshal(val, &s); jerr != nil {
				continue
			}
			if s.ExpiresAt.After(now) {
				sessions = append(sessions, &s)
			}
		}
		return nil
	})
	return sessions, err
}

// UpdateSession overwrites the session payload. The per-user index key is
// stable (userID does not change), so it is left untouched.
func (q *Repository) UpdateSession(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()
	buf, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return q.update(func(txn *badger.Txn) error {
		if _, err := txn.Get(sessionKey(session.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrSessionNotFound
			}
			return err
		}
		return txn.Set(sessionKey(session.ID), buf)
	})
}

// DeleteSession removes a session and its per-user index.
func (q *Repository) DeleteSession(ctx context.Context, id string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, sessionKey(id))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrSessionNotFound
			}
			return err
		}
		var s Session
		if err := json.Unmarshal(val, &s); err != nil {
			return err
		}
		if err := txn.Delete(sessionKey(id)); err != nil {
			return err
		}
		_ = txn.Delete(sessionUserKey(s.UserID, id))
		return nil
	})
}

// DeleteSessionsByUserID removes every session belonging to a user.
func (q *Repository) DeleteSessionsByUserID(ctx context.Context, userID string) error {
	prefix := []byte(keySessionUserIndex + userID + ":")
	return q.update(func(txn *badger.Txn) error {
		// Collect index keys first — mutating during forward iteration is unsafe.
		type pair struct{ idxKey, sid []byte }
		var pairs []pair
		iter := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			key := append([]byte{}, iter.Item().Key()...)
			sep := -1
			for i := len(key) - 1; i >= 0; i-- {
				if key[i] == ':' {
					sep = i
					break
				}
			}
			if sep < 0 {
				continue
			}
			pairs = append(pairs, pair{idxKey: key, sid: key[sep+1:]})
		}
		iter.Close()

		for _, p := range pairs {
			_ = txn.Delete(p.idxKey)
			_ = txn.Delete(sessionKey(string(p.sid)))
		}
		return nil
	})
}

// DeleteExpiredSessions removes all sessions past their expiry.
func (q *Repository) DeleteExpiredSessions(ctx context.Context) error {
	prefix := []byte(keySessionPrefix)
	now := time.Now()
	return q.update(func(txn *badger.Txn) error {
		type expired struct {
			id, userID string
		}
		var toDelete []expired
		iter := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			val, err := iter.Item().ValueCopy(nil)
			if err != nil {
				continue
			}
			var s Session
			if err := json.Unmarshal(val, &s); err != nil {
				continue
			}
			if s.ExpiresAt.Before(now) {
				toDelete = append(toDelete, expired{id: s.ID, userID: s.UserID})
			}
		}
		iter.Close()

		for _, e := range toDelete {
			_ = txn.Delete(sessionKey(e.id))
			_ = txn.Delete(sessionUserKey(e.userID, e.id))
		}
		return nil
	})
}

// --- Password reset operations ---

// CreatePasswordReset stores a reset token (unused, with expiry).
func (q *Repository) CreatePasswordReset(ctx context.Context, token string, userID string, email string, expiresAt time.Time) error {
	pr := PasswordReset{
		Token:     token,
		UserID:    userID,
		Email:     email,
		ExpiresAt: expiresAt,
		Used:      false,
		CreatedAt: time.Now(),
	}
	buf, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	return q.update(func(txn *badger.Txn) error {
		return txn.Set(passwordResetKey(token), buf)
	})
}

// GetPasswordReset returns a valid (unused, not expired) reset entry.
func (q *Repository) GetPasswordReset(ctx context.Context, token string) (*PasswordReset, error) {
	var pr *PasswordReset
	err := q.view(func(txn *badger.Txn) error {
		val, err := txnGet(txn, passwordResetKey(token))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrPasswordResetNotFound
			}
			return err
		}
		var p PasswordReset
		if err := json.Unmarshal(val, &p); err != nil {
			return err
		}
		if p.Used || p.ExpiresAt.Before(time.Now()) {
			return ErrPasswordResetNotFound
		}
		pr = &p
		return nil
	})
	return pr, err
}

// MarkPasswordResetUsed flags a reset token as consumed.
func (q *Repository) MarkPasswordResetUsed(ctx context.Context, token string) error {
	return q.update(func(txn *badger.Txn) error {
		val, err := txnGet(txn, passwordResetKey(token))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrPasswordResetNotFound
			}
			return err
		}
		var p PasswordReset
		if err := json.Unmarshal(val, &p); err != nil {
			return err
		}
		p.Used = true
		buf, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return txn.Set(passwordResetKey(token), buf)
	})
}

// DeleteExpiredPasswordResets removes all reset tokens past their expiry.
func (q *Repository) DeleteExpiredPasswordResets(ctx context.Context) error {
	prefix := []byte(keyPasswordReset)
	now := time.Now()
	return q.update(func(txn *badger.Txn) error {
		var keys [][]byte
		iter := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
		for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
			val, err := iter.Item().ValueCopy(nil)
			if err != nil {
				continue
			}
			var p PasswordReset
			if err := json.Unmarshal(val, &p); err != nil {
				continue
			}
			if p.ExpiresAt.Before(now) {
				keys = append(keys, append([]byte{}, iter.Item().Key()...))
			}
		}
		iter.Close()
		for _, k := range keys {
			_ = txn.Delete(k)
		}
		return nil
	})
}

// DecodeSessionData parses a JSON session data blob.
func (q *Repository) DecodeSessionData(data string) (*models.SessionData, error) {
	var sessionData models.SessionData
	if err := sessionDataFromJSON(data, &sessionData); err != nil {
		return nil, err
	}
	return &sessionData, nil
}

// EncodeSessionData serializes session data to JSON.
func (q *Repository) EncodeSessionData(data *models.SessionData) (string, error) {
	return sessionDataToJSON(data)
}
