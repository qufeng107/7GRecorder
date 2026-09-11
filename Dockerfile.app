ARG RUNTIME_IMAGE=7grecorder-runtime:bookworm-20250811-biliup-1.2.4-v1

FROM ${RUNTIME_IMAGE}

COPY bin/7grecorder /usr/local/bin/7grecorder
