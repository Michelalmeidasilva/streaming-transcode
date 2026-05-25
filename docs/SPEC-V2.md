# Streaming Transcode - SPEC V2

## 1. Objetivo

Esta especificacao atualiza o plano de `SPEC.md` para o estado real dos projetos existentes em `infra`, `streaming-ingest` e `streaming-platform-upload`.

O objetivo da V2 e transformar o planejamento de transcoding em um servico integrado ao pipeline VOD atual:

```text
streaming-platform-upload
  -> Object Storage bruto
  -> streaming-ingest / Event Gateway
  -> RabbitMQ video_events
  -> streaming-transcode worker
  -> Object Storage transcoded/
  -> streaming-ingest metadata/status
  -> streaming-platform-upload UI
```

O `SPEC.md` original continua valido como diretriz de benchmark de codecs, qualidade, custo e desempenho. A V2 define como encaixar esse trabalho no que ja foi implementado.

## 2. Paralelo Entre SPEC Original e Projetos Existentes

| Tema do `SPEC.md` original | Estado atual nos projetos | Decisao V2 |
| --- | --- | --- |
| Ingestao de video | `streaming-platform-upload` ja implementa upload multipart S3/MinIO via Next.js, presigned URLs, progresso e persistencia remota no ingest. | Reutilizar o upload existente. O transcode nao recebe arquivo por HTTP. Ele consome eventos e le do storage. |
| Storage bruto | `infra` sobe MinIO local com bucket `videos`; Terraform em `infra/aws` cria bucket S3 com CORS, criptografia e lifecycle para `raw/` e `transcoded/`. | Padronizar chaves e prefixos para separar origem, thumbnails, saidas e metricas. |
| Eventos e fila | `streaming-ingest` recebe `POST /api/v1/events`, webhooks de storage e publica na exchange RabbitMQ `video_events` com routing key `video.<eventType>`. | O transcode deve declarar fila propria ligada a `video_events`, inicialmente em `video.upload.completed`. |
| Metadados | `streaming-ingest` persiste `videos`, `events` e `upload_sessions` em MongoDB; `streaming-platform-upload` consulta e atualiza via `/upload-state/videos`. | Expandir o documento de video com campos de transcode, sem criar outro cadastro paralelo. |
| Thumbnail | `streaming-platform-upload` ja gera thumbnail com FFmpeg ou fallback apos upload. | Manter thumbnail como recurso do upload. O transcode pode gerar thumbnails de renditions futuramente, mas nao e bloqueante. |
| Transcoding | `streaming-transcode` ainda contem apenas planejamento, `sample.mp4` e configuracoes; nao ha worker implementado. | Implementar worker como consumidor RabbitMQ + FFmpeg/Shaka Packager + storage adapter. |
| Benchmark de codecs | `SPEC.md` define H.264, H.265, H.266, VP9, AV1, VMAF/SSIM/PSNR, custo local/EC2. | Tratar benchmark como modo operacional separado do fluxo de producao. |
| Empacotamento HLS/DASH/CMAF | `streaming-platform-upload/SPEC.md` cita CMAF e ponto de integracao com transcode, mas nao ha empacotamento final. | O worker deve produzir HLS/DASH/CMAF em `transcoded/{videoId}/`. |

## 3. Arquitetura Integrada

```text
Usuario
  |
  v
streaming-platform-upload
  - Next.js 14
  - multipart upload
  - presigned URLs
  - status/progresso
  |
  | escreve objeto original
  v
MinIO/S3 bucket videos
  |
  | evento de aplicacao ou webhook de storage
  v
streaming-ingest
  - Go/Fiber Event Gateway
  - MongoDB videos/events/upload_sessions
  - RabbitMQ publisher
  |
  | exchange: video_events
  | routing key: video.upload.completed
  v
streaming-transcode
  - RabbitMQ consumer
  - ffprobe metadata
  - FFmpeg transcoding
  - Shaka Packager/FFmpeg segmenter
  - quality/performance metrics
  |
  | escreve saidas
  v
MinIO/S3 bucket videos
  - raw/original or {videoId}/{filename}
  - thumbnails/{videoId}.jpg
  - transcoded/{videoId}/...
  - metrics/{videoId}/...
  |
  | atualiza estado/eventos
  v
streaming-ingest
  - transcode/ready/error events
```

## 4. Estado Atual Por Projeto

### 4.1 infra

