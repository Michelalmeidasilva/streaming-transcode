Abaixo está um **planejamento de projeto** para desenvolver e validar uma plataforma própria de transcodificação de vídeo em alta escala, sem depender de serviços prontos como AWS Elemental MediaPackage/MediaConvert.

---

# Planejamento do Projeto — Sistema de Transcoding de Vídeo em Alta Escala

## 1. Objetivo

Desenvolver, implantar e testar uma solução própria para **processamento, transcodificação e preparação de vídeos para streaming**, executável em:

1. **Máquina local/on-premise**
2. **Servidores em nuvem**
3. **Instâncias EC2 com diferentes perfis de CPU/GPU**

A solução deve comparar codecs, qualidade, tempo de processamento, uso de CPU/GPU e custo total para processar um dataset de **30 vídeos comerciais**.

---

## 2. Escopo técnico

### Codecs do estudo comparativo

| Codec |    Nome técnico | Encoder sugerido                                         | Papel no estudo                            |
| ----- | --------------: | -------------------------------------------------------- | ------------------------------------------ |
| H.264 |             AVC | `libx264`, `h264_nvenc`                                  | Baseline de compatibilidade                |
| H.265 |            HEVC | `libx265`, `hevc_nvenc`                                  | Melhor compressão que H.264, bom para 4K   |
| H.266 |             VVC | VVenC                                                    | Codec avançado, foco em pesquisa/benchmark |
| VP9   | WebM/Google VP9 | `libvpx-vp9`                                             | Alternativa aberta para web                |
| AV1   | AOMedia Video 1 | `libsvtav1`, `libaom-av1`, `av1_nvenc` em GPU compatível | Codec moderno para alta eficiência         |

FFmpeg será a ferramenta-base recomendada para orquestrar os testes, pois é um conversor universal capaz de ler, filtrar e transcodificar uma ampla variedade de formatos, além de expor codecs/encoders por `libavcodec`. ([FFmpeg][2])

Para H.266/VVC, o estudo deve usar **VVenC**, encoder da Fraunhofer para VVC/H.266. A Fraunhofer descreve VVC/H.266 como sucessor de H.265/HEVC, com meta de redução de bitrate de cerca de 50% para qualidade subjetiva semelhante, mas sua adoção prática ainda deve ser tratada como menos madura que H.264, H.265, VP9 e AV1. ([Fraunhofer Heinrich-Hertz-Institut][3])

---

## 3. Arquitetura proposta

