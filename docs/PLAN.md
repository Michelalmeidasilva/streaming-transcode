# Streaming Transcode - Implementation Plan

## 1. Goal

Implementar o `streaming-transcode` como worker de processamento de video integrado ao pipeline existente de VOD, usando `SPEC-V2.md` como plano de integracao e `SPEC.md` como plano de benchmark tecnico.

O resultado esperado e:

```text
upload concluido
  -> evento video.upload.completed
  -> worker streaming-transcode
  -> ffprobe + FFmpeg
  -> HLS/DASH/CMAF em transcoded/{videoId}/
  -> metricas em metrics/{videoId}/
  -> eventos video.transcode.* e video.ready
  -> metadata atualizada no streaming-ingest
```

## 2. Principles

- O transcode nao deve receber upload por HTTP.
- O transcode consome eventos do RabbitMQ e le o arquivo original do MinIO/S3.
- O `streaming-ingest` continua sendo o gateway oficial de eventos e persistencia.
- O `streaming-platform-upload` continua responsavel por UX de upload, multipart e thumbnail.
- O primeiro perfil de producao sera H.264/AAC em 1080p e 720p com HLS/DASH.
- H.265, VP9, AV1, H.266, VMAF e analise de custo entram depois, no modo benchmark.

## 3. Delivery Phases

### Phase 1 - Contracts And Local Runtime

Objetivo: deixar a base executavel localmente e estabilizar os contratos antes de implementar FFmpeg.

Tasks:

1. Criar estrutura inicial do projeto `streaming-transcode`.
2. Escolher stack do worker. Recomendacao: Go, para alinhar com `streaming-ingest`, ou Node.js se a prioridade for reaproveitar SDKs e velocidade de prototipacao.
3. Criar configuracao por variaveis de ambiente:
   ```text
   RABBITMQ_URL
   STORAGE_PROVIDER
   STORAGE_BUCKET
   MINIO_ENDPOINT
   MINIO_ACCESS_KEY
   MINIO_SECRET_KEY
   AWS_REGION
   EVENT_GATEWAY_URL
   TRANSCODE_WORKDIR
   TRANSCODE_PROFILE
   ```
4. Criar Dockerfile com `ffmpeg` e `ffprobe`.
5. Adicionar `streaming-transcode` ao `../infra/docker-compose.yml`.
6. Declarar exchange/bindings no startup do worker:
   ```text
   exchange: video_events
   queue: transcode.jobs
   binding: video.upload.completed
   ```
7. Definir contrato interno para mensagem de entrada:
   ```json
   {
     "eventType": "upload.completed",
     "videoId": "uuid",
     "filename": "uuid/original.mp4",
     "objectKey": "uuid/original.mp4",
     "provider": "minio",
     "bucket": "videos",
     "size": 1048576000
   }
   ```
8. Implementar fallback para payloads atuais que contenham apenas `videoId`, `filename`, `provider` e `size`.
9. Atualizar `streaming-ingest/swagger.yaml` com eventos de transcode e schema de payload.

Acceptance criteria:

- `docker compose` sobe o worker junto com MinIO, RabbitMQ e Event Gateway.
- O worker conecta no RabbitMQ e declara a fila `transcode.jobs`.
- Um evento manual em `video.upload.completed` e recebido e logado com `videoId` e `objectKey`.
- Payload incompleto e rejeitado com log claro, sem crashar o processo.

## 4. Phase 2 - Worker MVP

Objetivo: processar um video real do storage local e gravar saidas HLS/DASH.

Tasks:

1. Implementar storage adapter MinIO/S3 com operacoes:
   ```text
   downloadObject(bucket, key, destination)
   uploadFile(bucket, key, source, contentType)
   objectExists(bucket, key)
   listPrefix(bucket, prefix)
   ```
2. Implementar workspace temporario por job:
   ```text
   {TRANSCODE_WORKDIR}/{jobId}/source/
   {TRANSCODE_WORKDIR}/{jobId}/work/
   {TRANSCODE_WORKDIR}/{jobId}/output/
   ```
3. Baixar objeto original usando `objectKey` ou fallback `filename`.
4. Executar `ffprobe` e gerar `media-info.json`.
5. Extrair metadados minimos:
   ```text
   durationSeconds
   width
   height
   fps
   videoCodec
   audioCodec
   bitrateKbps
   ```
6. Gerar plano de renditions inicial:
   ```text
   1080p se source >= 1080p
   720p se source >= 720p
   preservar FPS se source_fps <= 30
   downsample para 30 fps somente em fase posterior
   ```
7. Executar FFmpeg H.264/AAC:
   ```text
   1080p: libx264, yuv420p, 4-8 Mbps, AAC
   720p: libx264, yuv420p, 2-4 Mbps, AAC
   ```
8. Gerar HLS com master playlist:
   ```text
   transcoded/{videoId}/hls/master.m3u8
   transcoded/{videoId}/hls/1080p/index.m3u8
   transcoded/{videoId}/hls/720p/index.m3u8
   ```
9. Gerar DASH manifest:
   ```text
   transcoded/{videoId}/dash/manifest.mpd
   ```
