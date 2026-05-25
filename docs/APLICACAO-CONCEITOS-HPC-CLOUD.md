# Aplicação de Conceitos de HPC em Nuvem ao `streaming-transcode`

## Contexto

Os artigos analisados de Márcio Castro estudam principalmente:

- custo versus desempenho em nuvem;
- seleção de instâncias;
- `scale-up` versus `scale-out`;
- tolerância a falhas com instâncias `spot`;
- previsibilidade de execução;
- efeito de rede, armazenamento e runtime no desempenho.

O serviço `streaming-transcode` não é um workload HPC clássico `tightly-coupled` via MPI. Ele é um pipeline de transcoding orientado a eventos, com jobs independentes, executados por FFmpeg, com persistência em S3/MinIO e coordenação por RabbitMQ.

Mesmo assim, vários conceitos dos artigos se aplicam de forma direta.

## O que se aplica diretamente

### 1. Custo-desempenho deve ser medido por workload real

Nos artigos, a escolha de infraestrutura depende do perfil do workload, e não apenas do preço por hora. Isso se aplica integralmente aqui.

No `streaming-transcode`, a decisão de máquina ideal não deve ser feita por “mais vCPU” ou “instância mais barata”, mas por métricas como:

- `elapsed_s` por minuto de vídeo;
- `RTF` (`real-time factor`);
- custo por vídeo processado;
- custo por minuto transcodificado;
- custo por rendition gerada;
- throughput por dólar;
- qualidade entregue por bitrate.

Aplicação prática no serviço:

- usar a estrutura já prevista em `docs/BENCHMARK-IMPLEMENTATION-PLAN.md`;
- transformar benchmarks locais em matriz comparativa por `codec`, `preset`, `resolução` e `machine profile`;
- sempre comparar em dataset representativo, não em um único vídeo.

### 2. `Scale-up` versus `scale-out`

Nos artigos de StarPU e álgebra linear, a conclusão é que `scale-up` e `scale-out` dependem do workload. Isso se traduz aqui da seguinte forma:

- um job individual de transcoding tende a se beneficiar de `scale-up` local, isto é, máquina mais forte por worker;
- o sistema inteiro tende a se beneficiar de `scale-out`, isto é, mais workers consumindo jobs independentes da fila.

Aplicação prática no serviço:

- não distribuir um único encode entre várias máquinas;
- distribuir vídeos diferentes entre workers diferentes;
- comparar:
  - `1 worker` em máquina maior;
  - `N workers` em máquinas menores;
  - `1 worker por core group` versus múltiplos workers concorrentes na mesma VM.

Conclusão operacional:

- para esse serviço, o paralelismo principal é `inter-job`, não `intra-job`;
- o equivalente ao resultado dos artigos é: evitar assumir que horizontalizar sempre será melhor; pode ser mais barato rodar menos workers em máquinas mais fortes, dependendo do codec.

### 3. Seleção de instância por perfil de codec

Os estudos com burstable, on-demand e GPUs mostram que a infraestrutura ideal depende do comportamento computacional da carga.

No `streaming-transcode`, isso se traduz em perfis distintos:

- `h264` com preset rápido: CPU-bound moderado, alta vazão;
- `h265`: CPU-bound mais pesado;
- `av1`: CPU-bound muito pesado;
- `vp9`: CPU-bound pesado;
- `vvc`: potencialmente ainda mais caro por job;
- futuramente, encode por GPU altera completamente a curva custo-desempenho.

Aplicação prática no serviço:

- manter `machine profiles` explícitos no benchmark;
- comparar perfis como:
  - `cpu-general-small`
  - `cpu-compute-large`
  - `gpu-single-encoder`
  - `spot-cpu`
- não usar instâncias burstable para benchmark principal de encode pesado;
- usar burstable apenas para tarefas leves, fila de baixa prioridade ou pré-processamento, nunca como default sem validação.

### 4. Tolerância a falhas com `spot`

Nos artigos, `spot` só é viável com estratégia clara de recuperação. Isso é totalmente aplicável aqui.

Hoje o serviço já possui elementos úteis:

- fila RabbitMQ com retry e dead-letter;
- objetos persistidos em storage;
- estados de processamento no Event Gateway;
- saída final materializada em S3/MinIO.

O que falta para aplicar o conceito de forma madura:

- checkpoint por etapa lógica, não por processo FFmpeg;
- retomada idempotente por fase;
- separação explícita entre:
  - `download concluído`
  - `probe concluído`
  - `rendition X concluída`
  - `HLS concluído`
  - `DASH concluído`
  - `upload concluído`

Aplicação prática no serviço:

- persistir um manifesto intermediário por job em `metrics/{videoId}/job-state.json`;
- ao reprocessar, retomar do último artefato válido em vez de reiniciar tudo;
- tornar cada rendition uma unidade isolada de recuperação;
- se houver adoção futura de `spot`, interromper ou drenar workers com shutdown gracioso antes da revogação da instância.