Ja existe:

- `docker-compose.yml` com MongoDB, RabbitMQ, Redis, MinIO, Event Gateway e Mongo Express.
- Rede Docker compartilhada `vod-network`.
- MinIO local na porta `9000`, console `9001`, bucket `videos`.
- RabbitMQ na porta `5672`, management `15672`.
- MongoDB em `27017`.
- Terraform AWS para bucket S3 e usuario IAM com permissoes de multipart upload.
- Lifecycle S3 ja pensando em `transcoded/` e `raw/`.

Lacunas para transcode:

- Nao ha container/servico `streaming-transcode` no compose.
- Nao ha definicao de filas RabbitMQ especificas para transcoding.
- Nao ha volumes temporarios para processamento FFmpeg.
- Nao ha configuracao de DLQ/retry/backoff.
- Nao ha imagem com FFmpeg, libvmaf, Shaka Packager, VVenC ou encoders GPU.

### 4.2 streaming-ingest

Ja existe:

- API Go/Fiber em `/api/v1`.
- `POST /events` para eventos da aplicacao.
- `POST /webhooks/storage/:provider` para eventos MinIO/S3.
- Publisher RabbitMQ na exchange topic `video_events`.
- Routing key gerada como `video.<eventType>`, por exemplo `video.upload.completed`.
- Persistencia MongoDB de eventos e videos.
- Persistencia de estado de upload para o `streaming-platform-upload` em `/upload-state/...`.

Lacunas para transcode:

- O OpenAPI atual nao declara eventos de transcoding.
- O schema de `Video` nao possui campos de renditions, manifestos, metricas, jobId ou erro detalhado.
- Nao ha endpoint dedicado para workers atualizarem progresso de processamento.
- O publisher publica payloads genericos; o contrato do payload de `upload.completed` precisa ser estabilizado.

### 4.3 streaming-platform-upload

Ja existe:

- Next.js 14 com upload multipart.
- `S3Adapter`, `MinIOAdapter` e `MemoryAdapter`.
- Presigned URLs e fluxo `initiate -> chunk -> complete`.
- Persistencia remota via `IngestUploadStateClient`.
- Event dispatcher para enviar eventos ao Event Gateway quando `EVENT_GATEWAY_URL` esta configurado.
- Thumbnail assíncrona com FFmpeg/fallback.
- UI com lista, status e download.

Lacunas para transcode:

- `completeUpload` marca video como `processing`, mas atualmente agenda `ready` apos 2 segundos, sem esperar transcode real.
- Eventos `video.transcoded` existem na especificacao, mas nao estao encaminhados no dispatcher atual.
- A UI nao diferencia claramente `uploaded`, `queued`, `transcoding`, `packaging`, `ready` e `error`.
- A listagem ainda nao consome manifestos HLS/DASH nem renditions geradas.

### 4.4 streaming-transcode

Ja existe:

- `SPEC.md` com estudo tecnico de codecs, FPS, renditions, metricas, custo e benchmark.
- `sample.mp4` para experimentacao local.

Nao existe ainda:

- Worker executavel.
- Contrato de mensagem.
- Dockerfile.
- Storage adapter.
- Consumer RabbitMQ.
- Pipeline FFmpeg.
- Persistencia de metricas.
- Testes automatizados.

## 5. Contratos De Eventos V2

### 5.1 Evento De Entrada Para Transcode

O worker deve consumir da exchange `video_events` com binding:

```text
exchange: video_events
type: topic
queue: transcode.jobs
routing key inicial: video.upload.completed
```

Payload normalizado esperado:

```json
{
  "eventType": "upload.completed",
  "videoId": "uuid",
  "filename": "uuid/original-file.mp4",
  "originalName": "original-file.mp4",
  "size": 1048576000,
  "provider": "minio",
  "bucket": "videos",
  "objectKey": "uuid/original-file.mp4",
  "url": "http://localhost:9000/videos/uuid/original-file.mp4",
  "occurredAt": "2026-05-09T12:00:00Z"
}
```

Enquanto o ingest ainda publicar payloads menos completos, o worker deve aceitar fallback para:

- `videoId`
- `filename`
- `size`
- `provider`

Se `objectKey` nao vier no evento, usar `filename` como chave do objeto.

### 5.2 Eventos Emitidos Pelo Transcode

O worker deve publicar de volta na mesma exchange ou chamar `POST /api/v1/events`.

