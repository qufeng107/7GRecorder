FROM golang:1.24.6-bookworm AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
ARG GIT_SHA=dev
RUN go build -ldflags "-X github.com/7grecorder/7grecorder/backend/internal/version.BuildSHA=${GIT_SHA}" -o /out/7grecorder ./cmd/7grecorder

FROM debian:bookworm-20250811-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --home-dir /nonexistent --shell /usr/sbin/nologin app
COPY --from=backend-builder /out/7grecorder /usr/local/bin/7grecorder
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/7grecorder"]
CMD ["all"]
