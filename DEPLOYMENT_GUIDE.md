# 🌐 Complete Deployment Guide — Kinjo Platform

This guide provides end-to-end instructions for deploying the **Kinjo Backend API** (Go), **Database** (PostgreSQL), **Web Application** (React/Vite), and **Mobile Applications** (Capacitor Android/iOS) to production environments.

---

## 📋 Prerequisites & Infrastructure Requirements

| Component | Recommended Production Specs | Minimum Specs |
|---|---|---|
| **OS** | Ubuntu 22.04 LTS / Debian 12 | Ubuntu 20.04 LTS |
| **CPU / RAM** | 2 vCPU / 4 GB RAM | 1 vCPU / 1 GB RAM |
| **Database** | PostgreSQL 14+ (Managed DB or Self-Hosted) | PostgreSQL 14+ |
| **Domain & SSL** | Custom Domain with SSL/TLS (Let's Encrypt / Certbot) | Valid SSL domain |
| **Node.js** | v18.x or v20.x LTS | v18.x |
| **Go Runtime** | v1.22+ | v1.22 |

---

## ⚡ Local Development vs Production Backend Status

### How the Backend Runs Right Now (Local Dev & Testing)
1. **Local Go Engine**: The Go backend server runs locally on your machine via `go run ./cmd/api` on port `8080`.
2. **Cloud Database**: It is connected to your remote Supabase PostgreSQL database (`aws-0-ap-southeast-1.pooler.supabase.com`).
3. **Ngrok Tunneling**:
   - `frontend/.env.local` sets `VITE_API_URL=https://be81-14-194-0-162.ngrok-free.app/api`.
   - **Why Ngrok is used**: Mobile devices, Android native Capacitor apps, and browser security policies require an **HTTPS endpoint** for Google OAuth popups, browser camera access (face verification), and GPS geolocation. Ngrok tunnels secure external HTTPS requests to your local Go server at `http://localhost:8080`.

---

## 1. 🗄️ Database Setup & Migrations (PostgreSQL)

Kinjo uses PostgreSQL for user authentication, profiles, presence tracking, and device tokens.

### A. Create Production Database & User
Connect to your PostgreSQL server:
```sql
CREATE DATABASE kinjo_production;
CREATE USER kinjo_user WITH ENCRYPTED PASSWORD 'SUPER_SECURE_PASSWORD_HERE';
GRANT ALL PRIVILEGES ON DATABASE kinjo_production TO kinjo_user;
```

### B. Connection String Format
Your `DATABASE_URL` format:
```env
DATABASE_URL=postgresql://kinjo_user:SUPER_SECURE_PASSWORD_HERE@localhost:5432/kinjo_production?sslmode=disable
```

*(Note: The Go backend automatically executes all SQL migrations located in `internal/database/migrations` on server startup.)*

---

## 2. ⚙️ Go Backend API Deployment

### A. Environment Configuration (`.env`)
Create a `.env` file in your server deployment directory (e.g. `/var/www/kinjo-backend`):

```env
# Server Config
PORT=8080
ENVIRONMENT=production
LOG_LEVEL=info
BASE_URL=https://api.yourdomain.com
ALLOWED_ORIGINS=https://yourdomain.com,https://app.yourdomain.com,capacitor://localhost

# Database
DATABASE_URL=postgresql://kinjo_user:SUPER_SECURE_PASSWORD_HERE@localhost:5432/kinjo_production?sslmode=disable

# Security & JWT
JWT_SECRET=YOUR_RANDOM_LONG_SECRET_KEY_MIN_32_CHARS

# File Storage
STORAGE_DRIVER=local
STORAGE_DIR=./uploads

# Transactional Email Integration (SMTP for OTP & Welcome Emails)
SMTP_HOST=smtp.yourprovider.com
SMTP_PORT=587
SMTP_USERNAME=your_smtp_username
SMTP_PASSWORD=your_smtp_password
SMTP_SENDER_EMAIL=hello@yourdomain.com
SMTP_SENDER_NAME=Kinjo
SMTP_ENCRYPTION=tls

# Push Notifications (Firebase Cloud Messaging)
FCM_PROJECT_ID=your_firebase_project_id
FCM_SERVICE_ACCOUNT_KEY=/path/to/firebase-service-account.json

# Optional OpenAI Integration
OPENAI_API_KEY=your_openai_api_key_here
```

### B. Build Binary
Compile the Go backend binary on the target Linux environment:
```bash
cd backend/backend-go
go build -o kinjo-api ./cmd/api
```

### C. Configure Systemd Service
Create `/etc/systemd/system/kinjo-api.service`:
```ini
[Unit]
Description=Kinjo Go Backend Service
After=network.target postgresql.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/var/www/kinjo-backend
ExecStart=/var/www/kinjo-backend/kinjo-api
Restart=always
RestartSec=5
EnvironmentFile=/var/www/kinjo-backend/.env

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable kinjo-api
sudo systemctl start kinjo-api
sudo systemctl status kinjo-api
```

---

## 3. 🌐 Reverse Proxy & SSL Setup (Nginx + Let's Encrypt)

Install Nginx and Certbot:
```bash
sudo apt update
sudo apt install nginx certbot python3-certbot-nginx -y
```

Create `/etc/nginx/sites-available/api.yourdomain.com`:
```nginx
server {
    server_name api.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # Static uploads directory
    location /uploads/ {
        alias /var/www/kinjo-backend/uploads/;
        expires 30d;
        add_header Cache-Control "public, no-transform";
    }
}
```

Activate domain and enable SSL:
```bash
sudo ln -s /etc/nginx/sites-available/api.yourdomain.com /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
sudo certbot --nginx -d api.yourdomain.com
```

---

## 4. 🎨 Web Frontend Deployment

### A. Environment Configuration (`.env.production`)
Inside `/frontend/.env.production`:
```env
VITE_API_URL=https://api.yourdomain.com/api
```

### B. Build Production Bundle
```bash
cd frontend
npm install
npm run build
```
This generates optimized static production assets in `frontend/dist`.

### C. Serve via Nginx or Vercel/Netlify
For Nginx, copy `dist` contents to `/var/www/kinjo-web` and configure:
```nginx
server {
    server_name app.yourdomain.com;
    root /var/www/kinjo-web;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

---

## 5. 📱 Native Mobile Builds (Android & iOS)

### A. Android Build (APK & AAB Release Bundle)

1. Sync Capacitor web assets:
```bash
cd frontend
npm run build
npx cap sync android
```

2. Build Debug APK:
```bash
cd android
./gradlew assembleDebug
```
Output: `frontend/android/app/build/outputs/apk/debug/app-debug.apk`

3. Build Release Android App Bundle (AAB for Google Play):
```bash
cd android
./gradlew bundleRelease
```
Output: `frontend/android/app/build/outputs/bundle/release/app-release.aab`

### B. iOS Build (XCode Setup)

```bash
cd frontend
npm run build
npx cap sync ios
npx cap open ios
```
Open XCode to archive and submit your iOS app to the Apple App Store.

---

## 6. 🔔 Push Notification Setup (FCM & APNs)

1. **Android Push (Firebase Cloud Messaging)**:
   - Place your `google-services.json` in `frontend/android/app/google-services.json`.
   - Register FCM Server Credentials on backend if push relaying to Google FCM servers is required.

2. **Testing Custom Push API**:
   ```bash
   curl -X POST https://api.yourdomain.com/api/notifications/send-custom \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_LOGGED_IN_JWT" \
     -d '{
       "target_email": "user@example.com",
       "title": "Welcome to Kinjo!",
       "body": "Your discovery deck is ready."
     }'
   ```

---

## 7. ☁️ Step-by-Step Guide: Deploying on Utho Cloud (`kinjo.world`)

[Utho](https://utho.com) is an Indian Cloud Infrastructure provider offering high-performance, low-latency Cloud Compute (VPS) instances in Delhi NCR, Mumbai, and Bengaluru.

### Step 1: Create a Cloud Compute Instance on Utho
1. Log in to your [Utho Console](https://console.utho.com).
2. Navigate to **Compute** > **Cloud Instances** > click **Create Instance**.
3. **Choose Operating System**: Select **Ubuntu 22.04 LTS** or **Ubuntu 24.04 LTS**.
4. **Choose Plan**: A standard instance (1–2 vCPU, 2–4 GB RAM) is ideal.
5. **Select Datacenter Region**: Choose **Mumbai** or **Noida (Delhi NCR)** for optimal latency in India.
6. **Authentication**: Set an SSH Key or root password.
7. Click **Deploy Now**. Once deployed, copy your server's **Public IPv4 Address** (e.g. `103.xxx.xxx.xxx`).

---

### Step 2: Configure DNS for `kinjo.world`
In your domain registrar dashboard (where you purchased `kinjo.world`):
1. **API Backend**:
   - Record Type: `A`
   - Host: `api`
   - Value: `YOUR_UTHO_SERVER_IP`
   - TTL: Auto or 300s
2. **Web / Landing Site**:
   - Record Type: `A`
   - Host: `@`
   - Value: `YOUR_UTHO_SERVER_IP` (or your Vercel/Cloudflare CNAME)
   - Record Type: `CNAME`
   - Host: `www`
   - Value: `kinjo.world`

---

### Step 3: Connect & Prepare the Utho Server
From your local terminal, SSH into your Utho server:
```bash
ssh root@YOUR_UTHO_SERVER_IP
```

Update packages and install dependencies:
```bash
apt update && apt upgrade -y
apt install -y git curl wget build-essential nginx certbot python3-certbot-nginx
```

Install Go (v1.22+):
```bash
wget https://go.dev/dl/go1.22.6.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

