# URL Shortener

A small URL shortener with:

- A Go API in `backend/`
- A Next.js frontend in `frontend/`
- Email/password auth with JWT access cookies and refresh-token rotation
- `POST /shorten` to create short links
- `GET /links` to list the current user's links
- `GET /me` to inspect the current authenticated user
- `GET /{shortCode}` to redirect to the original URL
- `GET /stats/{shortCode}` to view basic link analytics
- Postgres-backed storage for persistence
- Versioned SQL migrations handled automatically on startup

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

Run the full project with Docker Compose:

```powershell
docker compose up --build
```

Open:

```text
http://localhost:3000
```

Docker Compose starts:

- PostgreSQL on `localhost:5432`
- Go API on `localhost:8081`
- Next.js frontend on `localhost:3000`

For manual backend runs without Docker, local configuration lives in
`backend/.env`. You can copy `backend/.env.example` and adjust values as needed.
Example:

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

Start the Go API manually:

```powershell
cd backend
go run .
```

Start the Next.js frontend manually in a second terminal:

```powershell
cd frontend
npm run dev
```

Run backend tests:

```powershell
cd backend
go test ./...
```

Run backend integration tests against Postgres:

```powershell
cd backend
$env:INTEGRATION_DATABASE_URL="postgres://postgres:postgres@localhost:5432/url_shortener?sslmode=disable"
go test ./...
```

The integration tests create a temporary Postgres schema and drop it after the
test run.

## Migrations

Database migrations live in:

```text
backend/migrations/
```

Migration files are ordered by numeric prefix, for example:

```text
000001_create_auth_and_urls.sql
```

The Go API runs any pending migrations on startup and records applied versions
in the `schema_migrations` table.

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
- The app runs pending SQL migrations when it starts.
