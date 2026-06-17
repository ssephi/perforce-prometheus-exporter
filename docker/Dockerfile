# syntax=docker/dockerfile:1.6

FROM python:3.12-slim

# Pin a Perforce release whose p4 binary we know is published. Override
# with `--build-arg P4_RELEASE=rXX.X` to pick a different one.
ARG P4_RELEASE=r24.2
ARG TARGETARCH

# Install the p4 CLI from the Perforce FTP. The URL layout is stable
# across releases: bin.<arch>/p4. We only support glibc-linux amd64/arm64,
# which is what `python:3.12-slim` runs.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    rm -rf /var/lib/apt/lists/*; \
    case "$TARGETARCH" in \
      amd64) P4_ARCH=linux26x86_64 ;; \
      arm64) P4_ARCH=linux26aarch64 ;; \
      *) echo "Unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://ftp.perforce.com/perforce/${P4_RELEASE}/bin.${P4_ARCH}/p4" \
      -o /usr/local/bin/p4; \
    chmod +x /usr/local/bin/p4; \
    /usr/local/bin/p4 -V

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY exporter.py ./
COPY perforce_exporter ./perforce_exporter

ENV EXPORTER_PORT=9117 \
    P4_BIN=p4 \
    P4_TIMEOUT_SECONDS=10

EXPOSE 9117

CMD ["python", "exporter.py"]
