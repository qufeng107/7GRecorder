FROM golang:1.24.6-bookworm AS backend-builder
WORKDIR /src/backend
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY backend/ ./
RUN go mod tidy
ARG GIT_SHA=dev
RUN go build -ldflags "-X github.com/7grecorder/7grecorder/backend/internal/version.BuildSHA=${GIT_SHA}" -o /out/7grecorder ./cmd/7grecorder

FROM debian:bookworm-20250811-slim AS runtime
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=backend-builder /out/7grecorder /usr/local/bin/7grecorder
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/7grecorder"]
CMD ["all"]