### 5. Previsibilidade operacional

Um ponto recorrente dos artigos é que previsibilidade importa tanto quanto média de desempenho. Isso vale muito aqui.

Para transcoding em produção, importa saber:

- p95 de tempo por codec;
- variância por resolução;
- impacto de origem do arquivo;
- efeito de I/O local versus remoto;
- fila média e tempo de espera.

Aplicação prática no serviço:

- enriquecer `observability.json` com:
  - `queue_wait_seconds`
  - `download_seconds`
  - `probe_seconds`
  - `transcode_seconds_total`
  - `packaging_seconds`
  - `upload_seconds`
  - `failure_stage`
- produzir relatórios com `p50/p95/p99`, e não apenas média.

### 6. Rede e storage importam

Nos artigos científicos, rede e storage alteram fortemente o desempenho. Aqui isso também é verdadeiro, embora em outro formato.

No `streaming-transcode`, os gargalos mais prováveis são:

- download do vídeo original do MinIO/S3;
- upload dos segmentos HLS/DASH;
- I/O local durante encode;
- storage remoto durante execução concorrente.

Aplicação prática no serviço:

- separar benchmark de encode puro do benchmark ponta a ponta;
- medir:
  - tempo de `download`;
  - tempo de `encode`;
  - tempo de `packaging`;
  - tempo de `upload`;
- comparar disco local rápido versus volume de rede;
- comparar saída segmentada pequena versus arquivo único grande.

## O que não se transfere diretamente

### 1. MPI e comunicação tightly-coupled

Os artigos sobre MPI, Lattice Boltzmann e N-Body tratam de comunicação intensa entre processos distribuídos. Isso não é o perfil deste serviço.

No `streaming-transcode`:

- cada job é majoritariamente independente;
- a fila coordena concorrência, mas não existe troca intensa de estado entre workers;
- o custo principal está em CPU, codec e I/O, não em sincronização distribuída.

Portanto:

- não vale tentar “paralelizar um vídeo em múltiplos nós” como linha principal;
- o paralelismo distribuído correto é por lote de vídeos.

### 2. Escalonadores estilo StarPU

StarPU é relevante como inspiração de “workload-aware scheduling”, mas não como tecnologia diretamente aplicável.

Aqui o equivalente prático seria:

- roteamento de jobs para filas por codec;
- workers especializados por perfil de hardware;
- despacho por capacidade real da máquina.

## Como traduzir isso em arquitetura do serviço

## 1. Filas por classe de workload

Em vez de uma única fila homogênea, separar por perfil:

- `transcode.h264.standard`
- `transcode.h265.heavy`
- `transcode.av1.heavy`
- `transcode.benchmark`

Benefícios:

- melhor alocação por tipo de worker;
- menor interferência entre jobs leves e pesados;
- previsibilidade melhor por SLA.

## 2. Workers especializados por hardware

Aplicar o conceito de infraestrutura alinhada ao workload:

- worker CPU para `h264`/`h265`;
- worker GPU para perfis futuros acelerados;
- worker spot para fila assíncrona de menor prioridade;
- worker on-demand para backlog crítico.

## 3. Benchmark contínuo como base de decisão

Os artigos mostram que não se deve escolher infraestrutura por intuição.

No serviço, isso significa institucionalizar benchmark:

- dataset fixo e versionado;
- execuções repetidas por matriz de codec/preset/máquina;
- relatórios comparáveis entre versões;
- recalcular custo com preços atuais do provedor antes de decisão de produção.

## 4. Idempotência forte e retomada por artefato

Se a meta for usar `spot` ou aumentar a escala com segurança:

- cada saída deve ser verificável por hash/tamanho/metadado;
- o worker deve reconhecer artefatos já gerados;
- reexecução deve ser segura sem duplicar estado lógico.

O código já tem um início disso ao verificar `transcoded/{videoId}/hls/master.m3u8`, mas isso ainda é coarse-grained. O ideal é granularidade por rendition e por pacote.

## 5. Observabilidade orientada a decisão de infraestrutura

Os artigos mais úteis do conjunto não medem apenas “funciona”; eles medem:

- custo;
- tempo;
- escalabilidade;
- estabilidade;
- impacto do ambiente.

Aplicação no serviço:

- transformar `observability.json` em base analítica para seleção de instância;
- adicionar campos de máquina:
  - tipo de instância;
  - preço/hora;
  - custo estimado do job;
  - provedor/região;
  - modo `spot` ou `on-demand`.

## Conceitos aplicados na criacao do dataset atual

O dataset atual em `outputs/reference-pipeline-preselecao-y4m` ja reflete, na pratica, varios dos conceitos discutidos acima, mesmo antes da execucao completa em nuvem.

