# Relatorio Vimeo: analise, filtragem e automacao de downloads

Data: 2026-05-24
Repositorio: `streaming-transcode`

## Objetivo

Consolidar a analise de videos do Vimeo para:

- extrair bitrate, resolucao, codec, entrega e licenca quando disponivel
- filtrar os videos mais adequados para testes de codec
- automatizar o download dos videos selecionados

## O que foi implementado

### 1. CLI em Go para analisar videos do Vimeo

Foi criado o comando:

- [cmd/vimeo-analyzer/main.go](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/cmd/vimeo-analyzer/main.go:1)

Principais capacidades:

- leitura de URLs/IDs do Vimeo
- consulta de formatos via `yt-dlp`
- saida em `csv` ou `json`
- colunas com:
  - `input_url`
  - `video_id`
  - `title`
  - `license`
  - `delivery`
  - `variant`
  - `codec`
  - `width`
  - `height`
  - `fps`
  - `bitrate_kbps`
  - `bandwidth_bps`
  - `source_url`
  - `error`

Arquivos relacionados:

- [cmd/vimeo-analyzer/main_test.go](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/cmd/vimeo-analyzer/main_test.go:1)
- [dataset/vimeo_urls.txt](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/dataset/vimeo_urls.txt:1)

### 2. Mudanca de estrategia tecnica

Abordagem inicial:

- consulta direta ao endpoint `player.vimeo.com/video/{id}/config`

Problemas encontrados:

- falha de DNS dentro do sandbox
- mesmo fora do sandbox, Vimeo respondeu `403 Forbidden`

Abordagem final adotada:

- uso de `yt-dlp` como backend do analisador

Motivo:

- `yt-dlp` conseguiu extrair corretamente formatos `progressive`, `hls` e `dash`
- retornou bitrate e resolucao reais das variantes

### 3. Instalacao de ferramenta para visualizar CSV

Foi instalado via Homebrew:

- `visidata`

Uso:

```bash
vd vimeo-bitrates.csv
```

### 4. Script para baixar videos

Foi criado:

- [scripts/download_vimeo_videos.py](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/scripts/download_vimeo_videos.py:1)

Capacidades:

- leitura de [vimeo-bitrates.csv](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/vimeo-bitrates.csv:1)
- deduplicacao por `video_id`
- download com `yt-dlp`
- suporte a `--cookies-from-browser`
- retomada por indice com `--start-at`
- retries com backoff
- pausa entre downloads para reduzir `429 Too Many Requests`
- controle de repeticao via `downloads/vimeo/.downloaded.txt`

## Arquivos gerados

### Dataset bruto completo

- [vimeo-bitrates.full.csv](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/vimeo-bitrates.full.csv:1)

Descricao:

- resultado completo da analise
- mantem varias variantes por video
- total atual: `2445` linhas

### Dataset filtrado final

- [vimeo-bitrates.csv](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/vimeo-bitrates.csv:1)

Descricao:

- somente uma linha por `video_id`
- somente a variante escolhida como melhor candidata por video
- total atual: `57` linhas contando o cabecalho

### Backup de tentativa de filtro intermediaria

- [vimeo-bitrates.pre-filter-broken.csv](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/vimeo-bitrates.pre-filter-broken.csv:1)

Descricao:

- backup de uma filtragem intermediaria
- preservado apenas para auditoria

## Regras de filtragem aplicadas no CSV final

O arquivo final [vimeo-bitrates.csv](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/vimeo-bitrates.csv:1) foi reescrito com estas regras:

1. Manter apenas uma linha por `video_id`.
2. Escolher a linha de maior `height`.
3. Em empate de resolucao, escolher a linha de maior `bitrate_kbps`.
4. Remover videos cujo melhor `bitrate_kbps` ficou abaixo de `5000`.

Observacao importante:

- a filtragem final foi refeita com parser CSV real em Python para evitar erro de parse em titulos com virgula

## Videos removidos por bitrate baixo

Os seguintes videos foram removidos do arquivo final por ficarem abaixo do corte de `5000 kbps`:

- `1192171109` | `SNARE` | `858p` | `3419 kbps`
- `158915032` | `SJ Summer -TV spot` | `720p` | `1431 kbps`
- `158920985` | `Akademikliniken Skincare - Peach` | `2160p` | `2476 kbps`
- `158921536` | `Akademikliniken Skincare - Banana` | `2160p` | `2109 kbps`
- `810341649` | `Dermal Therapy Acne Control Range | The only popping you'll be doing is the fun kind!` | `1440p` | `4895 kbps`
- `853253127` | `Pain Meds Pharmacy` | `2160p` | `2433 kbps`

## Comandos usados e recomendados

### Gerar CSV dos videos do Vimeo

```bash
env GOCACHE=/private/tmp/go-build go run ./cmd/vimeo-analyzer -input dataset/vimeo_urls.txt > vimeo-bitrates.csv
```

Com cookies do navegador:

```bash
env GOCACHE=/private/tmp/go-build go run ./cmd/vimeo-analyzer -input dataset/vimeo_urls.txt --cookies-from-browser firefox > vimeo-bitrates.csv
```

### Visualizar CSV

```bash
vd vimeo-bitrates.csv
```

### Baixar videos

Padrao:

```bash
python3 scripts/download_vimeo_videos.py
```

Com cookies do Firefox:

```bash
python3 scripts/download_vimeo_videos.py --cookies-from-browser firefox
```

Retomando a partir de um indice:

```bash
python3 scripts/download_vimeo_videos.py --start-at 2 --sleep-seconds 30 --retries 6 --retry-backoff 60 --cookies-from-browser firefox
```

Teste sem baixar:

```bash
python3 scripts/download_vimeo_videos.py --limit 3 --dry-run
```

## Problemas encontrados

### 1. Bloqueio do endpoint publico do player do Vimeo

Problemas observados:

- `no such host` no ambiente restrito
- `403 Forbidden` ao acessar `player.vimeo.com/video/{id}/config`

Resolucao:

- abandono da abordagem de fetch HTTP direto
- migracao para `yt-dlp`

### 2. Rate limit do Vimeo durante downloads

Erro observado:

- `HTTP Error 429: Too Many Requests`

Mitigacoes implementadas:

- `--sleep-seconds`
- `--retries`
- `--retry-backoff`
- `--start-at`
- recomendacao de usar `--cookies-from-browser firefox`

## Estado atual

Artefatos prontos:

- analisador Vimeo em Go
- testes do analisador
- dataset bruto completo
- dataset final filtrado
- script de download com retomada e retry
- ferramenta `visidata` instalada

Diretorios relevantes:

- [cmd/vimeo-analyzer](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/cmd/vimeo-analyzer:1)
- [scripts](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/scripts:1)
- [downloads/vimeo](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/downloads/vimeo:1)
- [relatorios](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/relatorios:1)

## Proximos passos sugeridos

- deduplicar ainda mais o dataset por orientacao ou familia de conteudo
- separar benchmarks por categoria: estatico, produto, entrevista, animacao, 4K
- incluir metrica de duracao no filtro final
- produzir um novo CSV apenas com os videos ja baixados
