# 🔄 nextya-sync

A powerful CLI tool for synchronizing files between Nextcloud and Yandex Disk cloud storage services.

## ⚙️ Configuration

### 🌐 Environment Variables

Create a `.env` file in the project root:

```env
YANDEX_TOKEN=your_yandex_oauth_token
YANDEX_TARGET_PATH=/nextcloud
NEXTCLOUD_URL=https://nextcloud-host.com
NEXTCLOUD_USERNAME=admin
NEXTCLOUD_PASSWORD=password
NEXTCLOUD_SYNC_PATHS=/data /documents /photos
```

### 📄 Configuration File

Create `~/.nextya-sync.yaml`:

```yaml
yandex:
  token: "your_yandex_oauth_token"
  target_path: "/nextcloud"

nextcloud:
  url: "https://nextcloud-host.com"
  username: "admin"
  password: "password"
  sync_paths:
    - "/data"
    - "/documents"
    - "/photos"
```

### 🏃‍♂️ Command Line Flags

All configuration options can be specified via command-line flags:

```bash
nextya-sync \
  --yandex-token "your_token" \
  --yandex-target-path "/nextcloud" \
  --nextcloud-url "https://nextcloud-host.com" \
  --nextcloud-username "admin" \
  --nextcloud-password "password" \
  --nextcloud-paths "/data /documents /photos"
```

## 🔐 Authentication

### 🟡 Yandex Disk OAuth Token

1. Go to [Yandex OAuth](https://oauth.yandex.com/) 🌐
2. Create a new application 📱
3. Get OAuth token with `cloud_api:disk.read` and `cloud_api:disk.write` permissions 🔑

### ☁️ Nextcloud Credentials

Use your regular Nextcloud username and password, or create an app password for better security. 🔒


## 🔧 Configuration Priority

Settings are loaded in this order (later overrides earlier):
1. 📄 Configuration file (`~/.nextya-sync.yaml`)
2. 🌐 Environment variables
3. 🏃‍♂️ Command-line flags

## ⏰ Automated Scheduling with Cron

### 🤖 Setting up Cron Job

To run synchronization automatically at regular intervals, you can set up a cron job:

#### 1. 📝 Create a sync script

Create a script file `/home/user/scripts/nextya-sync.sh`:

```bash
#!/bin/bash

# Set environment variables
export YANDEX_TOKEN="y0_your_token_here"
export NEXTCLOUD_URL="https://cloud.example.com"
export NEXTCLOUD_USERNAME="myuser"
export NEXTCLOUD_PASSWORD="mypassword"
export NEXTCLOUD_SYNC_PATHS="/Documents /Photos"
export YANDEX_TARGET_PATH="/backup/nextcloud"

# Set the path to your binary
SYNC_BIN="/path/to/nextya-sync"

# Create log directory if it doesn't exist
LOG_DIR="/var/log/nextya-sync"
mkdir -p "$LOG_DIR"

# Run sync with logging
echo "$(date): Starting sync..." >> "$LOG_DIR/sync.log"
$SYNC_BIN >> "$LOG_DIR/sync.log" 2>&1
echo "$(date): Sync completed with exit code: $?" >> "$LOG_DIR/sync.log"
echo "---" >> "$LOG_DIR/sync.log"
```

#### 2. 🔐 Make the script executable

```bash
chmod +x /home/user/scripts/nextya-sync.sh
```

#### 3. ⏰ Add to crontab

```bash
# Edit crontab
crontab -e

# Add one of these lines based on your needs:

# Every hour at minute 0
0 * * * * /home/user/scripts/nextya-sync.sh

# Every 6 hours
0 */6 * * * /home/user/scripts/nextya-sync.sh

# Daily at 2 AM
0 2 * * * /home/user/scripts/nextya-sync.sh

# Every Monday at 3 AM (weekly)
0 3 * * 1 /home/user/scripts/nextya-sync.sh

# Every 15 minutes (frequent sync)
*/15 * * * * /home/user/scripts/nextya-sync.sh
```

#### 4. 📝 Monitor Logs

```bash
# View recent sync logs
tail -f /var/log/nextya-sync/sync.log

# View cron logs
grep nextya-sync /var/log/syslog

# Check cron job status
crontab -l
```