---

### Step 4: Transfer Code & Configure Environment
1. Create application directory:
   ```bash
   mkdir -p /var/www/kinjo-backend
   cd /var/www/kinjo-backend
   ```
2. Upload or clone your repository backend files into `/var/www/kinjo-backend`.
3. Create the production `.env` file:
   ```bash
   nano /var/www/kinjo-backend/.env
   ```
   Paste your production secrets:
   ```env
   ENVIRONMENT=production
   PORT=8080
   BASE_URL=https://api.kinjo.world
   ALLOWED_ORIGINS=https://kinjo.world,https://www.kinjo.world,capacitor://localhost

   # PostgreSQL
   DATABASE_URL=postgresql://postgres:YOUR_DB_PASS@YOUR_DB_HOST:5432/kinjo?sslmode=require

   # Accounts allowed to broadcast push notifications (comma-separated).
   # Leave empty to disable the broadcast endpoint entirely.
   ADMIN_EMAILS=you@kinjo.world

   # SMTP Transactional Email
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USERNAME=your_email@gmail.com
   SMTP_PASSWORD=your_app_password
   SMTP_SENDER_EMAIL=hello@kinjo.world
   SMTP_SENDER_NAME=Kinjo
   SMTP_ENCRYPTION=tls

   # Push Notifications
   FCM_PROJECT_ID=your_fcm_project_id
   FCM_SERVICE_ACCOUNT_KEY=/var/www/kinjo-backend/firebase-service-account.json
   ```
