# Discussao Academica: Aplicacao de Conceitos de HPC em Nuvem ao Workload de Transcoding

## 1. Contextualizacao

O microservico `streaming-transcode` compoe uma arquitetura de Video on Demand (VOD) orientada a eventos. Sua funcao principal e consumir mensagens de upload concluido, obter o arquivo original em armazenamento de objetos, executar transcodificacao com FFmpeg, gerar pacotes HLS/DASH e publicar os artefatos resultantes em storage. Embora esse servico nao seja, em sentido estrito, uma aplicacao classica de High Performance Computing (HPC) baseada em MPI e comunicacao fortemente acoplada, ele apresenta caracteristicas que permitem a aplicacao de varios conceitos discutidos nos trabalhos de Marcio Castro e colaboradores sobre HPC em nuvem.

A diferenca central esta no tipo de paralelismo. Nos trabalhos sobre resolvedores MPI, Lattice Boltzmann, N-Body e algebra linear densa, o foco recai sobre aplicacoes cientificas que frequentemente exigem coordenacao entre processos, comunicacao entre nos, uso intensivo de CPU/GPU e sensibilidade a latencia de rede. No `streaming-transcode`, por outro lado, o paralelismo mais natural ocorre entre jobs independentes: cada video pode ser tratado como uma unidade autonoma de processamento. Assim, os conceitos de HPC em nuvem devem ser transpostos para o dominio de transcoding como uma analise de workload, custo, infraestrutura, tolerancia a falhas e escalabilidade operacional.

Essa distincao e importante para o TCC porque evita uma equivalencia simplista entre transcoding e HPC classico. O servico nao precisa necessariamente transformar um unico video em uma aplicacao distribuida tightly-coupled. A contribuicao mais consistente esta em aplicar principios de avaliacao experimental, selecao de infraestrutura e otimizacao custo-desempenho, tal como defendido em `HPC@Cloud: A Provider-Agnostic Software Framework for Enabling HPC in Public Cloud Platforms`, na dissertacao `HPC@Cloud: A Provider-Agnostic Toolkit to Enable the Execution of HPC Applications on Public Clouds` e nos estudos posteriores sobre StarPU, instancias spot e instancias burstable.

## 2. Workload principal: transcoding como processamento paralelo por jobs independentes

O workload predominante do `streaming-transcode` e composto por tarefas independentes de conversao de video. Cada mensagem consumida da fila representa um video a ser processado. O worker realiza download, analise com `ffprobe`, geracao de renditions, empacotamento HLS/DASH e upload dos resultados. Esse fluxo se aproxima mais de um workload do tipo bag-of-tasks do que de uma aplicacao HPC fortemente acoplada.

Essa caracteristica aproxima o servico de uma classe de problemas em que o ganho de escala ocorre por replicacao de workers, e nao por decomposicao distribuida de uma unica execucao. Em outras palavras, a estrategia mais adequada e processar varios videos simultaneamente em diferentes workers, que podem estar na mesma maquina ou distribuidos em varias maquinas. Essa interpretacao dialoga com os resultados de `Benchmarking the scalability of MPI-based parallel solvers for fluid dynamics in low-budget cloud infrastructure`, pois o artigo mostra que o uso eficiente de multiplos nos depende do tamanho do problema e do custo de comunicacao. No caso do transcoding, como videos independentes nao exigem comunicacao entre si, a distribuicao por jobs evita grande parte do overhead observado em aplicacoes MPI.

Esse ponto tambem se relaciona ao estudo `Performance Evaluation of Dense Linear Algebra Kernels using Chameleon and StarPU on AWS`, no qual a comparacao entre um no mais poderoso e um cluster de nos menores mostra que escalar horizontalmente nem sempre reduz custo ou tempo. Para o `streaming-transcode`, essa conclusao sugere que a arquitetura deve comparar empiricamente duas estrategias: usar maquinas maiores com menos workers ou usar varias maquinas menores com mais workers. A escolha nao deve ser feita por intuicao, mas por medicao do throughput por custo.

## 3. Workload CPU-bound: codecs, presets e custo computacional

A etapa de encode tende a ser o trecho mais intensivo em CPU quando codecs como H.264, H.265, VP9, AV1 ou VVC sao executados sem aceleracao por hardware. Nesse caso, o desempenho do microservico depende diretamente do codec, do preset, da resolucao, da duracao do video e da complexidade visual do conteudo. Esse comportamento permite aplicar os achados dos estudos sobre instancias burstable e on-demand.