10. Subir saidas para o bucket.
11. Subir `metrics/{videoId}/media-info.json`.
12. Publicar status via `POST /api/v1/events` usando event types sem prefixo `video.`:
   ```text
   transcode.started
   transcode.progress
   packaging.completed
   transcode.completed
   ready
   ```

Acceptance criteria:

- Um arquivo `sample.mp4` ou upload real gera HLS e DASH no MinIO.
- `media-info.json` e salvo em `metrics/{videoId}/media-info.json`.
- O worker publica `video.transcode.started`, `video.transcode.completed` e `video.ready` via ingest.
- O processo limpa workspace temporario ao final.
- Falha de FFmpeg gera `transcode.failed`.

## 5. Phase 3 - Ingest Persistence Integration

Objetivo: persistir o estado real do transcode no MongoDB atraves do `streaming-ingest`.

Tasks:

1. Expandir modelo de video do ingest para aceitar campos V2:
   ```text
   processingStatus
   source
   mediaInfo
   transcode
   playback
   metrics
   ```
2. Garantir que `PATCH /api/v1/upload-state/videos/{videoId}` aceite patches aninhados sem quebrar campos existentes.
3. Atualizar handlers de eventos para aplicar mudancas de status:
   ```text
   transcode.queued -> processingStatus=queued
   transcode.started -> processingStatus=transcoding
   packaging.completed -> processingStatus=packaging
   transcode.completed -> playback/metrics/transcode preenchidos
   ready -> processingStatus=ready, status=ready
   transcode.failed -> processingStatus=failed, status=error
   ```
4. Normalizar webhook MinIO/S3 para sempre incluir:
   ```text
   bucket
   objectKey
   provider
   occurredAt
   ```
5. Criar testes de repository/handler para persistencia dos novos campos.

Acceptance criteria:

- O documento de video no MongoDB mostra manifestos, renditions, status e metricas basicas.
- O PATCH preserva campos antigos como `thumbnailUrl`, `thumbnailStatus` e `title`.
- Eventos de transcode podem ser reproduzidos sem duplicar documentos.

## 6. Phase 4 - Upload UI Integration

Objetivo: fazer a UI refletir o estado real do pipeline em vez de marcar `ready` por timer.

Tasks:

1. Remover ou condicionar o timer de 2 segundos que chama `updateVideoStatus(..., 'ready')` apos upload.
2. Adicionar estados visuais:
   ```text
   uploading
   uploaded
   queued
   transcoding
   packaging
   ready
   error
   ```
3. Atualizar tipos TypeScript para `playback`, `renditions`, `metrics` e `processingStatus`.
4. Atualizar `IngestUploadStateClient` para mapear campos V2.
5. Exibir HLS/DASH quando `playback.hlsManifestPath` ou `playback.dashManifestPath` existir.
6. Manter thumbnail atual independente do transcode.
7. Adicionar testes de UI para estados de processamento.

Acceptance criteria:

- A UI nao marca video como pronto antes do evento `ready`.
- Usuario ve progresso/status de transcode.
- Video pronto mostra manifestos/renditions ou acao de playback/download adequada.

## 7. Phase 5 - Reliability

Objetivo: tornar o worker seguro para reprocessamento, falhas e operacao continua.

Tasks:

1. Implementar idempotencia por:
   ```text
   videoId + profile + sourceKey + sourceETag/version
   ```
2. Antes de processar, verificar se ja existem:
   ```text
   transcoded/{videoId}/hls/master.m3u8
   metrics/{videoId}/transcode-result.json
   ```
3. Implementar retry/backoff para erros transitorios:
   ```text
   storage timeout
   rede
   RabbitMQ temporarily unavailable
   processo FFmpeg interrompido
   ```
4. Implementar DLQ para falhas finais:
   ```text
   payload invalido
   objeto nao encontrado
   video corrompido
   codec nao suportado no perfil atual
   ```
5. Criar filas:
   ```text
   transcode.jobs
   transcode.retry
   transcode.dead
   ```
6. Registrar logs JSON por job:
   ```text
   jobId
   videoId
   profile
   sourceKey
   status
   elapsedMs
   errorType
   ```
7. Criar metricas operacionais basicas:
   ```text
   jobs_completed
   jobs_failed
   avg_elapsed_seconds
   avg_rtf
   output_size_mb
   ```

Acceptance criteria:

- Reentregar o mesmo evento nao duplica saidas.
- Falhas transitorias sao retentadas.
- Falhas definitivas vao para DLQ com motivo.
- Logs permitem rastrear um job fim-a-fim.

## 8. Phase 6 - Production Profile Hardening

Objetivo: melhorar qualidade e performance do perfil H.264 inicial.

Tasks:

1. Calibrar ladder H.264:
   ```text
   1080p: 4, 6, 8 Mbps
   720p: 2, 3, 4 Mbps
   ```
2. Definir presets FFmpeg:
   ```text
   local/dev: veryfast
   production baseline: medium
   benchmark: slow ou presets especificos
   ```