```text
Ingestão de vídeo
      ↓
Storage bruto
S3 / MinIO / Ceph / NAS local
      ↓
Extração de metadados
ffprobe + análise de FPS, duração, resolução, bitrate, complexidade
      ↓
Fila de jobs
Redis / RabbitMQ / Kafka / SQS-compatible
      ↓
Workers de transcodificação
CPU workers + GPU workers
      ↓
Validação de qualidade
VMAF, SSIM, PSNR, bitrate, tamanho final, playback check
      ↓
Empacotamento para streaming
HLS / DASH / CMAF
      ↓
Storage final
Object storage / filesystem distribuído

A solução não depende de MediaPackage/MediaConvert. O empacotamento vai feito com **FFmpeg segmenter** e **Shaka Packager**, e o armazenamento será o S3 na nuvem e o MinIO local.

---

## 4. Dataset de 30 vídeos

### Distribuição proposta

| Grupo                           | Quantidade | Conteúdo                                               |
| ------------------------------- | ---------: | ------------------------------------------------------ |
| Comerciais de beleza            |          6 | Produtos de pele, maquiagem, cabelo, close-up de rosto |
| Comerciais de medicamentos      |          6 | Texto legal, embalagens, pessoas, cenas explicativas   |
| Comerciais de perfume           |          6 | Alto contraste, partículas, blur, cenas artísticas     |
| Comercial educativo de farmácia |          6 | Fala, lettering, animações, gráficos simples           |
| Comerciais sazonais             |          6 | Antigripais, protetor solar, kits de beleza, vitaminas |

### Subdivisão de complexidade

Cada grupo deve conter:

| Complexidade | Quantidade por grupo | Característica                                              |
| ------------ | -------------------: | ----------------------------------------------------------- |
| Baixa        |                    2 | Fundo estático, pouco movimento, poucos cortes              |
| Média        |                    2 | Pessoas, movimento moderado, texto sobreposto               |
| Alta         |                    2 | Câmera rápida, partículas, cortes frequentes, muito detalhe |

### Metadados obrigatórios por vídeo

Para cada um dos 30 vídeos, registrar:

```text
video_id
categoria
subcategoria
duração
resolução de origem
FPS de origem
codec de origem
bitrate de origem
áudio: codec, sample rate, canais
número de cortes por minuto
nível de movimento
nível de detalhe/textura
presença de texto pequeno
presença de pele/rosto
presença de produto/embalagem
classificação de complexidade: baixa, média, alta
```

---

## 5. Mapeamento do FPS ideal

Regra principal: **não alterar FPS sem necessidade**. O FPS ideal deve ser derivado do FPS de origem e da característica visual do vídeo.

| Tipo de vídeo                                        |                          FPS recomendado | Justificativa                             |
| ---------------------------------------------------- | ---------------------------------------: | ----------------------------------------- |
| Produto estático, beleza, embalagem, lettering       |                         24, 25 ou 30 fps | Baixo ganho perceptual acima disso        |
| Educativo com apresentador/fala                      |                             25 ou 30 fps | Boa fluidez com custo controlado          |
| Perfume com câmera lenta ou estética cinematográfica |                   Preservar FPS original | Evita artefatos em motion blur            |
| Comercial com cortes rápidos                         | 30 fps, ou preservar se origem for maior | Reduz judder em transições                |
| Esportes, games ou movimento extremo                 |                                50/60 fps | Só aplicar se o conteúdo realmente exigir |

Critério de decisão:

```text
Se source_fps <= 30:
    manter source_fps
Se source_fps > 30 e motion_score baixo:
    testar downsample para 30 fps
Se source_fps > 30 e motion_score alto:
    preservar source_fps
Nunca interpolar FPS para cima sem requisito explícito.
```

---

## 6. Renditions de saída

O estudo deve gerar três resoluções principais:

| Rendition | Resolução | Uso                                    |
| --------- | --------: | -------------------------------------- |
| 4K        | 3840x2160 | Alta qualidade / TV / benchmark pesado |
| 1080p     | 1920x1080 | Streaming principal                    |
| 720p      |  1280x720 | Redes limitadas / mobile / fallback    |

### Ladder inicial de bitrate para teste

Estes valores são apenas ponto de partida. A decisão final deve vir de curva qualidade versus bitrate.

| Resolução |      H.264 |    H.265/VP9 |        AV1 |      H.266 |
| --------- | ---------: | -----------: | ---------: | ---------: |
| 4K        | 12–20 Mbps |    8–14 Mbps |  6–12 Mbps |  5–10 Mbps |
| 1080p     |   4–8 Mbps |   2.5–5 Mbps | 2–4.5 Mbps | 1.8–4 Mbps |
| 720p      |   2–4 Mbps | 1.2–2.5 Mbps | 1–2.2 Mbps | 0.8–2 Mbps |

O benchmark deve testar tanto **bitrate fixo/VBR controlado** quanto **modo qualidade**, por exemplo CRF/CQ, porque o objetivo é entender a relação entre qualidade, tamanho final e custo.

---

## 7. Métricas de qualidade

### Métricas obrigatórias

| Métrica         | Como medir                | Objetivo                                         |
| --------------- | ------------------------- | ------------------------------------------------ |
| Bitrate final   | `ffprobe`                 | Verificar consumo de banda                       |
| Tamanho final   | filesystem/object storage | Medir economia de armazenamento                  |
| Resolução       | `ffprobe`                 | Validar 4K, 1080p, 720p                          |
| FPS final       | `ffprobe`                 | Validar preservação ou conversão de FPS          |
| VMAF            | FFmpeg + libvmaf          | Qualidade perceptual objetiva                    |
| SSIM            | FFmpeg                    | Similaridade estrutural                          |
| PSNR            | FFmpeg                    | Diferença matemática entre original e codificado |
| Inspeção visual | Player + checklist        | Capturar artefatos não refletidos por métrica    |

VMAF é uma métrica full-reference usada para estimar qualidade perceptual comparando vídeo original e vídeo comprimido; é adequada para comparar codecs, encoders e configurações. ([Wikipedia][4])

### Critério inicial de aceitação

| Resolução | VMAF mínimo sugerido | Observação                     |
| --------- | -------------------: | ------------------------------ |
| 4K        |                 ≥ 95 | Para material premium          |
| 1080p     |                 ≥ 93 | Qualidade alta para streaming  |
| 720p      |                 ≥ 90 | Aceitável para mobile/fallback |

Esses thresholds devem ser calibrados após inspeção visual dos comerciais, especialmente porque vídeos com pele, texto pequeno, embalagem e partículas podem expor artefatos diferentes.

---

## 8. Métricas de desempenho

### Métricas obrigatórias

| Métrica                         | Definição                               |
| ------------------------------- | --------------------------------------- |
| Tempo total de transcodificação | Tempo wall-clock do job                 |
| Real-time factor — RTF          | `tempo_processamento / duração_vídeo`   |
| Velocidade relativa             | Ex.: 1h de vídeo em 30min = 2x realtime |
| FPS de encoding                 | Frames processados por segundo          |
| Uso médio de CPU                | `%CPU`, load average                    |
| Uso médio de GPU                | GPU utilization, encoder utilization    |
| Uso de memória                  | RAM média e pico                        |
| I/O de disco                    | leitura/escrita MB/s                    |
| Throughput por máquina          | vídeos/hora ou minutos de vídeo/hora    |
| Falhas/retries                  | jobs com erro, timeout, retry           |

Exemplo:

```text
RTF = tempo_de_transcodificação_segundos / duração_original_segundos

