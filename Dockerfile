FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
# Cross-compile for the target platform. CGO is disabled (pure-Go SQLite), so
# this is a fast native cross-build with no emulation.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/sprag ./cmd/sprag

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=builder /out/sprag /sprag
EXPOSE 8080
ENTRYPOINT ["/sprag"]
