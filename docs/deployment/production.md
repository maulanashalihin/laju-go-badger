# Production Deployment

This guide covers deploying Laju Go to production servers, including Ubuntu/Debian deployment, systemd configuration, Nginx reverse proxy, and SSL setup.

## Quick Start: Git-Based Deployment (Recommended)

Laju Go menggunakan git-based deployment — kloning repo di server, build, jalankan binary. Sederhana, tanpa container:

```bash
# Di server
cd /opt/laju-go
git pull
npm run build:all
sudo systemctl restart laju-go
```

Ada 3 script di `scripts/` untuk membantu:

- `first-deploy.sh` — setup pertama (buat user, direktori, systemd service)
- `deploy.sh` — full deploy flow
- `update-deploy.sh` — incremental update (pull + build + restart)

## Prerequisites

### Server Requirements

- **OS**: Ubuntu 20.04+ or Debian 11+
- **RAM**: Minimum 512MB (1GB recommended)
- **Storage**: 10GB+ (depends on database size)
- **CPU**: 1 core minimum (2+ recommended)

### Domain Setup

- Domain name pointing to your server IP
- DNS A record configured

## Step 1: Server Setup

### Update System

```bash
sudo apt update && sudo apt upgrade -y
```

### Create Application User

```bash
# Create www-data user if not exists
sudo useradd -r -s /bin/false www-data
```

## Step 2: Application Setup

### Option A: Using Deployment Script (Recommended)

The deployment script automates all steps below and **builds everything locally**:

```bash
# From your local machine
npm run deploy
```

This will:

- Build frontend and Go binary **on your local machine** (pure-Go static binary, no CGO)
- Upload only runtime artifacts (`laju-go`, `dist/`) to server
- Configure `.env` file
- Create and start systemd service

> **No build tools needed on the server.** The server only runs the pre-built static binary. No gcc, no C toolchain, no SQLite libraries required.

See [One-Click Deployment](one-click-deployment.md) for details.

### Option B: Manual Build Locally (Recommended for Custom Setups)

### Build on Your Local Machine

The Go binary is built with `CGO_ENABLED=0`, producing a fully static binary with no external C dependencies (Badger is pure Go).

```bash
# Create application directory on server
ssh user@your-server "sudo mkdir -p /opt/laju-go"

# Build locally (static binary, no CGO)
CGO_ENABLED=0 go build -o laju-go ./cmd/server
# or via npm script:
npm run build:linux

# Upload artifacts
scp laju-go user@your-server:/opt/laju-go/
scp -r dist user@your-server:/opt/laju-go/dist
scp .env.example user@your-server:/opt/laju-go/.env
```

> **Note:** No `migrations/` directory is uploaded — Badger is schema-less and does not use goose migrations.

### Option C: Build on Server (Not Recommended)

Building on the server installs Go, Node.js, and npm — leaving build tools and cache (`node_modules/`, `go/pkg/`) that are unnecessary at runtime. Because the binary is pure Go (`CGO_ENABLED=0`), no gcc or C compiler is required even when building on the server.

```bash
# Create application directory
sudo mkdir -p /opt/laju-go
cd /opt/laju-go

# Clone repository
sudo git clone https://github.com/maulanashalihin/laju-go.git .
```

### Configure Environment

```bash
# Copy environment file
sudo cp .env.example .env

# Edit configuration
sudo nano .env
```

### Production Environment Configuration

```bash
# .env
APP_ENV=production
APP_PORT=8080
APP_URL=https://yourdomain.com

# Database (Badger KV — path is a directory, not a file)
DB_PATH=/var/lib/laju/badger

# Session (generate secure random key)
SESSION_SECRET=<run: openssl rand -base64 32>

# Google OAuth (optional)
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
GOOGLE_REDIRECT_URL=https://yourdomain.com/auth/google/callback

# Email/SMTP
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=noreply@yourdomain.com
SMTP_PASS=your-app-password
FROM_EMAIL=noreply@yourdomain.com
FROM_NAME=Your App Name
```

### Create Data Directories

```bash
# Create Badger data directory (Badger stores data in a directory, not a single file)
sudo mkdir -p /var/lib/laju/badger

# Create storage directory
sudo mkdir -p /opt/laju-go/storage/avatars

# Create backups directory
sudo mkdir -p /opt/laju-go/backups

# Set ownership
sudo chown -R www-data:www-data /var/lib/laju
sudo chown -R www-data:www-data /opt/laju-go

# Set permissions
sudo chmod 755 /var/lib/laju
sudo chmod 770 /opt/laju-go/storage
sudo chmod 770 /opt/laju-go/backups
```

## Step 3: Systemd Service

### Create Service File

```bash
sudo nano /etc/systemd/system/laju-go.service
```

### Service Configuration

```ini
[Unit]
Description=Laju Go Application
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/opt/laju-go
ExecStart=/opt/laju-go/laju-go
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=laju-go

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/laju /opt/laju-go/storage /opt/laju-go/backups

# Environment
Environment="PATH=/usr/local/go/bin:/usr/bin:/bin"
EnvironmentFile=/opt/laju-go/.env

[Install]
WantedBy=multi-user.target
```

