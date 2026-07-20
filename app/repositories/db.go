package repositories

import (
	"crypto/rand"

	"github.com/dgraph-io/badger/v4"
	"github.com/oklog/ulid/v2"
)

// Key prefixes — Badger is a sorted KV store, so all keys under a prefix are
// contiguous and iterable via prefix scans.
const (
	keyUserPrefix       = "user:"        // user:<id>          -> JSON userRecord
	keyUserEmailIndex   = "idx:user:em:" // idx:user:em:<email> -> <id>
	keyUserGoogleIndex  = "idx:user:go:" // idx:user:go:<gid>   -> <id>
	keySessionPrefix    = "session:"     // session:<id>        -> JSON Session
	keySessionUserIndex = "idx:sess:u:"  // idx:sess:u:<uid>:<sid> -> "" (existence index)
	keyPasswordReset    = "pwreset:"     // pwreset:<token>     -> JSON PasswordReset
)

// Repository is the single layer that touches Badger. Services call its methods;
// handlers never touch it directly (Three-Tier Rule).
type Repository struct {
	db *badger.DB
}

// NewRepository wraps a Badger DB handle.
func NewRepository(db *badger.DB) *Repository {
	return &Repository{db: db}
}

// DB exposes the underlying handle (used by main for graceful Close).
func (q *Repository) DB() *badger.DB { return q.db }

// newULID generates a lexically-sortable, unique ID using crypto/rand entropy.
func newULID() (string, error) {
	id, err := ulid.New(ulid.Now(), rand.Reader)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// txnGet reads a key inside a transaction. Returns ("", badger.ErrKeyNotFound) if absent.
func txnGet(txn *badger.Txn, key []byte) ([]byte, error) {
	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	var val []byte
	err = item.Value(func(v []byte) error {
		val = append([]byte{}, v...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

// view runs a read-only transaction.
func (q *Repository) view(fn func(*badger.Txn) error) error {
	return q.db.View(fn)
}

// update runs a read-write transaction.
func (q *Repository) update(fn func(*badger.Txn) error) error {
	return q.db.Update(fn)
}