Em `Comparing Burstable and On-Demand AWS EC2 Instances using NAS Parallel Benchmarks`, Ferrari, Filho e Castro avaliam instancias burstable e instancias padrao da AWS usando NAS Parallel Benchmarks. O resultado indica que instancias burstable podem ser competitivas para algumas cargas, mas apresentam maior imprevisibilidade quando a aplicacao consome CPU de forma intensa ou prolongada. Essa conclusao e reforcada por `Avaliacao Preliminar do Desempenho e Custo Financeiro de Aplicacoes de HPC em Clusters de Instancias Burstable da AWS`, no qual os autores observam que instancias non-burstable sao mais adequadas para cargas longas e intensivas, enquanto instancias burstable podem ser mais apropriadas para cargas leves ou intermitentes.

Aplicando esse raciocinio ao `streaming-transcode`, a escolha de instancias burstable para encode pesado deve ser tratada com cautela. Codecs como AV1, VP9 e VVC podem manter uso elevado de CPU por longos periodos, consumindo creditos rapidamente e prejudicando previsibilidade. Por outro lado, workloads de baixa prioridade, validacoes curtas, thumbnails, analises de metadados ou pre-processamentos leves podem se beneficiar de instancias burstable, desde que exista medicao de custo real por minuto processado.

Portanto, para esse workload, a metrica principal nao deve ser apenas tempo total de execucao. Devem ser coletados `RTF` (real-time factor), custo por minuto de video, custo por rendition, consumo medio e maximo de CPU, variancia entre execucoes e taxa de falha. Essa abordagem segue a linha metodologica dos artigos, que avaliam desempenho junto com custo financeiro, e nao apenas desempenho bruto.

## 4. Workload heterogeneo: CPU, GPU e runtime de execucao

Os estudos sobre StarPU sao especialmente relevantes para discutir a possibilidade de aceleracao por GPU no `streaming-transcode`. Em `Performance Evaluation of N-Body Simulations on AWS with StarPU, OpenMP and MPI Runtime Systems`, Vanz, Munhoz e Castro demonstram que runtimes e modelos de programacao influenciam fortemente o desempenho em ambientes heterogeneos. O artigo mostra que StarPU pode apresentar excelente desempenho em configuracoes com GPU, mas sua eficiencia depende da granularidade das tarefas, da assimetria entre CPU e GPU e da configuracao da infraestrutura.

O artigo de periodico `Performance and Cost Evaluation of StarPU on AWS: Case Studies With Dense Linear Algebra Kernels and N-Body Simulations` aprofunda essa conclusao ao comparar workloads de algebra linear densa e simulacoes N-Body em configuracoes com nos fat e thin. O resultado mais importante para este TCC e que a melhor infraestrutura varia conforme a caracteristica do workload: em alguns casos, nos mais robustos e consolidados sao superiores; em outros, clusters de nos menores com GPU unica entregam melhor custo-desempenho.

No `streaming-transcode`, essa conclusao pode ser traduzida para a comparacao entre encode por CPU e encode acelerado por GPU. A decisao entre instancias CPU otimizadas e instancias com GPU nao deve assumir que GPU e automaticamente melhor. Aceleracao por GPU pode aumentar throughput, mas pode alterar qualidade visual, compatibilidade de codec, custo por hora, taxa de ocupacao e eficiencia por job. O benchmark do servico deve, portanto, comparar perfis como `cpu-general`, `cpu-compute`, `gpu-single-encoder` e `spot-cpu`, observando custo por minuto transcodificado e qualidade objetiva.

Essa discussao tambem permite transpor a ideia de runtime-aware scheduling. Embora StarPU nao seja uma dependencia adequada para o microservico, o principio pode ser aplicado por meio de filas especializadas: jobs H.264 leves podem ser roteados para workers CPU comuns; jobs AV1 ou VVC podem ir para workers mais robustos; jobs acelerados por GPU podem ser enviados a filas especificas associadas a maquinas com hardware apropriado. Assim, a ideia de escalonamento consciente do workload e preservada sem importar indevidamente uma arquitetura HPC para um servico de VOD.

## 5. Workload de I/O: armazenamento, rede e empacotamento HLS/DASH

O fluxo do `streaming-transcode` nao e composto apenas por encode. O servico tambem depende de download do arquivo original, escrita temporaria em disco, leitura dos arquivos intermediarios, geracao de segmentos HLS/DASH e upload de varios artefatos ao storage. Essa caracteristica aproxima o servico dos estudos que mostram o impacto de rede e armazenamento em workloads cientificos na nuvem.

