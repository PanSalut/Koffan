# Koffan

Open source shopping assistant - a simple and fast app for managing your shopping list.

## Features

- Organize products into sections (e.g., Dairy, Vegetables, Cleaning)
- Mark products as purchased
- Mark products as "uncertain" (to think about)
- Real-time synchronization (WebSocket)
- Responsive interface (mobile-first)
- Offline-ready mode
- Simple login system

## Tech Stack

- **Backend:** Go 1.21 + Fiber
- **Frontend:** HTMX + Alpine.js + Tailwind CSS
- **Database:** SQLite

## Local Setup

```bash
# Clone
git clone https://github.com/PanSalut/Koffan.git
cd Koffan

# Run
go run main.go

# App available at http://localhost:3000
```

Default password: `shopping123`

## Docker

```bash
docker-compose up -d
# App available at http://localhost:80
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | `development` | Set to `production` for secure cookies |
| `APP_PASSWORD` | `shopping123` | Login password |
| `PORT` | `80` (Docker) / `3000` (local) | Server port |
| `DB_PATH` | `./shopping.db` | Database file path |

## Deploy to Coolify

### Option 1: Docker Compose (via Git)

1. Add new resource → **Docker Compose** → Select your Git repository
2. Set domain in **Domains** section (e.g., `https://your-domain.com`)
3. Enable **Connect to Predefined Network** in Advanced settings
4. Add environment variable `APP_PASSWORD` with your password
5. Deploy

### Option 2: Docker Compose Empty (recommended)

1. Add new resource → **Docker Compose Empty**
2. Paste the following configuration (adjust domain and password):

```yaml
services:
  shopping-list:
    build: https://github.com/PanSalut/Koffan.git
    expose:
      - "80"
    environment:
      - APP_ENV=production
      - APP_PASSWORD=your-secure-password
      - PORT=80
      - DB_PATH=/data/shopping.db
    volumes:
      - shopping-data:/data
    restart: unless-stopped
    networks:
      - coolify
    labels:
      - traefik.enable=true
      - traefik.http.routers.shopping-list.rule=Host(`your-domain.com`)
      - traefik.http.routers.shopping-list.entryPoints=http
      - traefik.http.routers.shopping-list-secure.rule=Host(`your-domain.com`)
      - traefik.http.routers.shopping-list-secure.entryPoints=https
      - traefik.http.routers.shopping-list-secure.tls=true
      - traefik.http.routers.shopping-list-secure.tls.certresolver=letsencrypt
      - traefik.http.services.shopping-list.loadbalancer.server.port=80
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:80/login"]
      interval: 30s
      timeout: 3s
      retries: 3

volumes:
  shopping-data:

networks:
  coolify:
    external: true
```

3. Set domain in **Domains** section
4. Deploy

### Persistent Storage

Data is stored in `/data/shopping.db`. The `shopping-data` volume ensures your data persists across deployments.

## License

MIT
