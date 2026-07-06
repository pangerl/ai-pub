FROM node:22-alpine AS web-build

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.26.2 AS api-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=1 go build -o /out/ai-pub ./cmd/server

FROM debian:bookworm-slim AS app

WORKDIR /app
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates openssh-client \
  && rm -rf /var/lib/apt/lists/*
COPY --from=api-build /out/ai-pub /app/ai-pub
COPY --from=api-build /src/migrations /app/migrations
COPY --from=web-build /src/web/dist /app/web/dist
EXPOSE 8080
ENTRYPOINT ["/app/ai-pub"]