Regra importante do projeto atual: `streaming-ingest` transforma `eventType` em routing key usando `video.<eventType>`. Portanto:

- Ao chamar `POST /api/v1/events`, usar `eventType` sem prefixo `video.`.
- Ao publicar diretamente no RabbitMQ, usar a routing key completa com prefixo `video.`.

Eventos minimos:

```text
eventType HTTP: transcode.queued      -> routing key: video.transcode.queued
eventType HTTP: transcode.started     -> routing key: video.transcode.started
eventType HTTP: transcode.progress    -> routing key: video.transcode.progress
eventType HTTP: transcode.completed   -> routing key: video.transcode.completed
eventType HTTP: transcode.failed      -> routing key: video.transcode.failed
eventType HTTP: packaging.completed   -> routing key: video.packaging.completed
eventType HTTP: ready                 -> routing key: video.ready
```

Exemplo de `transcode.started` via HTTP:

```json
{
  "eventType": "transcode.started",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-job",
    "sourceKey": "uuid/original-file.mp4",
    "profile": "production-h264-hls-dash",
    "startedAt": "2026-05-09T12:01:00Z"
  }
}
```

Exemplo de `transcode.completed` via HTTP:

```json
{
  "eventType": "transcode.completed",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-job",
    "status": "completed",
    "durationSeconds": 300,
    "elapsedSeconds": 180,
    "rtf": 0.6,
    "renditions": [
      {
        "name": "1080p",
        "resolution": "1920x1080",
        "codec": "h264",
        "bitrateKbps": 6000,
        "manifestPath": "transcoded/uuid/hls/master.m3u8"
      }
    ],
    "dashManifestPath": "transcoded/uuid/dash/manifest.mpd",
    "hlsManifestPath": "transcoded/uuid/hls/master.m3u8",
    "metricsPath": "metrics/uuid/transcode-result.json",
    "completedAt": "2026-05-09T12:04:00Z"
  }
}
```

## 6. Modelo De Video V2

O documento persistido em `streaming-ingest` deve ser expandido sem quebrar os campos atuais.

Campos atuais relevantes:

```text
id
filename
originalName
title
size
status
progress
url
downloadUrl
thumbnailUrl
thumbnailStatus
mimeType
provider
createdAt
updatedAt
```

Campos V2 propostos:

```json
{
  "processingStatus": "queued|transcoding|packaging|ready|failed",
  "source": {
    "bucket": "videos",
    "key": "uuid/original-file.mp4",
    "provider": "minio",
    "size": 1048576000,
    "mimeType": "video/mp4"
  },
  "mediaInfo": {
    "durationSeconds": 300,
    "width": 3840,
    "height": 2160,
    "fps": 29.97,
    "videoCodec": "h264",
    "audioCodec": "aac",
    "bitrateKbps": 12000
  },
  "transcode": {
    "jobId": "uuid-job",
    "profile": "production-h264-hls-dash",
    "attempt": 1,
    "startedAt": "2026-05-09T12:01:00Z",
    "completedAt": "2026-05-09T12:04:00Z",
    "error": null
  },
  "playback": {
    "hlsManifestPath": "transcoded/uuid/hls/master.m3u8",
    "dashManifestPath": "transcoded/uuid/dash/manifest.mpd",
    "renditions": []
  },
  "metrics": {
    "rtf": 0.6,
    "elapsedSeconds": 180,
    "outputSizeMb": 420,
    "vmaf": 93.4,
    "ssim": 0.98,
    "psnr": 42.1,
    "metricsPath": "metrics/uuid/transcode-result.json"
  }
}
```

## 7. Storage Layout V2

Padrao recomendado para MinIO/S3:

```text
videos/
  raw/{videoId}/{originalName}
  thumbnails/{videoId}.jpg
  thumbnails/{videoId}-fallback.jpg
  transcoded/{videoId}/
    hls/
      master.m3u8
      1080p/index.m3u8
      1080p/segment-00001.m4s
      720p/index.m3u8
    dash/
      manifest.mpd
      1080p/segment-00001.m4s
      720p/segment-00001.m4s
  metrics/{videoId}/
    media-info.json
    transcode-result.json
    benchmark-runs.jsonl
```

Compatibilidade com o upload atual:

- Hoje o upload usa chave `${videoId}/${filename}`.
- A V2 deve suportar essa chave existente para nao quebrar uploads atuais.
- Nova gravacao pode migrar para `raw/{videoId}/{filename}` quando upload e ingest forem ajustados juntos.