Em `Evaluating the Parallel Simulation of Dynamics of Electrons in Molecules on AWS Spot Instances`, Munhoz, Castro e Rego avaliam a execucao da aplicacao DynEMol em AWS considerando instancias spot/on-demand, armazenamento EBS/FSx e tecnologias de rede como ENA/EFA. A conclusao do estudo e que a configuracao da infraestrutura afeta diretamente o desempenho e a relacao custo-beneficio. Ainda que o dominio da aplicacao seja diferente, o principio e diretamente aplicavel ao transcoding: storage e rede podem ser gargalos independentes do custo de CPU.

No caso do `streaming-transcode`, deve-se separar o benchmark de encode puro do benchmark ponta a ponta. O primeiro mede apenas a eficiencia do codec e da maquina. O segundo inclui download, empacotamento, upload e geracao de manifestos. Essa separacao e metodologicamente importante porque um worker pode parecer eficiente em encode, mas apresentar baixa eficiencia quando submetido a grande quantidade de segmentos HLS/DASH e operacoes de objeto em storage remoto.

A aplicacao pratica e enriquecer a observabilidade por etapa: `download_seconds`, `probe_seconds`, `transcode_seconds`, `packaging_seconds`, `upload_seconds`, tamanho de entrada, tamanho total de saida e quantidade de objetos gerados. Essa abordagem se alinha ao principio dos artigos de avaliar o sistema completo, e nao apenas o kernel computacional isolado.

## 6. Workload resiliente: instancias spot, falhas e retomada

Os artigos sobre instancias spot e tolerancia a falhas sao altamente aplicaveis ao `streaming-transcode`. Em `Avaliacao da Biblioteca SCR em Instancias AWS Spot Utilizando a Ferramenta HPC@Cloud`, Feres, Filho e Castro avaliam mecanismos de checkpoint/restart para aplicacoes HPC em clusters de instancias spot. O estudo mostra que a economia de custo proporcionada por instancias efemeras exige estrategias tecnicas de recuperacao. De forma semelhante, `Implementacao de Tolerancia a Falhas no Metodo Lattice Boltzmann para Execucao Resiliente em Instancias Efemeras da AWS` compara estrategias preemptivas e nao preemptivas, evidenciando o trade-off entre tempo de recuperacao, degradacao de desempenho e custo total.

No `streaming-transcode`, instancias spot podem ser interessantes porque transcoding e uma tarefa assincroma e tolerante a atraso em muitos cenarios de VOD. Entretanto, o uso de spot so e tecnicamente adequado se o pipeline for idempotente e retomavel. O servico ja possui elementos importantes para isso: fila RabbitMQ, dead-letter, retry, storage persistente e estados publicados no Event Gateway. Contudo, a verificacao atual de idempotencia e majoritariamente coarse-grained, baseada na existencia do manifesto HLS final.

Para aproximar o servico das boas praticas discutidas nos artigos, e necessario introduzir checkpoints logicos por etapa. Em vez de depender apenas de um resultado final, o worker deveria registrar a conclusao de cada fase: download, probe, cada rendition, empacotamento HLS, empacotamento DASH e upload. Cada artefato intermediario ou final deveria ser validado por tamanho, hash, metadados ou manifesto de estado. Assim, em caso de interrupcao da instancia spot, o reprocessamento poderia retomar do ultimo ponto consistente.

Essa abordagem adapta o conceito de checkpoint/restart ao dominio do transcoding. Diferentemente de uma aplicacao MPI, o checkpoint nao precisa capturar memoria de processos nem estado de comunicacao. Ele pode ser representado por artefatos persistidos em storage e por um manifesto de progresso do job. Essa adaptacao reduz complexidade e preserva o objetivo central dos artigos: transformar capacidade de nuvem barata e efemera em infraestrutura utilizavel com risco controlado.

## 7. Workload orquestrado: filas, Kubernetes e balanceamento

O artigo `Enabling Dynamic Rescheduling in Kubernetes Environments with Kubernetes Scheduling Extension (KSE)` contribui para a analise do `streaming-transcode` em um nivel diferente dos artigos de HPC. A preocupacao ali e o balanceamento de pods, uso de recursos, confiabilidade, disponibilidade e consumo de energia em clusters Kubernetes. Esse tema se conecta diretamente a uma evolucao natural do microservico: executar multiplos workers em um cluster orquestrado.

