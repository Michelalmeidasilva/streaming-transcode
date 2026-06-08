# Publicação de eventos best-effort (não falha o job)

## Sintoma

Todos os jobs do Batch (`vod-prod-transcode`) reportavam `FAILED` (exit code 1) — inclusive
vídeos cuja saída transcodada estava **completa no S3 e servindo no catálogo** (reproduzíveis).
O log do job mostrava:

```
transcode batch job started key=raw/<id>/...
queued/packaging/completed state publish failed: POST /events returned 500
transcode batch job failed: POST /events returned 500 Internal Server Error
```

## Causa-raiz

O worker fala com o **Event Gateway** (`streaming-ingest`) por dois canais:

- `PATCH /api/v1/upload-state/videos/:id` → escreve no **Mongo** (estado `ready`, `mediaInfo`,
  `playback`). É isso que faz o vídeo aparecer no catálogo do `streaming-distribution`.
- `POST /api/v1/events` → o gateway **publica no RabbitMQ** (eventos de UI/analytics).

No incidente, o `POST /events` retornava **500** (falha de publish no RabbitMQ/CloudAMQP da
Lambda de ingest), mas o `PATCH /videos` (Mongo) **funcionava**. Em `complete()`
(`internal/worker/processor.go`), porém, o evento `ready` (`markReady` → `POST /events`) era
**fatal**:

```go
if err := p.events.PatchVideo(ctx, result.VideoID, patch); err != nil { return err } // ok (Mongo)
return p.markReady(ctx, result.VideoID)  // POST /events → 500 → erro propaga → exit 1
```

Resultado: o vídeo ficava `ready` no catálogo (PATCH ok), mas o job saía com erro → Batch
marcava `FAILED`. Falso-negativo: status enganoso, observabilidade quebrada e possíveis
retries de Spot desperdiçando custo. O mesmo acontecia no atalho "already transcoded".

## Correção

Os **eventos** de ciclo de vida passam a ser best-effort; o sucesso do job é decidido pela
produção da mídia + o `PATCH ready` (estado real do catálogo):

```go
if err := p.events.PatchVideo(ctx, result.VideoID, patch); err != nil { return err }
if err := p.markReady(ctx, result.VideoID); err != nil {
    p.logger.Printf("ready event publish failed (non-fatal, video already marked ready): %v", err)
}
return nil
```

O atalho "already transcoded" também passou a logar (em vez de retornar) o erro do evento.
Os demais publishes do pipeline (`queued`/`started`/`packaging`/`progress`) já eram
best-effort. O `PATCH ready` continua fatal — se ele falha, o vídeo realmente não fica
pronto e o job **deve** falhar/retry.

## Teste

`internal/worker/processor_test.go::TestProcessorSucceedsWhenEventPublishFails`: servidor de
teste devolve **500 em `POST /events`** e **200 em `PATCH /videos`**; `Process()` deve
retornar `nil`, ter enviado o PATCH `ready` e subido a mídia.

## Observação (causa do 500 a montante)

O `POST /events` retornar 500 é uma fragilidade do **ingest** (Lambda mantendo conexão AMQP
com CloudAMQP; publish falha quando a conexão está velha/derrubada entre invocações). Esta
mudança torna o transcode resiliente a isso, mas o ideal é também endurecer o publish no
ingest (reconnect/retry) — fora do escopo deste fix.