## 8. Pipeline Do Worker

### 8.1 Fluxo Principal

```text
1. Consumir video.upload.completed.
2. Validar idempotencia por videoId/jobId.
3. Atualizar status para queued/transcoding.
4. Baixar ou streamar objeto original do MinIO/S3 para workspace temporario.
5. Rodar ffprobe e salvar media-info.json.
6. Gerar plano de renditions.
7. Executar FFmpeg por rendition.
8. Empacotar HLS/DASH/CMAF.
9. Medir metricas basicas e, quando habilitado, VMAF/SSIM/PSNR.
10. Subir saidas para transcoded/{videoId}/.
11. Subir metricas para metrics/{videoId}/.
12. Publicar video.transcode.completed e video.ready.
13. Em falha recuperavel, retry; em falha final, publicar video.transcode.failed.
```

### 8.2 Perfil De Producao Inicial

O benchmark do `SPEC.md` compara 5 codecs. Para producao inicial, usar um perfil mais simples:

| Perfil | Codec | Renditions | Saida | Motivo |
| --- | --- | --- | --- | --- |
| `production-h264-hls-dash` | H.264/AAC | 1080p, 720p | HLS + DASH/CMAF | Compatibilidade ampla e menor risco operacional. |

Ladder inicial:

| Rendition | Resolucao | Bitrate alvo |
| --- | ---: | ---: |
| 1080p | 1920x1080 | 4-8 Mbps |
| 720p | 1280x720 | 2-4 Mbps |

4K, H.265, VP9, AV1 e H.266 entram primeiro no modo benchmark antes de virarem perfil de entrega.

### 8.3 Perfil De Benchmark

O modo benchmark deve executar a matriz definida no `SPEC.md`:

```text
30 videos x 5 codecs x 3 resolucoes x 3 niveis de qualidade = 1.350 jobs
```

Esse modo nao deve bloquear o pipeline de upload do usuario. Deve rodar como fila separada:

```text
queue: transcode.benchmark.jobs
routing key: video.benchmark.requested
```

## 9. Requisitos De Infra Para Transcode

### 9.1 Local

Adicionar ao `infra/docker-compose.yml` um servico futuro:

```text
streaming-transcode
  build: ../streaming-transcode
  depends_on:
    rabbitmq
    minio
    event-gateway
  environment:
    RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
    STORAGE_PROVIDER=minio
    STORAGE_BUCKET=videos
    MINIO_ENDPOINT=http://minio:9000
    MINIO_ACCESS_KEY=admin
    MINIO_SECRET_KEY=password123
    EVENT_GATEWAY_URL=http://event-gateway:8080/api/v1
    TRANSCODE_WORKDIR=/tmp/transcode
```

Imagem base deve conter:

- FFmpeg com libx264, libx265, libvpx-vp9 e libsvtav1 quando possivel.
- ffprobe.
- Shaka Packager ou alternativa FFmpeg para HLS/DASH.
- libvmaf para modo benchmark.
- VVenC apenas no perfil de pesquisa H.266.

### 9.2 AWS

O Terraform atual cobre S3 e IAM para storage. Para transcoding em cloud faltam:

- EC2/ECS/Fargate ou AWS Batch para workers CPU.
- Opcional: EC2 G6/L4 para GPU.
- RabbitMQ gerenciado ou broker compativel.
- MongoDB gerenciado ou Atlas.
- IAM role para worker com `GetObject`, `PutObject`, `ListBucket`, `AbortMultipartUpload`.
- Logs e metricas por job.

## 10. Idempotencia, Retry E DLQ

O transcoding deve ser idempotente por:

```text
videoId + profile + sourceKey + sourceETag/version
```

Regras:

- Se `transcoded/{videoId}/hls/master.m3u8` e `metrics/{videoId}/transcode-result.json` ja existem para o mesmo perfil, publicar `video.ready` sem reprocessar.
- Retries devem ocorrer apenas para falhas transitórias de storage, rede ou processo interrompido.
- Falhas de codec invalido, arquivo corrompido ou payload incompleto devem ir para DLQ.
- Cada tentativa deve atualizar `transcode.attempt`.

Filas recomendadas:

```text
transcode.jobs
transcode.retry
transcode.dead
transcode.benchmark.jobs
```