RTF = 0,5  → processa 2x mais rápido que tempo real
RTF = 1,0  → processa em tempo real
RTF = 2,0  → leva o dobro da duração do vídeo
```

---

## 9. Métricas de custo

### Custo local/on-premise

Para uma máquina local:

```text
custo_execução_local =
    energia_kWh
  + amortização_hardware
  + manutenção proporcional
  + armazenamento proporcional
```

Fórmula:

```text
energia_kWh = potência_média_kW × tempo_processamento_horas

custo_energia = energia_kWh × tarifa_kWh

amortização_hora =
    custo_total_hardware / vida_útil_em_horas_ativas

custo_local_total =
    custo_energia + (amortização_hora × tempo_processamento_horas)
```

Exemplo de campos que devem ser coletados:

```text
cpu_model
gpu_model
ram_gb
storage_type
potência_média_watts
tarifa_kWh
custo_hardware
vida_útil_meses
utilização_média_mensal
```

### Custo EC2

A AWS cobra EC2 On-Demand por hora ou por segundo, com mínimo de 60 segundos, sem compromisso de longo prazo. ([Amazon Web Services, Inc.][5])

Fórmula:

```text
custo_ec2 =
    tempo_horas × preço_hora_instância
  + custo_EBS
  + custo_S3/object storage
  + custo_transferência
  + logs/monitoramento
