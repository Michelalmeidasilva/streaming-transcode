# Streaming Transcode

Worker de transcoding para o pipeline VOD.

## O que este serviço faz

O worker consome `video.upload.completed` da exchange RabbitMQ `video_events`, baixa o arquivo original do MinIO/S3, gera HLS/DASH com FFmpeg e atualiza o Event Gateway.

Fluxo principal:

1. `streaming-platform-upload` envia o arquivo original para storage.
2. `streaming-ingest` recebe `upload.completed` em `POST /api/v1/events`.
3. `streaming-ingest` publica `video.upload.completed` no RabbitMQ.
4. `streaming-transcode` consome a mensagem em `transcode.jobs`.
5. O worker baixa o arquivo original, gera renditions H.264/AAC, HLS e DASH.
6. Os outputs são enviados para MinIO/S3 em `transcoded/{videoId}/...`.
7. Métricas são enviadas para `metrics/{videoId}/...`.
8. O upload-state é atualizado para `queued`, `transcoding`, `packaging`, `ready` ou `failed`.

## Pré-requisitos

- Docker e Docker Compose.
- Go para rodar testes locais do worker.
- Node.js/npm para rodar testes do `streaming-platform-upload`, quando validar o fluxo de upload.
- FFmpeg/ffprobe apenas se executar o worker fora do Docker. A imagem Docker já instala as dependências necessárias.

## Configuração

Variáveis principais:

```text
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
STORAGE_PROVIDER=minio
STORAGE_BUCKET=videos
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=admin
MINIO_SECRET_KEY=password123
EVENT_GATEWAY_URL=http://event-gateway:8080/api/v1
TRANSCODE_WORKDIR=/tmp/transcode
TRANSCODE_PROFILE=production-h264-hls-dash
TRANSCODE_CODECS=h264
TRANSCODE_RETRY_QUEUE=transcode.retry
TRANSCODE_DEAD_QUEUE=transcode.dead
TRANSCODE_MAX_ATTEMPTS=3
TRANSCODE_RETRY_DELAY_SECONDS=60
```

Variáveis opcionais relevantes:

```text
TRANSCODE_QUEUE=transcode.jobs
TRANSCODE_BINDING_KEY=video.upload.completed
FFMPEG_PRESET=veryfast
TRANSCODE_MAX_FILE_SIZE_MB=0
```

`TRANSCODE_CODECS` aceita uma lista separada por virgula. Codecs suportados: `h264`, `h265`, `av1`, `vp9` e `vvc`. O alias `vpc` tambem e aceito e normalizado para `vvc`.

`TRANSCODE_MAX_FILE_SIZE_MB` define o limite de tamanho do arquivo fonte em megabytes. O valor `0` (padrão) desativa a verificação. Quando configurado, jobs com `size` acima do limite são rejeitados como erro terminal antes do download, sem consumir banda ou CPU.

No `streaming-platform-upload`, a migração para o namespace canônico de entrada pode ser testada com:

```text
UPLOAD_RAW_PREFIX_ENABLED=true
```

Quando ativa, a chave de upload muda de `{videoId}/{filename}` para `raw/{videoId}/{filename}`.

## Como rodar com Docker Compose

Suba a infraestrutura e o worker pelo Compose compartilhado:

```bash
cd ../infra
docker compose up --build
```

Serviços usados pelo fluxo:

```text
RabbitMQ UI: http://localhost:15672
RabbitMQ login: guest / guest
MinIO Console: http://localhost:9001
MinIO login: admin / password123
Event Gateway: http://localhost:8080/api/v1
Mongo Express: http://localhost:8081
```

Verifique se o worker iniciou:

```bash
cd ../infra
docker compose ps
docker compose logs --tail=100 streaming-transcode
```

Log esperado:

```text
streaming-transcode worker started queue=transcode.jobs binding=video.upload.completed
```

## Como rodar o worker localmente