## 11. Integracao Com UI Upload

Mudancas necessarias no `streaming-platform-upload`:

- Remover o comportamento que marca `ready` por timer fixo apos 2 segundos quando transcode real estiver habilitado.
- Encaminhar/consumir eventos com routing key `video.transcode.*` e `video.ready`.
- Mostrar estados separados: `uploading`, `uploaded`, `queued`, `transcoding`, `packaging`, `ready`, `error`.
- Exibir `hlsManifestPath`/`dashManifestPath` quando disponiveis.
- Manter thumbnail assíncrona atual independente do transcode.

## 12. Mudancas Necessarias No Ingest

Mudancas minimas:

- Atualizar `swagger.yaml` com eventos de transcoding.
- Adicionar campos V2 ao modelo persistido sem quebrar campos atuais.
- Criar endpoint ou aceitar patch existente para atualizacao segura por worker:

```text
PATCH /api/v1/upload-state/videos/{videoId}
```

- Normalizar payload de `upload.completed` vindo de webhook MinIO/S3 para incluir `bucket`, `objectKey`, `provider` e `occurredAt`.
- Documentar routing keys oficiais.

## 13. Criterios De Aceitacao V2

1. Um upload concluido no `streaming-platform-upload` gera evento consumivel pelo worker.
2. O worker consome `video.upload.completed` na exchange `video_events`.
3. O worker executa `ffprobe` e persiste `media-info.json`.
4. O worker gera pelo menos H.264/AAC em 1080p e 720p.
5. O worker gera HLS master playlist e DASH manifest.
6. As saidas sao gravadas em `transcoded/{videoId}/`.
7. O worker publica routing keys `video.transcode.started`, `video.transcode.completed` e `video.ready`.
8. O ingest atualiza o documento do video com manifestos, renditions e metricas.
9. A UI deixa de marcar `ready` por timer e passa a refletir o status real.
10. Falhas geram `video.transcode.failed` com motivo rastreavel.
11. Reprocessamento do mesmo evento nao duplica saidas nem corrompe metadados.
12. O pipeline local roda com `infra/docker-compose.yml`.

## 14. Backlog De Implementacao

### Fase 1 - Contratos E Infra Local

- Definir schema oficial de `upload.completed`.
- Declarar filas `transcode.jobs`, `transcode.retry` e `transcode.dead`.
- Adicionar `streaming-transcode` ao Docker Compose.
- Criar Dockerfile com FFmpeg/ffprobe.
- Atualizar OpenAPI do ingest.

### Fase 2 - Worker MVP

- Criar consumer RabbitMQ.
- Criar storage adapter MinIO/S3.
- Implementar workspace temporario.
- Rodar ffprobe.
- Gerar 1080p e 720p H.264/AAC.
- Gerar HLS/DASH.
- Publicar eventos de status.

### Fase 3 - Persistencia E UI

- Expandir modelo de video no ingest.
- Atualizar `PATCH /upload-state/videos/{videoId}` para campos V2.
- Remover timer falso de `ready` do upload.
- Exibir estados de transcode na UI.
- Exibir manifestos e playback quando pronto.

### Fase 4 - Resiliencia

- Implementar idempotencia.
- Implementar retry/backoff.
- Implementar DLQ.
- Registrar logs estruturados por job.
- Criar testes de contrato para eventos.

### Fase 5 - Benchmark Do SPEC Original

- Implementar modo benchmark separado.
- Executar matriz de codecs do `SPEC.md`.
- Coletar VMAF/SSIM/PSNR.
- Medir RTF, CPU/GPU, memoria, I/O e custo.
- Produzir recomendacao de codec por perfil.

## 15. Decisao Arquitetural

A V2 nao deve substituir `streaming-ingest` nem mover a mensageria para o upload. O caminho correto e:

- `streaming-platform-upload` continua responsavel por experiencia de upload, multipart, storage write inicial e thumbnail.
- `streaming-ingest` continua sendo o gateway oficial de eventos, persistencia de estado e publicador RabbitMQ.
- `infra` continua centralizando dependencias compartilhadas.
- `streaming-transcode` deve nascer como worker isolado, sem API publica obrigatoria, consumindo eventos e escrevendo resultados no storage.

Essa separacao preserva o que ja existe e transforma o plano de transcoding em uma extensao natural do pipeline atual, em vez de criar um segundo fluxo paralelo de ingestao/processamento.
