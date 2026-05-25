# Streaming Transcode - Benchmark Implementation Plan

Date: 2026-05-09

Source: `SPEC.md`

## 1. Objetivo

Implantar um modo de benchmark separado do fluxo produtivo de transcode para comparar codecs, resoluções, presets, qualidade objetiva, tempo de processamento, uso de recursos e custo.

O benchmark deve responder:

- quanto tempo leva para processar o dataset de 30 vídeos comerciais
- qual codec entrega melhor qualidade por bitrate
- qual codec entrega melhor custo por minuto processado
- qual máquina entrega melhor throughput por custo
- quais configurações podem virar candidatas de produção

## 2. Premissas de Implantação

- O fluxo produtivo atual continua isolado em `video.upload.completed` e `transcode.jobs`.
- O benchmark usa fluxo próprio, sem afetar uploads reais.
- A primeira entrega deve funcionar localmente com FFmpeg.
- GPU, EC2 e H.266/VVC entram depois da matriz local CPU estar estável.
- Preços de EC2 da `SPEC.md` são referência metodológica e devem ser recalculados no momento da execução, na região final.

## 3. Estado Atual Aproveitável

Já existe no `streaming-transcode`:

- worker RabbitMQ para eventos de produção
- download/upload em MinIO/S3
- `ffprobe` para metadados
- runner FFmpeg
- geração HLS/DASH
- métricas básicas de tempo e RTF
- testes unitários acima de 80%

O benchmark deve reaproveitar:

- `internal/transcode/ffprobe.go`
- `internal/transcode/runner.go`
- `internal/storage/minio.go`
- `internal/config/config.go`
- contratos de resultado existentes em `internal/domain/types.go`

O benchmark deve adicionar novos módulos sem acoplar o worker produtivo.

## 4. Arquitetura Alvo

```text
Dataset manifest
      ↓
Benchmark planner
      ↓
Job matrix
      ↓
Benchmark runner local ou fila dedicada
      ↓
FFmpeg / encoder profile
      ↓
Resource sampler
      ↓
Quality analyzer
      ↓
Result writer
      ↓
Reports CSV/JSON/Markdown
```

Componentes novos sugeridos:

```text
cmd/benchmark
internal/benchmark/catalog
internal/benchmark/planner
internal/benchmark/profiles
internal/benchmark/runner
internal/benchmark/metrics
internal/benchmark/quality
internal/benchmark/report
```

Fila dedicada futura:

```text
Exchange: video_events
Queue: transcode.benchmark.jobs
Routing key: video.benchmark.requested
```

## 5. Modelo de Dados

## 5.1 Dataset Manifest

Arquivo sugerido:

```text
benchmark/dataset/videos.json
```

Schema inicial:

```json
{
  "datasetId": "commercials-30-v1",
  "videos": [
    {
      "videoId": "beauty-low-001",
      "category": "beauty",
      "subcategory": "skin-care",
      "complexity": "low",
      "sourcePath": "benchmark/sources/beauty-low-001.mp4",
      "storageBucket": "videos",
      "storageKey": "benchmark/sources/beauty-low-001.mp4",
      "hasSmallText": true,
      "hasFaceOrSkin": true,
      "hasProductPackage": true,
      "notes": "close-up product and face"
    }
  ]
}
```

Campos derivados por `ffprobe`:

```text
duration_s
source_resolution
source_fps
source_codec
source_bitrate_kbps
audio_codec
audio_sample_rate
audio_channels
```

Campos manuais ou semiautomáticos:

```text
cuts_per_minute
motion_score
texture_score
complexity
has_small_text
has_face_or_skin
has_product_package
```

## 5.2 Benchmark Job

Arquivo gerado:

```text
benchmark/runs/{runId}/jobs.jsonl
```

Um job por linha:

```json
{
  "runId": "local-cpu-20260509-001",
  "jobId": "beauty-low-001-h264-1080p-crf23-local",
  "videoId": "beauty-low-001",
  "sourceKey": "benchmark/sources/beauty-low-001.mp4",
  "codec": "h264",
  "encoder": "libx264",
  "targetResolution": "1080p",
  "width": 1920,
  "height": 1080,
  "targetFps": 30,
  "mode": "quality",
  "crfOrCq": "23",
  "bitrateTargetKbps": null,
  "preset": "medium",
  "machineProfile": "local-cpu",
  "repeat": 1
}
```

