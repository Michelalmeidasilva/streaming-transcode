FROM golang:1.26-bookworm AS build

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/streaming-transcode ./cmd/worker
RUN CGO_ENABLED=0 go build -o /out/transcode-local ./cmd/transcode-local
RUN CGO_ENABLED=0 go build -o /out/benchmark ./cmd/benchmark

FROM golang:1.26-bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates curl xz-utils \
  && rm -rf /var/lib/apt/lists/*

# The apt ffmpeg encodes (libx264/libx265/libsvtav1) but has NO libvmaf. For the
# R-D benchmark's VMAF measurement we drop in a static BtbN ffmpeg (libvmaf built
# in) as `ffmpeg-vmaf`. The encode path is unchanged — production still uses the
# apt `ffmpeg`; this binary is only invoked for VMAF (rd mode) via VMAF_FFMPEG_PATH.
RUN curl -fsSL https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n7.1-latest-linux64-gpl-7.1.tar.xz -o /tmp/ffv.tar.xz \
  && tar -xJf /tmp/ffv.tar.xz -C /tmp --strip-components=2 --wildcards '*/bin/ffmpeg' \
  && mv /tmp/ffmpeg /usr/local/bin/ffmpeg-vmaf \
  && rm -f /tmp/ffv.tar.xz

COPY --from=build /out/streaming-transcode /usr/local/bin/streaming-transcode
COPY --from=build /out/transcode-local /usr/local/bin/transcode-local
COPY --from=build /out/benchmark /usr/local/bin/benchmark

ENV TRANSCODE_WORKDIR=/tmp/transcode VMAF_FFMPEG_PATH=ffmpeg-vmaf
# CMD (não ENTRYPOINT): o worker RabbitMQ é o default em dev, mas o AWS Batch
# sobrescreve o command com ["transcode-local", "<s3-key>"]. Com ENTRYPOINT fixo o
# command seria ANEXADO (rodando o worker, que falha ao conectar no RabbitMQ);
# com CMD o command do Batch SUBSTITUI e roda o binário transcode-local direto.
# Ambos os binários estão no PATH (/usr/local/bin).
CMD ["streaming-transcode"]