3. Implementar regra de FPS:
   ```text
   source_fps <= 30: preservar
   source_fps > 30: preservar inicialmente; testar downsample no benchmark
   nunca interpolar para cima
   ```
4. Validar playback HLS/DASH em pelo menos um player.
5. Medir tempo, tamanho de saida, bitrate final e RTF.
6. Gerar `metrics/{videoId}/transcode-result.json`.

Acceptance criteria:

- HLS e DASH reproduzem sem erro.
- Audio e video permanecem sincronizados.
- Metricas minimas estao disponiveis por job.
- Perfil H.264 inicial fica documentado como default operacional.

## 9. Phase 7 - Benchmark Mode

Objetivo: implementar o estudo comparativo previsto no `SPEC.md` sem bloquear o pipeline de producao.

Tasks:

1. Criar fila separada:
   ```text
   queue: transcode.benchmark.jobs
   routing key: video.benchmark.requested
   ```
2. Criar catalogo dos 30 videos comerciais:
   ```text
   6 beleza
   6 medicamentos
   6 perfume
   6 educativo de farmacia
   6 sazonais
   ```
3. Registrar metadados obrigatorios:
   ```text
   categoria
   subcategoria
   duracao
   resolucao
   fps
   codec origem
   bitrate origem
   audio codec/sample rate/canais
   cortes por minuto
   motion_score
   detail_score
   texto pequeno
   pele/rosto
   produto/embalagem
   complexidade
   ```
4. Gerar matriz de jobs:
   ```text
   30 videos x 5 codecs x 3 resolucoes x 3 niveis de qualidade = 1.350 jobs
   ```
5. Implementar codecs de benchmark:
   ```text
   H.264: libx264, h264_nvenc
   H.265: libx265, hevc_nvenc
   VP9: libvpx-vp9
   AV1: libsvtav1, libaom-av1, av1_nvenc quando disponivel
   H.266: VVenC
   ```
6. Coletar metricas de qualidade:
   ```text
   VMAF
   SSIM
   PSNR
   bitrate final
   tamanho final
   resolucao final
   FPS final
   ```
7. Coletar metricas de desempenho:
   ```text
   elapsed_s
   rtf
   encoding_fps
   CPU media/max
   GPU media/encoder
   RAM media/pico
   disk read/write
   retries/failures
   ```
8. Coletar custo:
   ```text
   local energia + amortizacao
   EC2 tempo_horas x preco_hora + EBS + S3 + transferencia + logs
   ```
9. Persistir resultados em:
   ```text
   metrics/benchmark/{runId}/results.jsonl
   metrics/benchmark/{runId}/results.csv
   ```
10. Gerar ranking por codec, maquina, qualidade, custo e compatibilidade.

Acceptance criteria:

- Benchmark roda isolado do fluxo de upload normal.
- Uma execucao parcial pode ser retomada.
- Resultados seguem o schema do `SPEC.md`.
- Relatorio final recomenda codec por cenario.

## 10. Test Strategy

Unit tests:

- Parser de evento `upload.completed`.
- Normalizacao de `objectKey`.
- Plano de renditions.
- Parser de `ffprobe`.
- Geracao de payloads `transcode.*`.
- Idempotencia.

Integration tests:

- RabbitMQ consumer recebe evento e chama handler.
- MinIO adapter baixa e sobe objetos.
- Worker processa `sample.mp4` e gera outputs esperados.
- Event Gateway recebe eventos publicados pelo worker.
- Ingest persiste campos V2.

End-to-end local:

```text
1. Subir infra.
2. Subir upload app.
3. Fazer upload de video.
4. Confirmar evento video.upload.completed.
5. Confirmar worker processando.
6. Confirmar arquivos em transcoded/{videoId}/.
7. Confirmar video ready na UI.
```

## 11. Implementation Order

Ordem recomendada para reduzir risco:

1. Criar worker minimo sem FFmpeg, apenas consumidor e logs.
2. Adicionar Dockerfile e compose.
3. Adicionar MinIO download/upload.
4. Adicionar ffprobe.
5. Adicionar FFmpeg H.264 720p primeiro.
6. Adicionar 1080p.
7. Adicionar HLS.
8. Adicionar DASH.
9. Adicionar eventos de status.
10. Persistir campos V2 no ingest.
11. Atualizar UI para status real.
12. Adicionar idempotencia/retry/DLQ.
13. Implementar benchmark separado.

## 12. Definition Of Done

O MVP integrado esta pronto quando:

- Um upload real dispara o worker via RabbitMQ.
- O worker gera HLS/DASH H.264/AAC em `transcoded/{videoId}/`.
- O worker salva `media-info.json` e `transcode-result.json`.
- O ingest registra status, manifestos e metricas.
- A UI mostra `ready` apenas depois do evento real.
- Reprocessar o mesmo evento nao duplica trabalho.
- Falhas sao visiveis e rastreaveis.

O projeto completo esta pronto quando:

- O modo benchmark executa a matriz de codecs do `SPEC.md`.
- Ha comparacao de qualidade, tempo, custo e compatibilidade.
- Existe recomendacao documentada para producao local, EC2 CPU e EC2 GPU.
