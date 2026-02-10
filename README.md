# Shukarsh Enterprises ✿

Cute kawaii-style product showcase for [Shukarsh Enterprises](https://www.meesho.com/ShuKarshEnterprises). Built with Go + SQLite.

[![Deploy on Fly.io](https://img.shields.io/badge/Deploy%20on-Fly.io-8b5cf6?style=for-the-badge&logo=fly.io)](https://fly.io/docs/getting-started/)

## 🚀 Deploy to Fly.io (2 commands)

**Prerequisites:** Install [flyctl](https://fly.io/docs/flyctl/install/)

```bash
# 1. Clone and enter the repo
git clone https://github.com/iamlordutkarsh/shukarsh.git
cd shukarsh

# 2. Launch on Fly.io (creates app + volume + deploys)
fly launch --copy-config --yes

# 3. Set your admin password
fly secrets set ADMIN_PASSWORD=your_secret_password

# Done! Your site is live at https://shukarsh.fly.dev
```

### Custom Domain

```bash
fly certs add yourdomain.com
# Then point your DNS A record to the IP shown
```

### Useful Commands

```bash
fly deploy          # Redeploy after changes
fly logs            # View live logs
fly ssh console     # SSH into the VM
fly status          # Check app status
```

## 🛠️ Local Development

```bash
make build
./shukarsh-server --listen :8000 --admin-password mypass
# Open http://localhost:8000
```

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `ADMIN_PASSWORD` | Admin panel password | _(no auth)_ |
| `DB_PATH` | SQLite database path | `db.sqlite3` |
| `UPLOADS_DIR` | Uploaded images directory | `./uploads` |

## 📁 Project Structure

```
cmd/srv/          → Main binary entrypoint
srv/              → HTTP server, handlers, routes
srv/templates/    → Go HTML templates (home, product, admin, search)
srv/static/       → PWA assets, icons, manifest
db/               → SQLite setup + migrations
db/migrations/    → SQL migration files
Dockerfile        → Multi-stage Docker build
fly.toml          → Fly.io deployment config
```

## ✨ Features

- 🏠 KawaiiStore-style homepage with animated category bubbles
- 🎠 Hero carousel with featured products
- 🛍️ Product detail pages with image gallery
- 🔍 Search with suggestion chips
- 📱 PWA — installable as mobile app
- 🌙 Dark mode
- 🔐 Password-protected admin panel
- 📷 Image upload from device
- 📦 Bulk import from Meesho
- 🗺️ SEO sitemap + robots.txt
- 📱 QR code generator per product