No servico atual, RabbitMQ ja atua como mecanismo de desacoplamento e distribuicao de carga. Em um ambiente Kubernetes, os workers poderiam ser escalados horizontalmente conforme backlog da fila, tempo medio de processamento, SLA e perfil do codec. Entretanto, a simples replicacao de pods pode gerar desequilibrio se todos os jobs forem tratados como equivalentes. Um video curto H.264 e um video longo AV1 possuem custos computacionais muito diferentes.

A contribuicao do estudo sobre KSE e reforcar que o escalonamento deve ser consciente do estado do cluster e das caracteristicas do workload. No `streaming-transcode`, isso pode ser implementado por filas separadas por classe de carga, workers especializados e metricas de autoscaling baseadas nao apenas no tamanho da fila, mas tambem no custo esperado dos jobs. Essa abordagem aproxima o servico de um modelo de orquestracao sensivel a recursos, sem exigir uma conversao para HPC classico.

## 8. Workload sustentavel: eficiencia energetica e Green Cloud Computing

O artigo `Green Cloud Computing: Challenges and Opportunities`, de Cordeiro, Francesquini, Amaris, Castro, Baldassin e Lima, amplia a discussao para sustentabilidade. O trabalho argumenta que plataformas de nuvem devem ser avaliadas tambem por eficiencia energetica e impacto ambiental, e nao apenas por custo financeiro e desempenho.

Essa perspectiva e relevante para transcoding porque encode de video pode ser uma atividade computacionalmente cara, especialmente em codecs mais modernos como AV1 e VVC. Um pipeline que reduz bitrate e melhora eficiencia de distribuicao pode economizar banda e armazenamento no longo prazo, mas pode tambem aumentar substancialmente o custo computacional inicial. Assim, existe um trade-off entre custo de processamento, custo de armazenamento, custo de entrega e impacto energetico.

Para o TCC, essa dimensao pode ser formulada como uma extensao da analise custo-desempenho: a configuracao ideal nao e necessariamente a que executa mais rapido, nem a que gera menor arquivo, mas a que equilibra qualidade, tempo de processamento, custo operacional, uso de recursos e impacto ambiental. Essa linha de raciocinio permite conectar o microservico a discussoes atuais de sustentabilidade em computacao em nuvem.

## 9. Workload experimental: benchmark como metodo de decisao

Um ponto comum aos artigos analisados e a centralidade da avaliacao experimental. `HPC@Cloud` propoe uma abordagem orientada a ferramenta para provisionar, testar e estimar custos de aplicacoes HPC em nuvens publicas. Os estudos sobre DynEMol, SCR, Lattice Boltzmann, N-Body, Chameleon/StarPU e instancias burstable seguem a mesma logica: a infraestrutura deve ser escolhida a partir de experimentos reprodutiveis, com metricas de desempenho e custo.

No `streaming-transcode`, essa abordagem ja aparece no documento `BENCHMARK-IMPLEMENTATION-PLAN.md`, que propoe comparar codecs, resolucoes, presets, qualidade objetiva, tempo de processamento, uso de recursos e custo. A contribuicao dos artigos e reforcar que esse benchmark nao deve ser um acessorio do projeto, mas o principal mecanismo de decisao arquitetural.

O benchmark deve ser estruturado por workload. Uma matriz minima deveria incluir:

- codec: H.264, H.265, AV1, VP9 e VVC;
- resolucao: 360p, 480p, 720p, 1080p e, se aplicavel, 4K;
- preset: rapido, medio e lento;
- perfil de maquina: CPU geral, CPU compute-optimized, GPU e spot;
- paralelismo: um worker por maquina, multiplos workers por maquina e multiplas maquinas;
- dataset: videos curtos, longos, com pouco movimento, com muito movimento, com texto pequeno e com pele/produto.

As metricas devem incluir `elapsed_s`, `RTF`, tamanho de saida, taxa de compressao, qualidade objetiva, uso medio/maximo de CPU, memoria, I/O, tempo por etapa, custo por job e custo por minuto processado. Essa estrutura permite responder empiricamente se a melhor estrategia e `scale-up`, `scale-out`, uso de GPU, uso de spot ou segmentacao de filas por codec.

## 10. Analise integrada

