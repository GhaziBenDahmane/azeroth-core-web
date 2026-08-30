# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /src
ARG NPM_CONFIG_REGISTRY=https://registry.npmjs.org/
ARG NPM_CONFIG_REPLACE_REGISTRY_HOST=never
ARG NPM_CONFIG_STRICT_SSL=true
COPY package.json package-lock.json ./
RUN --mount=type=secret,id=build_ca,required=false \
    if [ -f /run/secrets/build_ca ]; then NODE_EXTRA_CA_CERTS=/run/secrets/build_ca npm ci --no-audit --no-fund; else npm ci --no-audit --no-fund; fi
COPY astro.config.mjs tsconfig.json webcore.config.scss ./
COPY src ./src
COPY public ./public
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS api
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN --mount=type=secret,id=build_ca,required=false \
    if [ -f /run/secrets/build_ca ]; then SSL_CERT_FILE=/run/secrets/build_ca go mod download; else go mod download; fi
COPY . .
COPY --from=ui /src/dist ./dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /portal .

FROM alpine:3.22
COPY --from=api /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S portal && adduser -S -G portal portal
COPY --from=api /portal /usr/local/bin/portal
USER portal
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
ENTRYPOINT ["portal"]
