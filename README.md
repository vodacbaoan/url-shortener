# URL Shortener

A small Go URL shortener with:

- A built-in browser UI at `/`
- `POST /shorten` to create short links
- `GET /{shortCode}` to redirect to the original URL
- `GET /stats/{shortCode}` to view basic link analytics
- Postgres-backed storage for persistence
- Schema setup handled automatically on startup

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

Home page:

```http
GET /
```

Health check:

```http
GET /healthz
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

## Notes

- Only absolute `http://` and `https://` URLs are accepted.
- Extra JSON fields in the shorten request are rejected.
- `DATABASE_URL` is required; the app always uses PostgreSQL.
- The app ensures the required table/columns exist when it starts.
