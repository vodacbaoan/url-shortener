# URL Shortener

A small Go URL shortener with:

- A Next.js frontend in `frontend/`
- Email/password auth with JWT access cookies and refresh-token rotation
- `POST /shorten` to create short links
- `GET /links` to list the current user's links
- `GET /me` to inspect the current authenticated user
- `GET /{shortCode}` to redirect to the original URL
- `GET /stats/{shortCode}` to view basic link analytics
- Postgres-backed storage for persistence
- Schema setup handled automatically on startup

## Stack

- Go
- `net/http`
- Chi
- Next.js
- React
- TypeScript
- PostgreSQL
- Docker for local database setup

## Requirements

- Go 1.25+
- Node.js
- Docker Desktop

## Local Setup

Start PostgreSQL 18 with Docker Compose:

```powershell
docker compose up -d
```

Local configuration lives in `.env`. Example:

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5432/url_shortener?sslmode=disable
PORT=8081
JWT_ACCESS_SECRET=replace-with-a-long-random-secret
JWT_ISSUER=url-shortener
APP_ENV=development
FRONTEND_ORIGIN=http://localhost:3000
```

Install frontend dependencies:

```powershell
cd frontend
npm install
```

## Run

Start the Go API:

```powershell
go run .
```

Start the Next.js frontend in a second terminal:

```powershell
cd frontend
npm run dev
```

Open:

```text
http://localhost:3000
```

## API

API info:

```http
GET /
```

Health check:

```http
GET /healthz
```

Sign up:

```http
POST /auth/signup
Content-Type: application/json

{
  "email": "you@example.com",
  "password": "password123"
}
```

Login:

```http
POST /auth/login
Content-Type: application/json

{
  "email": "you@example.com",
  "password": "password123"
}
```

Current user:

```http
GET /me
```

Create short URL:

```http
POST /shorten
Content-Type: application/json

{
  "url": "https://example.com"
}
```

Example response:

```json
{
  "short_code": "Ab12Cd"
}
```

Redirect:

```http
GET /Ab12Cd
```

Stats:

```http
GET /stats/Ab12Cd
```

Owned links:

```http
GET /links
```

## Notes

- Only absolute `http://` and `https://` URLs are accepted.
- Extra JSON fields in the shorten request are rejected.
- `DATABASE_URL` is required; the app always uses PostgreSQL.
- `JWT_ACCESS_SECRET` is required for signing access tokens.
- Link creation and stats are authenticated; public redirects stay public.
- The app ensures the required table/columns exist when it starts.
