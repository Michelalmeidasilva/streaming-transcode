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
  && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/streaming-transcode /usr/local/bin/streaming-transcode
COPY --from=build /out/transcode-local /usr/local/bin/transcode-local
COPY --from=build /out/benchmark /usr/local/bin/benchmark

ENV TRANSCODE_WORKDIR=/tmp/transcode
# CMD (não ENTRYPOINT): o worker RabbitMQ é o default em dev, mas o AWS Batch
# sobrescreve o command com ["transcode-local", "<s3-key>"]. Com ENTRYPOINT fixo o
# command seria ANEXADO (rodando o worker, que falha ao conectar no RabbitMQ);
# com CMD o command do Batch SUBSTITUI e roda o binário transcode-local direto.
# Ambos os binários estão no PATH (/usr/local/bin).
CMD ["streaming-transcode"]
