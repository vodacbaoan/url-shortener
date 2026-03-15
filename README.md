# URL Shortener

A small Go URL shortener with:

- `POST /shorten` to create short links
- `GET /{shortCode}` to redirect to the original URL
- Postgres-backed storage for persistence
- in-memory fallback when `DATABASE_URL` is not set

## Stack

- Go
- `net/http`
- PostgreSQL
- Docker for local database setup

## Requirements

- Go 1.23+
- Docker Desktop

## Local Setup

Start PostgreSQL 18 in Docker:

```powershell
docker run --name url-shortener-db -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=url_shortener -p 5432:5432 -v url_shortener_pg18:/var/lib/postgresql -d postgres:18
```

Create the table if needed:

```sql
CREATE TABLE IF NOT EXISTS shortened_urls (
    short_code TEXT PRIMARY KEY,
    target_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Local configuration lives in `.env`. Example:

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5432/url_shortener?sslmode=disable
PORT=8081
```

## Run

```powershell
go run .
```

## API

Health check:

```http
GET /
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

## Notes

- Only absolute `http://` and `https://` URLs are accepted.
- Extra JSON fields in the shorten request are rejected.
- If `DATABASE_URL` is missing, the app falls back to in-memory storage.
- In-memory data is lost when the server stops.