```

### Instâncias EC2 candidatas para benchmark

Preços abaixo são referências em **us-east-1** e devem ser recalculados na região final, especialmente se o processamento ocorrer em **sa-east-1 / São Paulo**.

| Perfil        | Instância   | Uso                          | Preço referência |
| ------------- | ----------- | ---------------------------- | ---------------: |
| CPU média     | c7i.4xlarge | H.264/H.265/VP9/AV1 software |      US$ 0,714/h |
| CPU alta      | c7i.8xlarge | Software encoding paralelo   |      US$ 1,428/h |
| GPU L4 básica | g6.xlarge   | H.264/H.265/AV1 via hardware |     US$ 0,8048/h |
| GPU L4 maior  | g6.2xlarge  | GPU + mais CPU/RAM           |     US$ 0,9776/h |
| GPU L4 escala | g6.8xlarge  | Mais paralelismo             |     US$ 2,0144/h |

As instâncias C7i são indicadas pela AWS para workloads compute-intensive, incluindo codificação de vídeo, e as G6 usam GPUs NVIDIA L4; a página da AWS informa que G6 tem decodificadores/encoders de vídeo e capacidade de hardware encoding AV1. ([Amazon Web Services, Inc.][6])

Referências de preço: c7i.4xlarge a US$ 0,714/h, c7i.8xlarge a US$ 1,428/h, g6.xlarge a US$ 0,8048/h e g6.2xlarge a US$ 0,9776/h em us-east-1. ([Vantage][7])

---

## 10. Experimento principal: converter 30 vídeos

### Rodada 1 — Benchmark técnico completo

Objetivo: comparar codecs e presets.

```text
30 vídeos
× 5 codecs
× 3 resoluções
× 3 níveis de qualidade/bitrate
= 1.350 jobs
```

Essa rodada serve para construir a curva:

```text
qualidade × bitrate × tempo × custo
```

### Rodada 2 — Benchmark operacional

Objetivo: medir quanto tempo e custo são necessários para processar os **30 vídeos reais** com uma configuração candidata de produção.

Configurações sugeridas:

| Cenário               | Codec | Resoluções      | Ambiente     |
| --------------------- | ----- | --------------- | ------------ |
| Baseline compatível   | H.264 | 4K, 1080p, 720p | CPU e GPU    |
| Alta compressão atual | H.265 | 4K, 1080p, 720p | CPU e GPU    |
| Web aberto            | VP9   | 4K, 1080p, 720p | CPU          |
| Moderno aberto        | AV1   | 4K, 1080p, 720p | CPU e GPU L4 |
| Pesquisa/futuro       | H.266 | 4K, 1080p, 720p | CPU          |

---

## 11. Modelo de cálculo do tempo para 30 vídeos

Para cada vídeo:

```text
tempo_job = duração_video × RTF(codec, resolução, preset, máquina)
```

Para 30 vídeos em uma máquina:

```text
tempo_total_30 =
    soma(tempo_job_1 ... tempo_job_30) / paralelismo_efetivo
```

Exemplo de leitura:

```text
Se o total bruto dos vídeos for 30 minutos
e o codec/máquina processar a 2x realtime,
tempo estimado = 15 minutos + overhead.
```

Para processamento paralelo:

```text
paralelismo_efetivo = min(
    número_de_jobs_paralelos,
    limite_CPU,
    limite_GPU_encoder,
    limite_IO,
    limite_memória
)
```

---

## 12. Schema de coleta de resultados

Criar um CSV/Parquet por execução:

```text
run_id
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

---

## 13. Pipeline de benchmark

### Etapa A — Preparação

1. Normalizar nomes dos 30 arquivos.
2. Extrair metadados com `ffprobe`.
3. Classificar complexidade: baixa, média, alta.
4. Definir FPS ideal por vídeo.
5. Gerar plano de jobs.

### Etapa B — Execução

1. Executar cada transcode em ambiente controlado.
2. Registrar logs em JSON.
3. Medir CPU/GPU/memória/I/O.
4. Repetir cada teste crítico 3 vezes.
5. Usar mediana como valor oficial.

### Etapa C — Validação

1. Comparar arquivo final contra original.
2. Medir VMAF/SSIM/PSNR.
3. Validar playback em HLS/DASH.
4. Checar sincronismo áudio/vídeo.
5. Checar artefatos visuais em cenas críticas.

### Etapa D — Consolidação

1. Gerar ranking por codec.
2. Gerar ranking por máquina.
3. Gerar custo por 30 vídeos.
4. Gerar custo por minuto processado.
5. Gerar recomendação de produção.

---

## 14. Cronograma sugerido

| Semana | Atividade                                     | Entregável                       |
| -----: | --------------------------------------------- | -------------------------------- |
|      1 | Definição de arquitetura e ambiente           | Documento de arquitetura e setup |
|      2 | Preparação do dataset e extração de metadados | Catálogo dos 30 vídeos           |
|      3 | Implementação do pipeline de jobs             | Transcoder funcional             |
|      4 | Benchmark local/on-premise                    | Relatório da máquina local       |
|      5 | Benchmark EC2 CPU                             | Relatório c7i/cpu-only           |
|      6 | Benchmark EC2 GPU                             | Relatório g6/gpu                 |
|      7 | Análise de qualidade e custo                  | Matriz codec × custo × qualidade |
|      8 | Recomendação final e plano de produção        | Relatório executivo + backlog    |