Use esta opção quando RabbitMQ, MinIO e `streaming-ingest` já estiverem rodando no Docker, mas você quiser executar o worker pelo Go:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
STORAGE_PROVIDER=minio \
STORAGE_BUCKET=videos \
MINIO_ENDPOINT=http://localhost:9000 \
MINIO_ACCESS_KEY=admin \
MINIO_SECRET_KEY=password123 \
EVENT_GATEWAY_URL=http://localhost:8080/api/v1 \
TRANSCODE_WORKDIR=/tmp/transcode \
TRANSCODE_PROFILE=production-h264-hls-dash \
TRANSCODE_CODECS=h264 \
TRANSCODE_QUEUE=transcode.jobs \
TRANSCODE_RETRY_QUEUE=transcode.retry \
TRANSCODE_DEAD_QUEUE=transcode.dead \
TRANSCODE_BINDING_KEY=video.upload.completed \
TRANSCODE_MAX_ATTEMPTS=3 \
TRANSCODE_RETRY_DELAY_SECONDS=60 \
TRANSCODE_MAX_FILE_SIZE_MB=2048 \
go run ./cmd/worker
```

## Como testar automaticamente

No `streaming-transcode`:

```bash
go test ./...
go test ./internal/... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```

Cobertura esperada no estado atual:

```text
total: 81.7%
```

## Como analisar bitrates de videos do Vimeo

O repositorio inclui um CLI que usa `yt-dlp` para consultar o Vimeo e listar os bitrates encontrados em arquivos progressivos e variantes HLS/DASH.

Usando a lista padrao em [dataset/vimeo_urls.txt](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/dataset/vimeo_urls.txt):

```bash
go run ./cmd/vimeo-analyzer -input dataset/vimeo_urls.txt > vimeo-bitrates.csv
```

Saida em JSON:

```bash
go run ./cmd/vimeo-analyzer -input dataset/vimeo_urls.txt -format json > vimeo-bitrates.json
```

Tambem aceita URLs ou IDs na linha de comando:

```bash
go run ./cmd/vimeo-analyzer https://vimeo.com/1078990193 339952895
```

Se algum video exigir sessao do navegador:

```bash
go run ./cmd/vimeo-analyzer -input dataset/vimeo_urls.txt --cookies-from-browser chrome > vimeo-bitrates.csv
```

Colunas principais da saida CSV:

```text
delivery        progressive, hls ou dash
variant         qualidade detectada, como 1080p ou 720p
bitrate_kbps    bitrate da variante em kbps
bandwidth_bps   largura de banda original informada pelo manifesto/config
source_url      URL do arquivo ou da variante
error           erro de acesso ou parsing, quando existir
```

No `streaming-ingest`:

```bash
cd ../streaming-ingest
go test ./...
```

No `streaming-platform-upload`:

```bash
cd ../streaming-platform-upload
npm test -- --runInBand src/lib/services/__tests__/UploadService.test.ts
npx tsc --noEmit
```

Validação do Compose:

```bash
cd ../infra
docker compose config --quiet
```

## Como rodar benchmarks locais com compose generico

O `compose.yaml` deste repositorio tem um unico servico generico, `transcode-local`, para rodar o `transcode-local` em Linux com o mesmo padrao de observabilidade.

Esse modelo e mais compativel com cloud porque:

- nao replica um servico por codec
- usa os mesmos parametros que um job/evento usaria em producao
- permite variar codec, resolucao, bitrate, entrada e saida sem editar o compose

Build da imagem local:

```bash
mkdir -p dist
env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE=/private/tmp/go-build go build -o dist/transcode-local-linux-amd64 ./cmd/transcode-local
docker compose -f compose.yaml build
```

Exemplo AV1 720p:

```bash
docker compose -f compose.yaml up --abort-on-container-exit transcode-local
```

Exemplo H.265 1080p com overrides:

```bash
INPUT=/workspace/outputs/reference-pipeline-preselecao-y4m/comercial-cortes/847661749-Major-Fade-Active-Seal-Moisturizer-chicwithkels/reference.y4m \
OUTPUT=/workspace/outputs/docker-validation/847661749-h265-1080p.mp4 \
WIDTH=1920 \
HEIGHT=1080 \
BITRATE_KBPS=6000 \
TRANSCODE_CODEC=h265 \
docker compose -f compose.yaml up --abort-on-container-exit transcode-local
```

Exemplo VP9 720p:

```bash
TRANSCODE_CODEC=vp9 \
INPUT=/workspace/outputs/reference-pipeline-preselecao-y4m/comercial-cortes/847661749-Major-Fade-Active-Seal-Moisturizer-chicwithkels/reference.y4m \
OUTPUT=/workspace/outputs/docker-validation/847661749-vp9-720p.mp4 \
WIDTH=1280 \
HEIGHT=720 \
BITRATE_KBPS=3000 \
docker compose -f compose.yaml up --abort-on-container-exit transcode-local
```

Limpeza:

```bash
docker compose -f compose.yaml down
```

O log final do `transcode-local` sempre inclui:

```text
observability supported=... samples=... elapsed=... rtf=... avgCpu=... maxCpu=... outputSize=... outputBitrate=...
```

## Como enviar parametros de transcoding no job

O worker principal agora aceita parametros de transcoding no payload de `video.upload.completed`.

Exemplo:

```json
{
  "videoId": "video-benchmark-001",
  "objectKey": "raw/video-benchmark-001/source.y4m",
  "bucket": "videos",
  "transcode": {
    "profile": "benchmark-av1-720p",
    "preset": "slow",
    "renditions": [
      {
        "name": "custom-av1-720p",
        "codec": "av1",
        "width": 1280,
        "height": 720,
        "bitrateKbps": 2500
      }
    ]
  }
}
```

Campos aceitos em `transcode`:

```text
profile      opcional, sobrescreve o profile logico do job
preset       opcional, sobrescreve o preset do ffmpeg para as renditions do job
codecs       opcional, lista de codecs quando quiser manter o plano automatico por resolucao
renditions   opcional, lista explicita de saidas com codec, largura, altura e bitrate
```

Regra de fallback:

- se `transcode.renditions` vier preenchido, o worker usa exatamente essas saídas
- se `transcode.renditions` nao vier, mas `transcode.codecs` vier, o worker usa o plano automatico com esses codecs
- se nada vier, o worker continua usando o comportamento atual baseado em `TRANSCODE_CODECS`

## Como validar o fluxo completo manualmente

Este fluxo não depende da UI. Ele envia `sample.mp4` para o MinIO, cria o estado inicial no `streaming-ingest`, publica `upload.completed` e valida os outputs gerados.

Se repetir o teste com o mesmo `videoId`, o worker pode detectar que `transcoded/{videoId}/hls/master.m3u8` já existe e apenas republicar `ready`. Para forçar uma nova execução completa, use outro `videoId` ou remova os objetos antigos no MinIO.

1. Suba o ambiente:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker compose up --build
```