## 5.3 Benchmark Result

Arquivo gerado:

```text
benchmark/runs/{runId}/results.jsonl
benchmark/runs/{runId}/results.csv
```

Campos obrigatórios:

```text
run_id
job_id
video_id
categoria
complexidade
source_duration_s
source_resolution
source_fps
target_resolution
target_fps
codec
encoder
preset
bitrate_target_kbps
crf_or_cq
machine_type
cpu_model
gpu_model
parallel_jobs
elapsed_s
rtf
encoding_fps
avg_cpu_pct
max_cpu_pct
avg_gpu_pct
gpu_encoder_pct
avg_ram_gb
disk_read_mb_s
disk_write_mb_s
output_size_mb
output_bitrate_kbps
vmaf
ssim
psnr
cost_usd
status
error_message
```

## 6. Matriz Inicial de Benchmark

## 6.1 Codecs

| Codec | Encoder CPU | Encoder GPU | Observação |
| --- | --- | --- | --- |
| H.264 | `libx264` | `h264_nvenc` | baseline de compatibilidade |
| H.265 | `libx265` | `hevc_nvenc` | alta compressão atual |
| VP9 | `libvpx-vp9` | não priorizar | web aberto |
| AV1 | `libsvtav1` ou `libaom-av1` | `av1_nvenc` quando disponível | moderno e eficiente |
| H.266/VVC | VVenC | não priorizar | pesquisa, não produção inicial |

## 6.2 Resoluções

```text
4K: 3840x2160
1080p: 1920x1080
720p: 1280x720
```

## 6.3 Modos de Qualidade

Rodada técnica completa:

```text
30 vídeos × 5 codecs × 3 resoluções × 3 níveis = 1.350 jobs
```

Níveis iniciais:

```text
quality-high
quality-medium
quality-low
```

Cada perfil deve mapear para CRF/CQ ou bitrate conforme o codec.

Exemplo inicial:

| Codec | High | Medium | Low |
| --- | --- | --- | --- |
| H.264/libx264 | CRF 18 | CRF 23 | CRF 28 |
| H.265/libx265 | CRF 20 | CRF 26 | CRF 30 |
| VP9/libvpx-vp9 | CRF 24 | CRF 32 | CRF 38 |
| AV1/libsvtav1 | CRF 24 | CRF 32 | CRF 40 |
| H.266/VVenC | QP baixo | QP médio | QP alto |

Os valores devem ser calibrados após a primeira amostra de vídeos.

## 7. Fases de Implantação

## Fase 0 - Preparação e Decisões de Escopo

Objetivo:

- fechar a primeira matriz local viável antes de cloud/GPU

Tarefas:

- confirmar se os 30 vídeos comerciais estão disponíveis e podem ser usados no benchmark
- definir onde os arquivos brutos ficarão: filesystem local ou MinIO
- escolher primeira máquina local de referência
- registrar dados de máquina local:
  - CPU
  - GPU
  - RAM
  - storage
  - potência média estimada
  - tarifa de energia
- definir se H.266/VVC entra na primeira matriz ou fica em rodada separada

Entregáveis:

- `benchmark/dataset/videos.json`
- `benchmark/machines/local.json`
- decisão documentada de matriz inicial

Critério de aceite:

- dataset e máquina local estão catalogados antes de gerar jobs

## Fase 1 - Catálogo do Dataset

Objetivo:

- transformar os 30 vídeos em um catálogo versionado

Tarefas:

- criar loader de manifest JSON
- rodar `ffprobe` em todos os vídeos
- persistir metadados derivados
- adicionar campos manuais de categoria e complexidade
- calcular FPS recomendado conforme regra da SPEC
- validar se todos os vídeos têm áudio/vídeo legíveis

Arquivos novos esperados:

```text
internal/benchmark/catalog
benchmark/dataset/videos.json
benchmark/dataset/catalog.generated.json
```

Critério de aceite:

- comando de catálogo gera metadados para 30/30 vídeos
- falhas de leitura são registradas com motivo

Comando alvo:

```bash
go run ./cmd/benchmark catalog \
  --dataset benchmark/dataset/videos.json \
  --out benchmark/dataset/catalog.generated.json
```

## Fase 2 - Planner da Matriz de Jobs

Objetivo:

- gerar a matriz `vídeo × codec × resolução × qualidade × máquina`

Tarefas:

- criar abstração de codec profile
- criar profiles de H.264, H.265, VP9 e AV1 CPU
- preparar H.266/VVC como profile experimental separado
- gerar jobs JSONL
- permitir filtros:
  - codec
  - resolução
  - máquina
  - complexidade
  - limite de vídeos
  - repetição
- suportar dry-run com contagem de jobs

Arquivos novos esperados:

```text
internal/benchmark/planner
internal/benchmark/profiles
benchmark/runs/{runId}/jobs.jsonl
```

Critério de aceite:

- planner gera 1.350 jobs para a matriz completa
- planner permite gerar uma matriz smoke com poucos jobs

Comandos alvo:

```bash
go run ./cmd/benchmark plan \
  --catalog benchmark/dataset/catalog.generated.json \
  --machine local-cpu \
  --out benchmark/runs/local-smoke/jobs.jsonl \
  --limit-videos 2 \
  --codecs h264,h265 \
  --resolutions 1080p,720p
```

```bash
go run ./cmd/benchmark plan \
  --catalog benchmark/dataset/catalog.generated.json \
  --machine local-cpu \
  --out benchmark/runs/local-full/jobs.jsonl
```

## Fase 3 - Runner Local de Benchmark

Objetivo:

- executar jobs localmente de forma reproduzível

Tarefas:

- criar runner que lê `jobs.jsonl`
- executar FFmpeg com profile do job
- gerar output em diretório isolado por job
- medir tempo wall-clock e RTF
- capturar stdout/stderr do FFmpeg
- extrair bitrate/tamanho final via `ffprobe` e filesystem
- suportar paralelismo controlado
- suportar resume de jobs já concluídos

Arquivos novos esperados:

```text
internal/benchmark/runner
benchmark/runs/{runId}/outputs/{jobId}/...
benchmark/runs/{runId}/logs/{jobId}.log
benchmark/runs/{runId}/results.jsonl
```

Critério de aceite:

- smoke benchmark executa pelo menos 2 vídeos × 2 codecs × 2 resoluções
- resultados são gravados mesmo em caso de falha
- jobs podem ser retomados sem repetir os concluídos

Comando alvo:

```bash
go run ./cmd/benchmark run \
  --jobs benchmark/runs/local-smoke/jobs.jsonl \
  --out benchmark/runs/local-smoke \
  --parallel 1
```

## Fase 4 - Resource Sampler

Objetivo:

- medir CPU, memória, disco e GPU durante cada job

Tarefas:

- coletar CPU e memória do processo FFmpeg
- coletar I/O quando disponível no sistema operacional
- detectar GPU NVIDIA com `nvidia-smi`
- coletar GPU utilization e encoder utilization quando disponível
- persistir amostras por job
- calcular média e pico

Arquivos novos esperados:

```text
internal/benchmark/metrics
benchmark/runs/{runId}/samples/{jobId}.jsonl
```

Critério de aceite:

- resultados incluem CPU/RAM para jobs locais
- em máquinas sem GPU, campos GPU ficam nulos ou zero sem quebrar o runner
- em máquinas NVIDIA, `nvidia-smi` alimenta campos GPU

## Fase 5 - Métricas de Qualidade

Objetivo:

- calcular VMAF, SSIM e PSNR para comparar original versus output

Tarefas:

- adicionar etapa opcional de qualidade pós-transcode
- validar presença de FFmpeg com `libvmaf`
- normalizar resolução/fps para comparação full-reference
- calcular:
  - VMAF
  - SSIM
  - PSNR
- persistir logs brutos de qualidade
- não bloquear smoke tests quando `libvmaf` não estiver instalado

Arquivos novos esperados:

