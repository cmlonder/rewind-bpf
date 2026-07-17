FROM golang:1.24.3-bookworm

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bpftrace \
        bpftool \
        clang \
        llvm \
        libbpf-dev \
        libelf-dev \
        zlib1g-dev \
        build-essential \
        git \
        make \
        mount \
        util-linux \
        iproute2 \
        kmod \
        fio \
        ca-certificates \
        jq \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/rewind
COPY . /opt/rewind

RUN go test ./... && go build -o /usr/local/bin/rewind ./cmd/rewind

CMD ["sleep", "infinity"]