---

## 15. Critérios de decisão

A escolha final não deve ser apenas “codec mais eficiente”. Deve considerar:

| Critério                                 | Peso sugerido |
| ---------------------------------------- | ------------: |
| Qualidade perceptual                     |           30% |
| Tempo de transcodificação                |           20% |
| Custo por 30 vídeos                      |           20% |
| Compatibilidade com players/dispositivos |           15% |
| Complexidade operacional                 |           10% |
| Risco de licenciamento/suporte           |            5% |

### Resultado esperado por perfil

| Perfil                                          | Codec provável |
| ----------------------------------------------- | -------------- |
| Máxima compatibilidade                          | H.264          |
| Melhor equilíbrio atual para TV/mobile modernos | H.265          |
| Melhor opção aberta moderna                     | AV1            |
| Web aberto legado/moderno                       | VP9            |
| Pesquisa e futuro                               | H.266/VVC      |

---

## 16. Recomendação técnica preliminar

Para produção, eu trataria o projeto em três trilhas:

1. **Baseline obrigatório:** H.264 para compatibilidade universal.
2. **Alta eficiência atual:** H.265 e AV1 para reduzir bitrate e armazenamento.
3. **Pesquisa:** H.266/VVC, sem assumir adoção imediata em produção.

Para infraestrutura:

* **CPU-only** é melhor para medir qualidade máxima e comparar codecs de forma justa.
* **GPU L4/G6** deve ser testada para throughput operacional, especialmente H.264, H.265 e AV1. A vantagem tende a ser tempo/custo, mas a qualidade por bitrate precisa ser comparada contra os encoders software.
* **H.266/VVC** deve ser avaliado como benchmark de futuro, não como codec primário de entrega.

---

## 17. Entregáveis finais do projeto

1. **Arquitetura da solução**
2. **Pipeline de transcodificação local e cloud**
3. **Dataset catalogado com 30 vídeos**
4. **Matriz de benchmark por codec**
5. **Relatório de qualidade por resolução**
6. **Relatório de tempo para converter 30 vídeos**
7. **Relatório de custo local**
8. **Relatório de custo EC2 por instância**
9. **Recomendação final de codec por cenário**
10. **Backlog para produção: autoscaling, fila, observabilidade, retry, storage e empacotamento HLS/DASH**

---

## 18. Próximo passo prático

O primeiro artefato a criar é a **planilha de benchmark**, com uma linha por combinação:

```text
vídeo × codec × resolução × bitrate/preset × máquina
```

Sem a duração real dos 30 vídeos e a especificação da máquina local, ainda não é possível calcular o custo final com precisão. O planejamento acima define a metodologia para chegar ao número correto de:

```text
tempo para converter 30 vídeos
custo local por rodada
custo EC2 por rodada
melhor codec por qualidade/custo
melhor máquina por throughput/custo
```

[1]: https://www.webmproject.org/vp9/?utm_source=chatgpt.com "VP9 Video Codec Summary"
[2]: https://ffmpeg.org/ffmpeg.html?utm_source=chatgpt.com "ffmpeg Documentation"
[3]: https://www.hhi.fraunhofer.de/en/departments/vca/technologies-and-solutions/h266-vvc.html?utm_source=chatgpt.com "H.266 / VVC - Fraunhofer Heinrich-Hertz-Institut"
[4]: https://en.wikipedia.org/wiki/Video_Multimethod_Assessment_Fusion?utm_source=chatgpt.com "Video Multimethod Assessment Fusion"
[5]: https://aws.amazon.com/ec2/pricing/on-demand/?utm_source=chatgpt.com "EC2 On-Demand Instance Pricing"
[6]: https://aws.amazon.com/about-aws/whats-new/2023/09/amazon-ec2-c7i-instances/?utm_source=chatgpt.com "Introducing Amazon EC2 C7i instances - AWS"
[7]: https://instances.vantage.sh/aws/ec2/c7i.4xlarge?utm_source=chatgpt.com "c7i.4xlarge pricing and specs - Vantage Instances"