4. Build the binary:
   ```bash
   cd /var/www/kinjo-backend
   go build -o kinjo-api ./cmd/api
   ```

---

### Step 5: Setup Systemd Service (Auto-Start & 24/7 Uptime)
Create `/etc/systemd/system/kinjo.service`:
```bash
cat << 'EOF' > /etc/systemd/system/kinjo.service
[Unit]
Description=Kinjo Go Backend API
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/var/www/kinjo-backend
ExecStart=/var/www/kinjo-backend/kinjo-api
Restart=always
RestartSec=5
EnvironmentFile=/var/www/kinjo-backend/.env
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
```

Start and enable the service:
```bash
systemctl daemon-reload
systemctl enable kinjo
systemctl start kinjo
systemctl status kinjo
```

---

### Step 6: Configure Nginx & Free SSL (HTTPS)
Create `/etc/nginx/sites-available/api.kinjo.world`:
```bash
cat << 'EOF' > /etc/nginx/sites-available/api.kinjo.world
server {
    server_name api.kinjo.world;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
EOF
```

Enable the site and issue free SSL certificates via Certbot:
```bash
ln -sf /etc/nginx/sites-available/api.kinjo.world /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx
certbot --nginx -d api.kinjo.world --non-interactive --agree-tos -m admin@kinjo.world
```

Your API is now live and secured at `https://api.kinjo.world/api/v1/health`!

---

## 📊 Summary Checklists

- [x] PostgreSQL Database configured & migrations automatically applied.
- [x] Go backend built & running via Systemd service.
- [x] Nginx Reverse Proxy & Let's Encrypt SSL/TLS active.
- [x] Web Application deployed to static hosting / CDN.
- [x] Capacitor Android APK & AAB generated.
- [x] Push notification device token registration endpoint active (`POST /api/notifications/register-token`).
- [x] Utho Cloud Compute instance setup and DNS mapping verified.