### Enable and Start Service

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service on boot
sudo systemctl enable laju-go

# Start service
sudo systemctl start laju-go

# Check status
sudo systemctl status laju-go
```

### Service Management Commands

```bash
# Start
sudo systemctl start laju-go

# Stop
sudo systemctl stop laju-go

# Restart
sudo systemctl restart laju-go

# Reload (if supported)
sudo systemctl reload laju-go

# Check status
sudo systemctl status laju-go

# View logs
sudo journalctl -u laju-go -f

# View recent errors
journalctl -u laju-go -p err -n 50
```

## Step 4: Nginx Reverse Proxy

### Install Nginx

```bash
sudo apt install -y nginx
```

### Create Nginx Configuration

```bash
sudo nano /etc/nginx/sites-available/laju-go
```

### Nginx Configuration

```nginx
server {
    listen 80;
    server_name yourdomain.com www.yourdomain.com;

    # Redirect HTTP to HTTPS (after SSL setup)
    # return 301 https://$server_name$request_uri;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Port $server_port;
        
        # Proxy timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        # Buffering
        proxy_buffering off;
    }

    # Static assets (optional - Go serves these directly)
    # location /assets/ {
    #     alias /opt/laju-go/dist/assets/;
    #     expires 1y;
    #     add_header Cache-Control "public, immutable";
    # }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
}
```

### Enable Site

```bash
# Create symlink
sudo ln -s /etc/nginx/sites-available/laju-go /etc/nginx/sites-enabled/

# Remove default site
sudo rm /etc/nginx/sites-enabled/default

# Test configuration
sudo nginx -t

# Reload Nginx
sudo systemctl reload nginx
```

## Step 5: SSL with Let's Encrypt

### Install Certbot

```bash
sudo apt install -y certbot python3-certbot-nginx
```

### Obtain SSL Certificate

```bash
sudo certbot --nginx -d yourdomain.com -d www.yourdomain.com
```

### Auto-Renewal

Certbot installs automatic renewal. Test renewal:

```bash
sudo certbot renew --dry-run
```

### Verify SSL

Visit `https://yourdomain.com` and check for the padlock icon.

## Step 6: Database Notes

### Badger KV (No Tuning Required)

Badger is a pure-Go embedded LSM-tree key-value store. Unlike SQLite, there are no PRAGMAs to set, no WAL mode to enable, and no connection pool to size. Badger manages its own compaction and background goroutines.

The application opens the database at the configured `DB_PATH` (a directory) on startup:

```go
// Applied automatically on startup — no manual PRAGMA tuning.
db, err := badger.Open(badger.DefaultOptions(cfg.DBPath))
```

Key prefixes used in the store:

- `user:<id>` — user record (ID is a ULID string)
- `idx:user:em:<email>` — email index
- `idx:user:go:<gid>` — Google OAuth ID index
- `session:<id>` — session record
- `idx:sess:u:<uid>:<sid>` — session-by-user index
- `pwreset:<token>` — password reset token

### Verify the Data Directory

```bash
# Badger creates its files inside this directory on first run
ls -la /var/lib/laju/badger
# You should see .manifest, .sst, and value log files after the app starts
```

## Step 7: Backup Strategy

> For production apps, schedule regular online backups via Badger's `db.Backup()` API (exposed through the app's backup endpoint). The cron backup below is simpler but has up to 24h of potential data loss depending on schedule.

### Database Backup Script

```bash
sudo nano /opt/laju-go/scripts/backup.sh
```

```bash
#!/bin/bash

# Backup script for Laju Go (Badger KV)

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/opt/laju-go/backups"
DB_DIR="/var/lib/laju/badger"

mkdir -p "$BACKUP_DIR"

# Option 1: Online backup via Badger's db.Backup() API (no downtime)
curl -s -o "$BACKUP_DIR/badger-$DATE.bak" \
    http://localhost:8080/admin/backup

# Option 2: Offline directory copy (stop app first)
# tar -czf "$BACKUP_DIR/badger-$DATE.tar.gz" -C /var/lib/laju badger

# Delete backups older than 30 days
find "$BACKUP_DIR" -name "badger-*.bak" -mtime +30 -delete

echo "Backup completed: $DATE"
```

### Make Script Executable

```bash
sudo chmod +x /opt/laju-go/scripts/backup.sh
```

### Schedule Daily Backup

```bash
sudo crontab -e
```

Add cron job:

```bash
# Daily backup at 2 AM
0 2 * * * /opt/laju-go/scripts/backup.sh
```

### Manual Backup

```bash
# Online backup via the app's backup endpoint
curl -s -o /opt/laju-go/backups/badger-backup-$(date +%Y%m%d).bak \
    http://localhost:8080/admin/backup

# Offline directory copy (stop the app first)
sudo systemctl stop laju-go
tar -czf /opt/laju-go/backups/badger-backup-$(date +%Y%m%d).tar.gz \
    -C /var/lib/laju badger
sudo systemctl start laju-go

# List backups
ls -lh /opt/laju-go/backups/
```