2. Em outro terminal, envie o arquivo de exemplo para o bucket `videos`:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker run --rm --network vod-network --entrypoint sh \
  -v "$PWD/../streaming-transcode:/data" \
  minio/mc \
  -c 'mc alias set local http://minio:9000 admin password123 >/dev/null && mc cp /data/sample.mp4 local/videos/e2e-sample/sample.mp4'
```

3. Crie o registro inicial no upload-state:

```bash
curl -sS -X PUT http://127.0.0.1:8080/api/v1/upload-state/videos/e2e-sample \
  -H 'Content-Type: application/json' \
  -d '{"id":"e2e-sample","filename":"e2e-sample/sample.mp4","originalName":"sample.mp4","title":"sample.mp4","size":687391,"status":"processing","progress":100,"createdAt":"2026-05-09T12:00:00Z","updatedAt":"2026-05-09T12:00:00Z","provider":"minio"}'
```

4. Publique o evento `upload.completed`:

```bash
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"videoId":"e2e-sample","filename":"e2e-sample/sample.mp4","objectKey":"e2e-sample/sample.mp4","originalName":"sample.mp4","size":687391,"provider":"minio","bucket":"videos","sourceETag":"local-sample-etag","sourceVersion":"v1"}}'
```

5. Acompanhe os logs:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker compose logs -f streaming-transcode event-gateway
```

6. Verifique os outputs no MinIO:

```bash
docker run --rm --network vod-network --entrypoint sh minio/mc \
  -c 'mc alias set local http://minio:9000 admin password123 >/dev/null && mc ls --recursive local/videos/transcoded/e2e-sample && mc ls --recursive local/videos/metrics/e2e-sample'
```

Arquivos esperados:

