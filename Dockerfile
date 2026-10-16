# syntax=docker/dockerfile:1

FROM golang:1.26.9-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY api ./api
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ENV APP_ENV=production \
    APP_VERSION=${VERSION}
COPY --from=build /out/api /api
COPY --from=build /out/admin /admin
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
