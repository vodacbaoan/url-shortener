FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/url-shortener .

FROM alpine:3.22

WORKDIR /app

RUN adduser -D -H appuser

COPY --from=builder /app/url-shortener ./url-shortener

USER appuser

EXPOSE 8081

CMD ["./url-shortener"]