### Restore from Backup

```bash
# Stop service
sudo systemctl stop laju-go

# Remove existing data directory
sudo rm -rf /var/lib/laju/badger

# Restore from online backup file (use the app's restore endpoint or CLI)
# OR restore from directory-copy backup:
sudo tar -xzf /opt/laju-go/backups/badger-backup-20260101.tar.gz -C /var/lib/laju/

# Set ownership
sudo chown -R www-data:www-data /var/lib/laju/badger

# Start service
sudo systemctl start laju-go
```

## Step 8: Monitoring

### Check Application Logs

```bash
# Real-time logs
sudo journalctl -u laju-go -f

# Last 100 lines
sudo journalctl -u laju-go -n 100

# Errors only
sudo journalctl -u laju-go -p err

# Today's logs
sudo journalctl -u laju-go --since today
```

### Health Check Endpoint

Add health check to your application:

```go
// routes/web.go
app.Get("/health", func(c *fiber.Ctx) error {
    return c.JSON(fiber.Map{
        "status": "healthy",
        "timestamp": time.Now(),
    })
})
```

Test health check:

```bash
curl http://localhost:8080/health
```

### Resource Monitoring

```bash
# Memory usage
free -h

# Disk usage
df -h

# CPU usage
top

# Process status
systemctl status laju-go
```

## Deployment Checklist

- [ ] Server updated (`apt update && apt upgrade`)
- [ ] Binary + assets uploaded (`laju-go`, `dist/`)
- [ ] Environment configured (`.env`)
- [ ] Frontend built (`npm run build`)
- [ ] Go binary built (`CGO_ENABLED=0 go build`)
- [ ] Environment configured (`.env`)
- [ ] Badger data directory created (`/var/lib/laju/badger`)
- [ ] Storage directory created
- [ ] Permissions set correctly
- [ ] Systemd service created
- [ ] Service enabled and started
- [ ] Nginx configured
- [ ] SSL certificate obtained
- [ ] Firewall configured (ports 80, 443)
- [ ] Backup script scheduled
- [ ] Monitoring configured

## Troubleshooting

### Service Won't Start

**Check logs**:

```bash
sudo journalctl -u laju-go -n 50
```

**Common issues**:

- Missing `.env` file
- Wrong `SESSION_SECRET`
- Badger data directory not writable
- Port already in use

### Badger Data Directory Issues

If Badger fails to open or reports corruption:

```bash
# Stop service
sudo systemctl stop laju-go

# Check the data directory exists and is owned by www-data
ls -la /var/lib/laju/badger
sudo chown -R www-data:www-data /var/lib/laju/badger

# If corrupted, run Badger's offline repair tool
badger --dir /var/lib/laju/badger repair

# If repair fails, restore from backup
sudo rm -rf /var/lib/laju/badger
sudo tar -xzf /opt/laju-go/backups/badger-backup-latest.tar.gz -C /var/lib/laju/
sudo chown -R www-data:www-data /var/lib/laju/badger

# Start service
sudo systemctl start laju-go
```

### Nginx 502 Bad Gateway

**Check if app is running**:

```bash
curl http://localhost:8080/health
```

**Check Nginx logs**:

```bash
sudo tail -f /var/log/nginx/error.log
```

### SSL Certificate Issues

**Renew certificate**:

```bash
sudo certbot renew
```

**Check certificate**:

```bash
sudo certbot certificates
```

## Security Hardening

### Firewall Configuration

```bash
# Install UFW
sudo apt install -y ufw

# Allow SSH
sudo ufw allow ssh

# Allow HTTP and HTTPS
sudo ufw allow http
sudo ufw allow https

# Enable firewall
sudo ufw enable

# Check status
sudo ufw status
```

### Disable Root Login

```bash
sudo nano /etc/ssh/sshd_config
```

```
PermitRootLogin no
PasswordAuthentication no
```

```bash
sudo systemctl restart sshd
```

### Automatic Security Updates

```bash
sudo apt install -y unattended-upgrades
sudo dpkg-reconfigure --priority=low unattended-upgrades
```

## Performance Tuning

### Systemd Service Tuning

```ini
[Service]
# Increase file descriptor limit (Badger opens many SSTable files)
LimitNOFILE=65535

# Memory limit (optional)
MemoryLimit=512M

# CPU limit (optional)
CPUQuota=80%
```

### Nginx Tuning

```nginx
# /etc/nginx/nginx.conf
worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 4096;
    use epoll;
    multi_accept on;
}

http {
    # Enable caching
    proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=app_cache:10m max_size=1g inactive=60m use_temp_path=off;
    
    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;
}
```

## Next Steps

- [Docker Deployment](docker.md) - Containerized deployment
- [Optimization Guide](optimization.md) - Performance optimization
- [Monitoring Guide](monitoring.md) - Application monitoring