```text
internal/benchmark/quality
benchmark/runs/{runId}/quality/{jobId}.json
```

Critério de aceite:

- VMAF/SSIM/PSNR são calculados para pelo menos H.264 e H.265 no smoke benchmark
- se `libvmaf` não existir, o job marca qualidade como `skipped_missing_dependency`

Comando alvo:

```bash
go run ./cmd/benchmark quality \
  --results benchmark/runs/local-smoke/results.jsonl \
  --out benchmark/runs/local-smoke
```

## Fase 6 - Validação de Playback e Packaging

Objetivo:

- garantir que os outputs candidatos podem ser empacotados e tocados

Tarefas:

- gerar HLS/DASH para os melhores candidatos por codec/resolução
- validar manifesto HLS e DASH
- executar ffprobe nos segmentos ou arquivos finais
- checar presença de áudio, vídeo, duração e sincronismo básico
- registrar checklist visual manual para cenas críticas

Critério de aceite:

- HLS e DASH geram manifesto válido para candidatos escolhidos
- relatório indica quais codecs são adequados para entrega e quais são apenas benchmark

## Fase 7 - Consolidação e Relatórios

Objetivo:

- transformar resultados brutos em decisão técnica

Tarefas:

- gerar CSV consolidado
- gerar ranking por codec
- gerar ranking por máquina
- gerar custo por 30 vídeos
- gerar custo por minuto processado
- aplicar pesos de decisão da SPEC:
  - qualidade perceptual: 30%
  - tempo: 20%
  - custo: 20%
  - compatibilidade: 15%
  - complexidade operacional: 10%
  - risco licenciamento/suporte: 5%
- gerar relatório Markdown executivo

Arquivos novos esperados:

```text
internal/benchmark/report
benchmark/runs/{runId}/results.csv
benchmark/runs/{runId}/summary.md
benchmark/runs/{runId}/recommendations.md
```

Critério de aceite:

- relatório final recomenda codec por cenário:
  - compatibilidade máxima
  - melhor qualidade/custo
  - melhor throughput/custo
  - web aberto
  - pesquisa/futuro

Comando alvo:

```bash
go run ./cmd/benchmark report \
  --results benchmark/runs/local-full/results.jsonl \
  --out benchmark/runs/local-full
```

## Fase 8 - Fila Dedicada de Benchmark

Objetivo:

- permitir benchmark distribuído sem usar a fila produtiva

Tarefas:

- criar evento `benchmark.requested`
- declarar fila `transcode.benchmark.jobs`
- criar worker ou modo de worker específico para benchmark
- publicar status de benchmark separado de status produtivo
- persistir resultados por `runId`
- impedir que benchmark altere `upload-state` de vídeos produtivos

Critério de aceite:

- job de benchmark pode ser criado por evento
- worker benchmark consome apenas `transcode.benchmark.jobs`
- resultados são gravados sem publicar `ready` produtivo

## Fase 9 - EC2 CPU/GPU

Objetivo:

- executar a mesma matriz em máquinas candidatas de cloud

Tarefas:

- criar `benchmark/machines/*.json` para cada perfil EC2
- automatizar setup de dependências:
  - FFmpeg com codecs necessários
  - libvmaf
  - drivers NVIDIA quando GPU
  - `nvidia-smi`
- executar matriz reduzida em:
  - CPU média
  - CPU alta
  - GPU L4 básica
  - GPU L4 maior
- recalcular preços por região no momento da execução
- registrar custo por job e por rodada

Critério de aceite:

- relatório compara local, EC2 CPU e EC2 GPU
- custo inclui tempo de instância, storage, transferência estimada e logs
- GPU reporta throughput e qualidade separados de software encoding

## Fase 10 - Rodada Completa e Decisão

Objetivo:

- executar a rodada final para suportar decisão de produção

Tarefas:

- rodar matriz técnica completa ou amostra estatisticamente suficiente
- repetir testes críticos 3 vezes
- usar mediana como valor oficial
- revisar visualmente cenas críticas
- produzir relatório executivo
- abrir backlog de produção com codecs escolhidos

Critério de aceite:

- decisão final não depende de opinião isolada
- recomendação inclui qualidade, custo, tempo, compatibilidade e risco operacional

## 8. Estratégia de Testes

Testes unitários:

- parser de dataset manifest
- geração de job matrix
- profiles de encoder
- cálculo de FPS recomendado
- cálculo de RTF
- cálculo de custo local/EC2
- parser de resultados FFmpeg/ffprobe
- agregador de relatório

Testes de integração locais:

- smoke benchmark com 1 vídeo pequeno
- quality step com dependências disponíveis
- geração de CSV/Markdown
- resume de execução parcialmente concluída

Testes que não devem rodar no CI padrão:

- matriz completa de 1.350 jobs
- VMAF pesado
- GPU
- H.266/VVC
- EC2

CI recomendado:

```bash
go test ./...
go test ./internal/benchmark/... -run Test
```

Comando manual de smoke:

```bash
go run ./cmd/benchmark catalog --dataset benchmark/dataset/videos.json --out benchmark/dataset/catalog.generated.json
go run ./cmd/benchmark plan --catalog benchmark/dataset/catalog.generated.json --machine local-cpu --out benchmark/runs/smoke/jobs.jsonl --limit-videos 1 --codecs h264 --resolutions 720p
go run ./cmd/benchmark run --jobs benchmark/runs/smoke/jobs.jsonl --out benchmark/runs/smoke --parallel 1
go run ./cmd/benchmark report --results benchmark/runs/smoke/results.jsonl --out benchmark/runs/smoke
```

## 9. Ordem Recomendada de Execução

1. Criar dataset manifest e catalogação.
2. Criar planner da matriz.
3. Criar runner local para H.264/H.265.
4. Adicionar VP9 e AV1 CPU.
5. Adicionar coleta de recursos.
6. Adicionar qualidade objetiva.
7. Adicionar relatórios.
8. Rodar smoke benchmark.
9. Rodar benchmark local completo.
10. Adicionar fila dedicada.
11. Rodar EC2 CPU.
12. Rodar EC2 GPU.
13. Adicionar H.266/VVC experimental.
14. Produzir recomendação final.

## 10. Riscos e Mitigações

| Risco | Impacto | Mitigação |
| --- | --- | --- |
| `libvmaf` ausente no FFmpeg local | qualidade objetiva incompleta | detectar dependência e marcar etapa como skipped |
| H.266/VVC difícil de empacotar ou instalar | atraso na matriz completa | isolar como profile experimental |
| GPU encoder com qualidade diferente de software | decisão enviesada por throughput | comparar qualidade por bitrate separadamente |
| Matriz completa cara e demorada | custo alto | começar por smoke e matriz reduzida |
| Dataset sem representatividade | decisão ruim | preservar as 5 categorias e 3 níveis de complexidade |
| Resultados afetados por paralelismo | métricas instáveis | executar testes críticos 3 vezes e usar mediana |
| Preços EC2 mudarem | custo incorreto | recalcular preços no início da rodada cloud |

## 11. Definição de Pronto

O benchmarking está implantado quando:

- os 30 vídeos estão catalogados
- a matriz de jobs é gerada automaticamente
- o runner executa jobs de forma retomável
- resultados são persistidos em JSONL e CSV
- RTF, tempo, bitrate, tamanho e status são coletados
- VMAF, SSIM e PSNR são coletados quando dependências existem
- CPU/RAM e GPU são medidos quando disponíveis
- relatórios de qualidade, tempo e custo são gerados
- local, EC2 CPU e EC2 GPU podem ser comparados
- existe recomendação final de codec por cenário

## 12. Primeiro Incremento Implementável

O primeiro PR deve ser pequeno e validar a base:

Escopo:

- `cmd/benchmark`
- manifest loader
- `ffprobe` catalog
- planner de matriz smoke
- testes unitários

Fora do primeiro PR:

- execução FFmpeg em massa
- VMAF/SSIM/PSNR
- GPU
- EC2
- fila dedicada

Critério de aceite do primeiro PR:

- `go run ./cmd/benchmark catalog` gera catálogo
- `go run ./cmd/benchmark plan` gera `jobs.jsonl`
- `go test ./...` passa
