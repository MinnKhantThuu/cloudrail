# syntax=docker/dockerfile:1
FROM node:22.20.0-alpine AS web
WORKDIR /src/apps/web
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

FROM golang:1.26.8-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/agent ./cmd/agent

FROM alpine:3.22 AS server
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 cloudrail
COPY --from=go-build /out/server /usr/local/bin/server
COPY --from=web /src/apps/web/dist /web
RUN mkdir -p /var/lib/cloudrail && chown 10001:10001 /var/lib/cloudrail
USER cloudrail
COPY VERSION /usr/share/cloudrail/VERSION
ENTRYPOINT ["server"]

FROM moby/buildkit:v0.33.0 AS build-tools

FROM alpine:3.22 AS agent
RUN apk add --no-cache ca-certificates
COPY --from=build-tools /usr/bin/buildctl /usr/local/bin/buildctl
COPY deploy/install-build-tools.sh /tmp/install-build-tools.sh
RUN sh /tmp/install-build-tools.sh && rm /tmp/install-build-tools.sh
COPY --from=go-build /out/agent /usr/local/bin/agent
COPY VERSION /usr/share/cloudrail/VERSION
ENTRYPOINT ["agent"]
