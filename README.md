<p align="center">
  <img src="static/koffan-logo.webp" alt="Koffan Logo" width="400">
</p>

<h1 align="center">Koffan</h1>

<p align="center">
  <strong>Free shopping assistant</strong><br>
  A fast and simple app for managing your shopping list together
</p>

<p align="center">
  <a href="https://railway.app/new/template?template=https://github.com/PanSalut/Koffan"><img src="https://railway.app/button.svg" alt="Deploy on Railway" height="32"></a>
  &nbsp;
  <a href="https://render.com/deploy?repo=https://github.com/PanSalut/Koffan"><img src="https://render.com/images/deploy-to-render-button.svg" alt="Deploy to Render" height="32"></a>
  &nbsp;
  <a href="https://cloud.digitalocean.com/apps/new?repo=https://github.com/PanSalut/Koffan/tree/main"><img src="https://www.deploytodo.com/do-btn-blue.svg" alt="Deploy to DigitalOcean" height="32"></a>
  &nbsp;
  <a href="https://heroku.com/deploy?template=https://github.com/PanSalut/Koffan"><img src="https://www.herokucdn.com/deploy/button.svg" alt="Deploy to Heroku" height="32"></a>
</p>

---

## Screenshots

<p align="center">
  <img src="screenshots/hero.png" alt="Koffan" width="700">
</p>

---

## What is Koffan?

Koffan is a lightweight web application for managing shopping lists, designed for couples and families. It allows real-time synchronization between multiple devices, so everyone knows what to buy and what's already in the cart.

The app works in any browser on both mobile and desktop. Just one password to log in - no complicated registration required.

## Why did I build this?

I needed an app that would let me and my wife create a shopping list together and do grocery shopping quickly and efficiently. I tested various solutions, but none of them were simple and fast enough.

I built the first version in **Next.js**, but it turned out to be very resource-heavy. I have a lot of other things running on my server, so I decided to optimize. I rewrote the app in **Go** and now it uses only **~2.5 MB RAM** instead of hundreds of megabytes!

## Features

- **Ultra-lightweight** - ~16 MB on disk, ~2.5 MB RAM
- **Multiple lists** - Create separate lists for different stores or purposes, with custom icons
- **PWA** - Install on your phone like a native app
- **Offline mode** - Add, edit, check/uncheck products without internet (auto-sync when back online)
- **Auto-completion** - Fuzzy search suggestions from your history, remembers sections
- Organize products into sections (e.g., Dairy, Vegetables, Cleaning)
- Mark products as purchased
- Mark products as "uncertain" (can't find it in the store)
- Real-time synchronization (WebSocket)
- Responsive interface (mobile-first)
- **Dark mode** - Automatic theme based on system preferences
- Multi-language support (PL, EN, DE, ES, FR, PT, UK, NO, LT, EL)
- Simple login system
- Rate limiting protection against brute-force attacks
- **REST API** - Programmatic access for integrations and migrations ([docs](https://github.com/PanSalut/Koffan/wiki/REST-API))

## Tech Stack

- **Backend:** Go 1.21 + Fiber
- **Frontend:** HTMX + Alpine.js + Tailwind CSS
- **Database:** SQLite

## Local Setup (without Docker)

You can run Koffan directly on your machine using Go. This works on any system (macOS, Linux, Windows).

### 1. Install Go

**macOS (Homebrew):**
```bash
brew install go
```

**Linux (Debian/Ubuntu):**
```bash
sudo apt install golang-go
```

**Windows:**
Download from [go.dev/dl](https://go.dev/dl/)

### 2. Clone and Run

```bash
git clone https://github.com/PanSalut/Koffan.git
cd Koffan
go run main.go
```

App available at http://localhost:3000

Default password: `shopping123`

To set a custom password:
```bash
APP_PASSWORD=yourpassword go run main.go
```

## Docker

### Quick Start (recommended)

```bash
docker run -d -p 3000:80 -e APP_PASSWORD=yourpassword -v koffan-data:/data ghcr.io/pansalut/koffan:latest
```

App available at http://localhost:3000

### Build from source

```bash
docker-compose up -d
# App available at http://localhost:80
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | `development` | Set to `production` for secure cookies |
| `APP_PASSWORD` | `shopping123` | Login password |
| `DISABLE_AUTH` | `false` | Set to `true` to disable authentication (for reverse proxy setups) |
| `PORT` | `80` (Docker) / `3000` (local) | Server port |
| `DB_PATH` | `./shopping.db` | Database file path |
| `DEFAULT_LANG` | `en` | Default UI language (pl, en, de, es, fr, pt, uk, no, lt, el) |
| `LOGIN_MAX_ATTEMPTS` | `5` | Max login attempts before lockout |
| `LOGIN_WINDOW_MINUTES` | `15` | Time window for counting attempts |
| `LOGIN_LOCKOUT_MINUTES` | `30` | Lockout duration after exceeding limit |
| `API_TOKEN` | *(disabled)* | Enable REST API with this token ([docs](https://github.com/PanSalut/Koffan/wiki/REST-API)) |

## Deploy to Your Server

### Docker

```bash
git clone https://github.com/PanSalut/Koffan.git
cd Koffan
docker build -t koffan .
docker run -d -p 80:80 -e APP_PASSWORD=your-password -v koffan-data:/data koffan
```

### Coolify

1. Add new resource → **Docker Compose** → Select your Git repository or use `https://github.com/PanSalut/Koffan`
2. Set domain in **Domains** section
3. Enable **Connect to Predefined Network** in Advanced settings
4. Add environment variable `APP_PASSWORD` with your password
5. Deploy

### Persistent Storage

Data is stored in `/data/shopping.db`. The volume ensures your data persists across deployments.

## Documentation

For more information, check the **[Wiki](https://github.com/PanSalut/Koffan/wiki)**:

- [REST API](https://github.com/PanSalut/Koffan/wiki/REST-API) - Programmatic access, migrations, integrations

## Feature Requests

Have an idea? Check [open feature requests](https://github.com/PanSalut/Koffan/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement) and vote with 👍 on the ones you want most.

Want to suggest something new? [Create an issue](https://github.com/PanSalut/Koffan/issues/new).

## Support

If you find this project useful, consider supporting its development:

[![Sponsor](https://img.shields.io/badge/Sponsor-%E2%9D%A4-pink?style=for-the-badge)](https://github.com/sponsors/PanSalut)

## License

MIT License with [Commons Clause](https://commonsclause.com/).

You are free to use, modify, and share this software for any purpose, including commercial use within your organization. However, you may not sell the software or offer it as a paid service.
