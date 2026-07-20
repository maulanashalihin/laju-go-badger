# Badger Data Protection & Recovery Guide

Complete guide to protecting production data from loss and implementing effective recovery strategies.

## Table of Contents

1. [Understanding Database Locks](#understanding-database-locks)
2. [Data Loss Scenarios](#data-loss-scenarios)
3. [Protection Strategies](#protection-strategies)
4. [Backup Implementation](#backup-implementation)
5. [Recovery Procedures](#recovery-procedures)
6. [Monitoring & Alerts](#monitoring--alerts)
7. [Production Checklist](#production-checklist)

---

## Understanding Database Locks

### Is Locked = Data Loss?

**Short answer: NO**

- Badger uses an LSM-tree with a managed write pipeline; there is no SQLite-style "database is locked" error
- Writes are buffered in a memtable and flushed to LSM files (SSTables) in the background
- Badger manages its own compaction goroutines, so you do not tune WAL or connection pools
- Data remains safe on disk once a transaction commits and `fsync()` completes

**Data loss ONLY occurs if:**

1. Power loss before `fsync()` completes on a committed transaction
2. Disk corruption
3. The `data/badger/` directory is deleted or partially removed manually
4. Catastrophic hardware failure

---

## Data Loss Scenarios

### Scenario 1: Write Contention (NOT Data Loss)

```
Situation:
- Application gets a conflict or transient write error under heavy load
- Users may see a retry on a write temporarily

Data Status:
✅ Data SAFE on disk
✅ No corruption
✅ No manual intervention needed

Recovery:
1. Badger retries / aborts the conflicting transaction internally
2. Application retry logic reattempts the write
3. If persistent: check for long-running read transactions blocking compaction

Data Loss: ❌ NONE
```

---

### Scenario 2: Power Loss During Write

```
Timeline:
T0: Badger transaction starts
T1: Write buffered in memtable
T2: ⚡ POWER FAILURE! (before fsync)
T3: Power restored

Data Status:
⚠️ Last uncommitted transaction MAY be lost
✅ Previous committed transactions SAFE
✅ Database NOT corrupted

Recovery:
1. Badger replays the value log and LSM on startup
2. Committed transactions are preserved
3. Uncommitted (un-fsync'd) writes are discarded

Data Loss: ⚠️ Only last ~1 second of writes
```

---

### Scenario 3: Value Log / LSM File Corruption

```
Situation:
- A SSTable or value log file in data/badger/ is corrupted (disk error, bug, etc.)
- Other LSM files may still be intact

Data Status:
✅ Most data SAFE in remaining LSM files
⚠️ Keys in the corrupted file may be unreadable

Recovery:
1. Stop the application
2. Restore the data/badger/ directory from backup (see backup strategy below)
3. If no backup, run Badger's offline repair tool:
   badger --dir data/badger --value-dir data/badger repair

Data Loss: ⚠️ Only keys in the corrupted file
```

---

### Scenario 4: Complete Database Corruption

```
Situation:
- The data/badger/ directory is corrupted or deleted
- Multiple LSM files affected

Data Status:
❌ Database unreadable
❌ Cannot recover from individual files

Recovery:
1. Restore from backup (see backup strategy below)
2. Replace the entire data/badger/ directory
3. Restart the application

Data Loss: ❌ Depends on backup recency
```

---

## Protection Strategies

### Layer 1: Badger Built-in Protection

```go
// Badger is opened with options tuned for durability.
// No PRAGMA tuning, no WAL mode, no connection pool sizing —
// Badger manages its own compaction and goroutines.

import (
    "github.com/dgraph-io/badger/v4"
    "github.com/oklog/ulid/v2"
)

db, err := badger.Open(badger.DefaultOptions("./data/badger").
    WithLoggingLevel(badger.INFO).
    WithValueLogFileSize(1<<28)) // 256MB value log files
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// All writes go through Badger transactions. A committed
// transaction is fsync'd to the value log, so committed data
// survives crashes. There is no separate WAL to checkpoint.
```

Badger handles the following automatically — no manual configuration needed:

- LSM-tree compaction (background goroutines)
- Value log garbage collection (`RunValueLogGC`)
- Memtable flush to SSTables
- Crash recovery on startup

---

### Layer 2: Automated Backups

#### Option A: Badger Online Backup via API (Recommended)

Use Badger's built-in `db.Backup()` API for consistent online backups with no downtime. This works while the application is running.

```go
// app/services/backup.go
package services

import (
    "os"
    "time"

    "github.com/dgraph-io/badger/v4"
)

func BackupDB(db *badger.DB, dest string) error {
    f, err := os.Create(dest)
    if err != nil {
        return err
    }
    defer f.Close()

    // Online backup — streams a consistent snapshot to the file.
    // sinceTs = 0 means back up everything.
    return db.Backup(f, 0)
}
```

Wrap it in a script and schedule with cron:

```bash
#!/bin/bash
# scripts/backup.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/opt/laju-go/backups"
DB_PATH="/opt/laju-go/data/badger"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Online backup is triggered by the app's backup endpoint or CLI:
curl -s -o "$BACKUP_DIR/badger-$DATE.bak" \
    http://localhost:8080/admin/backup

# Delete backups older than 30 days
find "$BACKUP_DIR" -name "badger-*.bak" -mtime +30 -delete

echo "Backup completed: $DATE"
```

**Schedule with cron**:

```bash
# Daily backup at 2 AM
0 2 * * * /opt/laju-go/scripts/backup.sh
```

---

#### Option B: Directory Copy (Simpler, Requires App Stopped)

Badger stores all data in the `data/badger/` directory. Copying the whole directory produces a valid backup, but the application must be stopped first to avoid copying files mid-compaction.

```bash
#!/bin/bash
# scripts/backup.sh

set -e

BACKUP_DIR="./backups"
DB_DIR="./data/badger"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Stop the app first for a consistent snapshot
# sudo systemctl stop laju-go

# Copy the entire Badger data directory
tar -czf "$BACKUP_DIR/backup_${TIMESTAMP}.tar.gz" -C ./data badger

# Restart the app
# sudo systemctl start laju-go

# Cleanup old backups (keep last 7 days)
find "$BACKUP_DIR" -name "backup_*.tar.gz" -mtime +7 -delete

echo "Backup completed: backup_${TIMESTAMP}.tar.gz"
```

**Cron job (every 6 hours):**

```bash
# crontab -e
0 */6 * * * cd /path/to/laju-go && ./scripts/backup.sh >> /var/log/laju-backup.log 2>&1
```

---

### Layer 3: Compaction & Value Log GC

Badger manages its own LSM-tree compaction in background goroutines. You do not need to run checkpoints manually. The only routine maintenance is value log garbage collection, which reclaims space from discarded versions:

```go
// Periodic value log GC to reclaim space from deleted/overwritten keys.
func autoValueLogGC(db *badger.DB) {
    ticker := time.NewTicker(10 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        // RunValueLogGC returns nil when there is nothing to GC.
        err := db.RunValueLogGC(0.5) // GC if 50% discardable
        if err != nil && err != badger.ErrNoRewrite {
            log.Printf("Value log GC error: %v", err)
        }
    }
}
```

**Why important:**

- Reclaims disk space from overwritten/deleted keys
- Keeps the value log from growing unbounded
- No equivalent of SQLite WAL checkpointing — compaction is automatic

---

### Layer 4: Replication (Advanced)

#### Option A: rsync to Remote Storage

Because Badger data is a single directory, you can replicate it with rsync. For a live (online) copy, prefer the `db.Backup()` API instead of rsyncing the directory directly.

```bash
#!/bin/bash
# scripts/sync-replica.sh

REMOTE_HOST="backup-server.example.com"
REMOTE_PATH="/backups/laju-go/"
LOCAL_DIR="./data/badger"

# Sync the Badger data directory (stop app first for consistency)
rsync -avz \
    --delete \
    -e ssh \
    "$LOCAL_DIR/" \
    user@$REMOTE_HOST:$REMOTE_PATH

echo "Sync completed to $REMOTE_HOST"
```

---

#### Option B: Scheduled Online Backup to S3

```bash
#!/bin/bash
# scripts/backup-to-s3.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="/tmp/badger-$DATE.bak"

# Online backup via the app's backup endpoint
curl -s -o "$BACKUP_FILE" http://localhost:8080/admin/backup

# Upload to S3
aws s3 cp "$BACKUP_FILE" s3://laju-go-backups/badger-$DATE.bak \
    --storage-class STANDARD_IA

# Clean up local temp file
rm "$BACKUP_FILE"

echo "S3 backup completed: $DATE"
```

**Benefits:**

- Online (no downtime) via Badger's `db.Backup()` API
- Point-in-time snapshots
- Cross-region redundancy with S3 replication

---

### Layer 5: Monitoring & Alerts

```go
// app/services/health.go
package services

import (
    "fmt"
    "os"
    "syscall"
    "time"

    "github.com/dgraph-io/badger/v4"
)

type HealthService struct {
    db *badger.DB
}

func (s *HealthService) CheckDatabase() error {
    // 1. Check that the data directory exists and is writable
    info, err := os.Stat("./data/badger")
    if err != nil {
        return fmt.Errorf("badger data directory inaccessible: %v", err)
    }
    if !info.IsDir() {
        return fmt.Errorf("badger data path is not a directory")
    }

    // 2. Check that Badger is responsive with a read transaction
    err = s.db.View(func(txn *badger.Txn) error {
        // A trivial read confirms the DB is open and readable.
        _, err := txn.Get([]byte("__healthcheck__"))
        if err == badger.ErrKeyNotFound {
            return nil
        }
        return err
    })
    if err != nil {
        return fmt.Errorf("badger read check failed: %v", err)
    }

    // 3. Check disk space
    stat := &syscall.Statfs_t{}
    err = syscall.Statfs("./data", stat)
    if err != nil {
        return fmt.Errorf("cannot check disk space: %v", err)
    }

    available := stat.Bavail * uint64(stat.Bsize)
    if available < 100_000_000 { // Less than 100MB
        return fmt.Errorf("low disk space: %d bytes available", available)
    }

    return nil
}

// StartMonitoring starts health check loop
func (s *HealthService) StartMonitoring(interval time.Duration, alertFunc func(error)) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()

        for range ticker.C {
            if err := s.CheckDatabase(); err != nil {
                alertFunc(err) // Send to Slack, email, etc.
            }
        }
    }()
}
```

---

## Recovery Procedures

### Recovery 1: After Transient Write Error

```go
func safeWrite(db *badger.DB, key, val []byte) error {
    maxRetries := 3
    for i := 0; i < maxRetries; i++ {
        err := db.Update(func(txn *badger.Txn) error {
            return txn.Set(key, val)
        })
        if err == nil {
            return nil
        }

        // Retry on conflict / transient errors
        if i < maxRetries-1 {
            time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
            continue
        }

        return err
    }
    return nil
}
```

---

### Recovery 2: After Power Failure

```bash
# 1. Badger auto-replays the value log and LSM on startup.
#    Simply restart the application:
sudo systemctl restart laju-go

# 2. Check logs for replay/compaction errors:
sudo journalctl -u laju-go -n 50

# 3. If Badger reports corruption, run the offline repair tool:
sudo systemctl stop laju-go
badger --dir /opt/laju-go/data/badger repair
sudo systemctl start laju-go

# 4. Restore from backup if repair fails:
sudo systemctl stop laju-go
rm -rf /opt/laju-go/data/badger
tar -xzf /opt/laju-go/backups/backup_20260328_120000.tar.gz -C /opt/laju-go/data/
sudo systemctl start laju-go
```

---

### Recovery 3: Complete Database Restore

```bash
#!/bin/bash
# scripts/restore.sh

BACKUP_FILE=$1

if [ -z "$BACKUP_FILE" ]; then
    echo "Usage: ./restore.sh <backup-file.tar.gz>"
    exit 1
fi

# Stop application
sudo systemctl stop laju-go

# Remove corrupted data directory
rm -rf /opt/laju-go/data/badger

# Extract backup (restores data/badger/)
tar -xzf "$BACKUP_FILE" -C /opt/laju-go/data/

# Set ownership
sudo chown -R www-data:www-data /opt/laju-go/data/badger

# Start application
sudo systemctl start laju-go

echo "Restore completed from $BACKUP_FILE"
```

---

## Production Checklist

### Prevention

- [ ] Badger data directory on a reliable disk (`data/badger/`)
- [ ] Automated backups every 6 hours (online via `db.Backup()` or offline directory copy)
- [ ] Backup retention: 7-30 days
- [ ] Off-site replication (S3 or remote server)
- [ ] Value log GC scheduled (or rely on Badger's automatic GC)
- [ ] Disk space monitoring
- [ ] No manual deletion of files inside `data/badger/`

---

### Recovery

- [ ] Documented restore procedure
- [ ] Tested backup restoration
- [ ] Emergency contact list
- [ ] Runbook for common issues

---

### Monitoring

- [ ] Database health checks (every 5 min)
- [ ] Data directory size alerts
- [ ] Backup success/failure alerts
- [ ] Disk space alerts (<1GB)
- [ ] Error rate monitoring

---

## Data Loss Risk Assessment

| Scenario | Probability | Impact | Mitigation |
|----------|-------------|--------|------------|
| **Write contention** | Low | Low (temporary) | Retry logic, transaction conflicts |
| **Power loss** | Low | Medium (last tx) | Badger fsync on commit, UPS |
| **File corruption** | Very Low | Low (affected keys) | Backups, Badger repair tool |
| **Disk failure** | Low | High (all data) | Backups, replication |
| **Human error** | Medium | High | Backups, access control |

---

## Summary

**For production with critical data:**

1. **Badger LSM storage** - Already in place, no PRAGMA/WAL tuning needed
2. **Automated backups** - Every 6 hours, keep 7-30 days
3. **Off-site replication** - S3 or remote server
4. **Monitoring** - Health checks, alerts
5. **Tested recovery** - Practice restore procedures

**Expected data loss:**

- **With online backups:** < 6 hours of data (depends on schedule)
- **With frequent S3 backups:** < 1 hour of data
- **Without backups:** Up to all data on disk failure

**Recovery time:**

- **Transient write error:** Automatic (seconds)
- **Power failure:** Automatic (seconds, Badger replays on startup)
- **Backup restore:** 5-30 minutes
- **Full disaster:** 1-4 hours

---

## Related Documentation

- [Database Guide](guide/database.md) - Database setup and Badger configuration
- [Production Deployment](deployment/production.md) - Production setup guide
- [Performance Optimization](deployment/optimization.md) - Badger tuning

---

## Changelog

| Date | Change | Reason |
|------|--------|--------|
| 2026-03-28 | Initial documentation | Complete data protection guide |
| 2026-03-28 | Migrated from SQLite to Badger KV | Database backend changed to Dgraph Badger |