A analise conjunta dos artigos indica que o `streaming-transcode` pode incorporar conceitos de HPC em nuvem sem se tornar uma aplicacao HPC classica. A contribuicao central dos trabalhos de Castro e colaboradores nao esta apenas nas tecnologias especificas, como MPI, StarPU, SCR ou ULFM, mas na forma de pensar infraestrutura: workload primeiro, medicao empirica depois, decisao arquitetural por custo-desempenho e resiliência como parte do desenho do sistema.

No contexto do microservico, essa visao leva a quatro conclusoes principais. Primeiro, o paralelismo principal deve ser por video, com multiplos workers processando jobs independentes. Segundo, a escolha de infraestrutura deve ser especifica por codec e perfil de workload, evitando generalizacoes como "mais maquinas e sempre melhor" ou "GPU e sempre melhor". Terceiro, o uso de instancias spot e viavel apenas se houver retomada segura por artefatos e idempotencia forte. Quarto, observabilidade e benchmark devem ser tratados como elementos centrais do sistema, pois sem metricas por etapa nao e possivel tomar decisoes confiaveis sobre nuvem.

Assim, a relacao entre os artigos e o `streaming-transcode` pode ser sintetizada da seguinte forma: os trabalhos de HPC em nuvem oferecem a fundamentacao para transformar o microservico em um sistema experimentalmente orientado, capaz de escolher recursos de nuvem conforme as caracteristicas reais do workload. Essa abordagem e mais adequada do que tentar converter o servico em uma aplicacao HPC tightly-coupled, pois preserva a natureza assíncrona, elastica e orientada a eventos do pipeline VOD.

## 11. Implicacoes para o TCC

Para o TCC, o `streaming-transcode` pode ser apresentado como um estudo de aplicacao dos principios de HPC em nuvem a um workload moderno de video sob demanda. O objetivo nao e provar que transcoding e HPC classico, mas demonstrar que a mesma disciplina experimental usada em HPC pode orientar a construcao de servicos de nuvem intensivos em computacao.

Essa formulacao permite conectar o projeto aos artigos de Marcio Castro em tres niveis:

- nivel metodologico: uso de benchmark, medicao de custo e reproducibilidade, conforme `HPC@Cloud`;
- nivel arquitetural: comparacao entre `scale-up`, `scale-out`, CPU, GPU e diferentes perfis de instancia, conforme os estudos com StarPU, N-Body, algebra linear e burstable instances;
- nivel operacional: tolerancia a falhas, uso de spot, retomada e observabilidade, conforme os estudos com SCR, Lattice Boltzmann e DynEMol.

Dessa forma, os artigos fornecem insumos consistentes para justificar decisoes de arquitetura no microservico e tambem para estruturar uma avaliacao experimental no TCC. O resultado esperado e uma discussao menos abstrata sobre computacao em nuvem e mais fundamentada em evidencias: qual infraestrutura processa melhor determinado workload, com que custo, com que previsibilidade e com que nivel de resiliência.

## Referencias do acervo utilizadas na discussao

- `Benchmarking the scalability of MPI-based parallel solvers for fluid dynamics in low-budget cloud infrastructure`.
- `HPC@Cloud: A Provider-Agnostic Software Framework for Enabling HPC in Public Cloud Platforms`.
- `HPC@Cloud: A Provider-Agnostic Toolkit to Enable the Execution of HPC Applications on Public Clouds`.
- `Evaluating the Parallel Simulation of Dynamics of Electrons in Molecules on AWS Spot Instances`.
- `Avaliacao da Biblioteca SCR em Instancias AWS Spot Utilizando a Ferramenta HPC@Cloud`.
- `Implementacao de Tolerancia a Falhas no Metodo Lattice Boltzmann para Execucao Resiliente em Instancias Efemeras da AWS`.
- `Comparing Burstable and On-Demand AWS EC2 Instances using NAS Parallel Benchmarks`.
- `Avaliacao Preliminar do Desempenho e Custo Financeiro de Aplicacoes de HPC em Clusters de Instancias Burstable da AWS`.
- `Performance Evaluation of Dense Linear Algebra Kernels using Chameleon and StarPU on AWS`.
- `Performance Evaluation of N-Body Simulations on AWS with StarPU, OpenMP and MPI Runtime Systems`.
- `Performance and Cost Evaluation of StarPU on AWS: Case Studies With Dense Linear Algebra Kernels and N-Body Simulations`.
- `Enabling Dynamic Rescheduling in Kubernetes Environments with Kubernetes Scheduling Extension (KSE)`.
- `Green Cloud Computing: Challenges and Opportunities`.
