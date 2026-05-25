# Validacao Docker Observabilidade AV1

Data: 2026-05-25

## Objetivo

Validar o transcoding AV1 e a coleta de observabilidade de CPU em ambiente Linux via Docker Compose.

## Arquivos adicionados/ajustados

- `Dockerfile`
- `Dockerfile.transcode-local`
- `compose.yaml`
- `.dockerignore`
- `README.md`

## Binario Linux gerado

Comando:

```bash
mkdir -p dist && env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOCACHE=/private/tmp/go-build go build -o dist/transcode-local-linux-amd64 ./cmd/transcode-local
```

Arquivo:

- `dist/transcode-local-linux-amd64`

## Execucao Docker

Build:

```bash
docker compose -f compose.yaml build transcode-local-av1
```

Run:

```bash
docker compose -f compose.yaml up --abort-on-container-exit --remove-orphans transcode-local-av1
```

Cleanup:

```bash
docker compose -f compose.yaml down
```

## Entrada usada

- `outputs/reference-pipeline-preselecao-y4m/comercial-cortes/847661749-Major-Fade-Active-Seal-Moisturizer-chicwithkels/reference.y4m`

## Saida gerada

- `outputs/docker-validation/847661749-av1-720p.mp4`

## Resultado da observabilidade no container Linux

Log final:

```text
observability supported=true samples=391 elapsed=197.392s rtf=26.118 avgCpu=195.40% maxCpu=831.62% outputSize=823439 outputBitrate=871kbps error=""
```

Interpretacao:

- `supported=true`: coleta de CPU/memoria funcionou no Linux via `/proc`
- `samples=391`: observabilidade coletou amostras durante todo o encode
- `avgCpu=195.40%`: uso medio aproximado de quase 2 vCPUs
- `maxCpu=831.62%`: pico observado durante o encode

## Validacao final com ffprobe

```json
{
  "streams": [
    {
      "codec_name": "av1",
      "width": 1280,
      "height": 720,
      "pix_fmt": "yuv420p",
      "avg_frame_rate": "60000/1001"
    }
  ],
  "format": {
    "duration": "7.557550",
    "size": "823439",
    "bit_rate": "871646"
  }
}
```

## SHA256

```text
955e6779d1a2882551b3be95eb2a079664c8498290d9edd90d882774cd889db5  outputs/docker-validation/847661749-av1-720p.mp4
```

## Conclusao

Validacao concluida com sucesso:

- o `compose.yaml` sobe corretamente
- o transcode AV1 roda dentro do container Linux
- a observabilidade de CPU funciona no Docker/Linux
- o artefato final foi validado com `ffprobe`

Atualizacao posterior:

- o `compose.yaml` foi simplificado para um unico servico generico, `transcode-local`
- codec, entrada, saida, resolucao e bitrate agora sao passados por variaveis de ambiente, no mesmo formato que um job cloud usaria
