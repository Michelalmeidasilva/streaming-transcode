FROM golang:1.26-bookworm AS build

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/streaming-transcode ./cmd/worker
RUN CGO_ENABLED=0 go build -o /out/transcode-local ./cmd/transcode-local

FROM golang:1.26-bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/streaming-transcode /usr/local/bin/streaming-transcode
COPY --from=build /out/transcode-local /usr/local/bin/transcode-local

ENV TRANSCODE_WORKDIR=/tmp/transcode
ENTRYPOINT ["streaming-transcode"]
