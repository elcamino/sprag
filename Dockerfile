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
# Pre-create the data directory so the final image can ship it owned by the
# nonroot user (65532); distroless has no shell to chown at runtime.
RUN mkdir -p /out/data

# The :nonroot variant runs as UID 65532 so a compromise of the process does
# not hand out root inside the container.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/sprag /sprag
# Named volumes copy this ownership on first use; for bind mounts the host
# directory must be writable by UID 65532 (see docker-compose.yml).
COPY --from=builder --chown=65532:65532 /out/data /data
EXPOSE 8080
ENTRYPOINT ["/sprag"]
