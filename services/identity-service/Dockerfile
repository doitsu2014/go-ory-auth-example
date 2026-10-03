# syntax=docker/dockerfile:1.7
# Multi-stage build: static binary on distroless, non-root (02-backend-go §2).
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
COPY . .
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/identity-service ./cmd/identity-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/identity-service /identity-service
USER nonroot:nonroot
EXPOSE 8080 8081 9090
ENTRYPOINT ["/identity-service"]
CMD ["serve"]