### 1. Benchmark com workload representativo

A construcao do dataset nao foi feita com um unico video de exemplo. Primeiro houve analise automatizada dos videos do Vimeo, seguida de filtragem por resolucao e bitrate, para formar uma pre-selecao mais adequada a testes de codec.

Aplicacao pratica:

- extracao automatizada de metadados tecnicos como `codec`, `width`, `height`, `fps` e `bitrate_kbps`;
- deduplicacao por `video_id`, mantendo a melhor variante por video;
- descarte de videos abaixo do corte de `5000 kbps`, para evitar amostras fracas para benchmark;
- formacao de um conjunto pequeno, mas mais representativo, para testes controlados.

### 2. Reducao dirigida do espaco de busca

Os relatarios mostram que a tentativa de converter lotes maiores foi bloqueada por falta de espaco em disco, porque o volume estimado de `Y4M` era muito alto. Em vez de insistir em um lote inviavel, foi feita uma reducao progressiva ate chegar a um subconjunto executavel.

Aplicacao pratica:

- preflight baseado em estimativa de espaco antes da conversao;
- rejeicao automatica de lotes inviaveis;
- reducao do dataset para um conjunto final de `6` videos;
- priorizacao de um lote que coubesse no ambiente real de teste.

Isso corresponde ao principio de HPC/cloud de adaptar o workload aos limites reais de recurso disponivel, em vez de assumir capacidade infinita.

### 3. Separacao entre objetivo cientifico e custo operacional

Para viabilizar o dataset final, foi usado o modo `--skip-master`, gerando apenas `reference.y4m`. Essa decisao reduziu custo de armazenamento e I/O sem comprometer o objetivo principal, que era montar uma base de referencia para benchmark e comparacao de codecs.

Aplicacao pratica:

- eliminacao temporaria de artefatos nao essenciais para a fase de benchmark;
- foco no artefato de maior valor experimental, o `reference.y4m`;
- reducao da sobrecarga operacional para maximizar a quantidade de amostras validas geradas.

### 4. Dataset heterogeneo para evitar vies de avaliacao

O conjunto final nao ficou homogêneo em `fps`, duracao ou categoria semantica. Ele inclui videos de grupos diferentes, com perfis de movimento e cadencia distintos, incluindo amostras em torno de `24 fps` e `60 fps`.

Aplicacao pratica:

- inclusao de categorias como `comercial-cortes`, `comercial-estatico - poucos cortes` e `institucional-eventos-treinamento`;
- preservacao de variedade de perfil visual e temporal;
- reducao do risco de otimizar o benchmark para um unico tipo de conteudo.

Isso segue o mesmo principio dos artigos: medir desempenho apenas em um caso favoravel gera conclusoes fracas sobre custo-desempenho.

### 5. Execucao incremental com validacao por artefato

O dataset final foi consolidado de forma incremental. Primeiro foram gerados `5` arquivos com log formal de sucesso e, depois, o ultimo video foi reprocessado isoladamente para completar o conjunto.

Aplicacao pratica:

- execucao em ondas menores em vez de um lote monolitico;
- validacao por presenca de `reference.y4m` e `conversion.log`;
- recuperacao dirigida do item que falhou, sem reiniciar todo o conjunto.

Esse comportamento ja antecipa uma estrategia importante para nuvem e `spot`: trabalhar com unidades pequenas, verificaveis e reprocessaveis.

## Priorização prática para este repositório

### Prioridade 1

- separar claramente benchmark de produção;
- medir custo por job com base em preço/hora configurável;
- enriquecer métricas por etapa;
- testar `scale-up` versus `scale-out` com dataset real.

### Prioridade 2

- introduzir filas por classe de codec;
- criar perfis de worker por capacidade de máquina;
- adicionar reprocessamento idempotente por rendition.

### Prioridade 3

- preparar execução segura em `spot`;
- adicionar política de drenagem e retomada;
- comparar CPU versus GPU no mesmo framework de benchmark.

## Conclusão

Os conceitos dos artigos de Márcio Castro são aplicáveis ao `streaming-transcode`, mas com adaptação correta ao domínio.

O ponto principal não é tratar transcoding como HPC distribuído por MPI. O ponto principal é aplicar ao serviço os princípios de:

- seleção empírica de infraestrutura;
- otimização custo-desempenho;
- comparação entre `scale-up` e `scale-out`;
- tolerância a falhas para infraestrutura efêmera;
- observabilidade suficiente para decisão arquitetural.

Em termos práticos, a melhor tradução desses artigos para este serviço é:

- benchmark rigoroso;
- workers alinhados ao perfil do codec;
- filas segmentadas;
- retomada segura;
- eventual uso de `spot` apenas com idempotência e checkpoint lógico por etapa.