```text
transcoded/e2e-sample/hls/master.m3u8
transcoded/e2e-sample/hls/{rendition}/...
transcoded/e2e-sample/dash/manifest.mpd
metrics/e2e-sample/media-info.json
metrics/e2e-sample/transcode-result.json
```

7. Verifique o estado final:

```bash
curl -sS http://127.0.0.1:8080/api/v1/upload-state/videos/e2e-sample
```

Campos esperados:

```text
status=ready
processingStatus=ready
transcode.fingerprint preenchido
transcode.attempt=1
playback.hlsManifestPath=transcoded/e2e-sample/hls/master.m3u8
playback.dashManifestPath=transcoded/e2e-sample/dash/manifest.mpd
metrics.metricsPath=metrics/e2e-sample/transcode-result.json
```

## Como validar retry e DLQ

O worker usa três filas:

```text
transcode.jobs
transcode.retry
transcode.dead
```

Comportamento esperado:

- falhas transitórias são publicadas em `transcode.retry`
- mensagens expiradas em `transcode.retry` voltam para `video.upload.completed`
- erros terminais e tentativas esgotadas vão para `transcode.dead`

Erros terminais — rejeitados imediatamente sem retry, sem download e sem uso de CPU:

| Cenário | `reason` no payload |
|---|---|
| `videoId` ausente | — (parse falha antes do job) |
| Extensão de arquivo não suportada | — (parse falha antes do job) |
| Codec inválido em `transcode.codecs` | `invalid_transcode_request` |
| `width` ou `height` ≤ 0 em rendition | `invalid_transcode_request` |
| Codec desconhecido em rendition | `invalid_transcode_request` |
| `size` acima de `TRANSCODE_MAX_FILE_SIZE_MB` | `file_too_large` |

Extensões de arquivo aceitas: `.mp4`, `.m4v`, `.mov`, `.mkv`, `.webm`, `.ts`, `.y4m`, `.m3u8`.

Para testar cada caso, publique via Event Gateway e observe a DLQ:

```bash
# videoId ausente
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"filename":"invalid/sample.mp4","objectKey":"invalid/sample.mp4","bucket":"videos"}}'

# extensão não suportada
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"videoId":"bad-ext","objectKey":"bad-ext/video.exe","bucket":"videos"}}'

# codec inválido no TranscodeRequest
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"videoId":"bad-codec","objectKey":"bad-codec/video.mp4","bucket":"videos","transcode":{"codecs":["xyz"]}}}'

# dimensões inválidas em rendition
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"videoId":"bad-dim","objectKey":"bad-dim/video.mp4","bucket":"videos","transcode":{"renditions":[{"width":0,"height":720,"codec":"h264"}]}}}'
```

Verifique a DLQ após cada publicação:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker exec rabbitmq rabbitmqctl list_queues name messages_ready messages_unacknowledged | grep transcode.dead
```

Resultado esperado — contador de `messages_ready` aumenta em 1 para cada evento inválido:

```text
transcode.dead    1    0
```

Também é possível inspecionar e ler o payload completo pela UI do RabbitMQ em `http://localhost:15672` → **Queues** → `transcode.dead` → **Get messages**.

## Como validar o fluxo pela UI de upload

Quando o `streaming-platform-upload` estiver configurado para enviar eventos ao `streaming-ingest`:

```bash
cd ../streaming-platform-upload
npm run dev
```

Valide no navegador:

- o upload termina em `processing`, não em `ready` imediato
- o `streaming-ingest` recebe `upload.completed`
- o worker consome `video.upload.completed`
- o estado evolui por `queued`, `transcoding`, `packaging` e `ready`
- os manifests HLS/DASH aparecem no documento do vídeo em `playback`

## Contratos de eventos

Os contratos detalhados estão em:

```text
EVENT-CONTRACTS.md
```

Eventos emitidos pelo worker:

```text
transcode.queued
transcode.started
transcode.progress
packaging.completed
transcode.completed
ready
transcode.failed
```

## Limitações conhecidas

- O retry usa uma fila com TTL fixo. Backoff exponencial por tentativa ainda não foi implementado.
- A propagação completa de ETag/version depende dos adapters de upload.
- Benchmark de codecs, VMAF/SSIM/PSNR, GPU/cloud e validação de playback E2E continuam como fases futuras.
