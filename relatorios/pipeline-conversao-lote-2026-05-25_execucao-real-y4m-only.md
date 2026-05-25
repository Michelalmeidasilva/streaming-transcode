# Relatorio de Execucao Real: Y4M Only

Data: 2026-05-25
Modo: `--skip-master`
Input: `dataset/pre-selecao-top4-bitrate`
Output: `outputs/reference-pipeline-preselecao-y4m`

## Resumo

- Videos na pre-selecao: `6`
- Arquivos `reference.y4m` encontrados: `6`
- Conversoes com `conversion.log` contendo `exit_code=0`: `5`
- Conversao com log sem fechamento formal: `1`
- Espaco livre restante ao final da execucao observada: `163 MiB`

## Videos processados

- `544796409` | `comercial-cortes` | `reference.y4m` gerado | tamanho aproximado `4.7 GiB`
- `847661749` | `comercial-cortes` | `reference.y4m` gerado | tamanho aproximado `5.2 GiB`
- `1008683772` | `comercial-estatico - poucos cortes` | `reference.y4m` gerado | tamanho aproximado `19 GiB`
- `842628073` | `comercial-estatico - poucos cortes` | `reference.y4m` gerado | tamanho aproximado `12 GiB`
- `1114576994` | `institucional-eventos-treinamento` | `reference.y4m` gerado | tamanho aproximado `48 GiB`
- `374557639` | `institucional-eventos-treinamento` | `reference.y4m` gerado | tamanho aproximado `28 GiB`

## Estado dos logs

Com `exit_code=0` confirmado:

- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/comercial-cortes/544796409-Versed-Skincare-Nix-It/conversion.log:1)
- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/comercial-cortes/847661749-Major-Fade-Active-Seal-Moisturizer-chicwithkels/conversion.log:1)
- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/comercial-estatico%20-%20poucos%20cortes/1008683772-MBC2-Cosmetics/conversion.log:1)
- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/comercial-estatico%20-%20poucos%20cortes/842628073-Ceramide-Moisturizer-alphasherpa/conversion.log:1)
- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/institucional-eventos-treinamento/1114576994-RealOptions-Obria-Medical-Clinics-Nancy-s-Reproductive-Loss-Healing-Story-of-Hope/conversion.log:1)

Sem fechamento formal do log, mas com `reference.y4m` presente:

- [conversion.log](/Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode/outputs/reference-pipeline-preselecao-y4m/institucional-eventos-treinamento/374557639-Total-Pharmacy-Supply-TPS-Wellgistics-Wholesale-Group-Discount-Pricing/conversion.log:1)

## Observacao critica

O disco atingiu `100%` de uso durante a execucao. Por isso:

- o pipeline principal nao gravou o relatorio final automatico desta rodada
- a execucao foi consolidada manualmente neste documento
- embora os `6` arquivos `reference.y4m` existam, o ultimo caso deve ser tratado como "gerado, mas requer validacao final" antes de uso definitivo

## Recomendacao imediata

Antes de qualquer nova execucao:

- liberar espaco em disco
- validar os `6` Y4M com `ffprobe`
- so depois retomar novos lotes
