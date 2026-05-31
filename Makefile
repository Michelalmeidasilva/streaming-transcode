.PHONY: help build build-linux build-image \
        test test-cover \
        transcode-h264 transcode-h265 transcode-av1 transcode-vp9 transcode-vvc \
        benchmark benchmark-h264 benchmark-h265 benchmark-av1 benchmark-vp9 benchmark-vvc \
        benchmark-all \
        pipeline-reference \
        docker-run docker-run-av1 docker-run-h264 \
        reports clean

# ─── defaults ────────────────────────────────────────────────────────────────
SAMPLE        ?= sample.mp4
CODEC         ?= av1
WIDTH         ?= 1280
HEIGHT        ?= 720
BITRATE_KBPS  ?= 3000
PRESET        ?= fast
REPORT_DIR    ?= relatorios
OUTPUT_DIR    ?= outputs/benchmark
RUNTIME       ?= auto          # auto | docker | local

BINARY_HOST   := dist/transcode-local-host
BINARY_LINUX  := dist/transcode-local-linux-amd64

# ─── help ────────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "streaming-transcode — estudo de caso de codecs de vídeo"
	@echo ""
	@echo "BUILD"
	@echo "  make build            Compila binário para o host atual"
	@echo "  make build-linux      Compila binário linux/amd64 (para Docker)"
	@echo "  make build-image      Builda imagem Docker (requer build-linux)"
	@echo ""
	@echo "TESTES"
	@echo "  make test             Roda todos os testes unitários"
	@echo "  make test-cover       Testes com cobertura (coverage.out)"
	@echo ""
	@echo "TRANSCODE MANUAL — uma rendição, execução local"
	@echo "  make transcode-h264   SAMPLE=<arquivo> WIDTH=<w> HEIGHT=<h> BITRATE_KBPS=<b>"
	@echo "  make transcode-h265"
	@echo "  make transcode-av1"
	@echo "  make transcode-vp9"
	@echo "  make transcode-vvc"
	@echo ""
	@echo "  Exemplo:"
	@echo "  make transcode-av1 SAMPLE=dataset/pre-selecao-top4-bitrate/comercial-cortes/544796409\ -\ Versed\ Skincare\ Nix-It.mp4"
	@echo ""
	@echo "BENCHMARK — multi-resolução (720p / 1080p / 2k / 4k)"
	@echo "  make benchmark        SAMPLE=<arquivo> CODEC=<codec> RUNTIME=<auto|docker|local>"
	@echo "  make benchmark-h264   atalho para CODEC=h264"
	@echo "  make benchmark-h265   atalho para CODEC=h265"
	@echo "  make benchmark-av1    atalho para CODEC=av1"
	@echo "  make benchmark-vp9    atalho para CODEC=vp9"
	@echo "  make benchmark-vvc    atalho para CODEC=vvc"
	@echo "  make benchmark-all    Roda todos os codecs em sequência"
	@echo ""
	@echo "PIPELINE"
	@echo "  make pipeline-reference  Converte MP4s do dataset para Y4M (entrada normalizada)"
	@echo ""
	@echo "DOCKER"
	@echo "  make docker-run       Transcodifica via Docker Compose (usa variáveis de ambiente)"
	@echo "  make docker-run-av1   Atalho AV1 720p com o SAMPLE padrão"
	@echo "  make docker-run-h264  Atalho H.264 720p com o SAMPLE padrão"
	@echo ""
	@echo "RELATÓRIOS"
	@echo "  make reports          Lista relatórios gerados em $(REPORT_DIR)/"
	@echo ""
	@echo "LIMPEZA"
	@echo "  make clean            Remove binários e outputs gerados"
	@echo ""

# ─── build ───────────────────────────────────────────────────────────────────
build:
	go build -o $(BINARY_HOST) ./cmd/transcode-local

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	  go build -o $(BINARY_LINUX) ./cmd/transcode-local

build-image: build-linux
	docker compose -f compose.yaml build transcode-local

# ─── testes ──────────────────────────────────────────────────────────────────
test:
	go test ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# ─── transcode manual ────────────────────────────────────────────────────────
_output = outputs/manual/$(notdir $(basename $(SAMPLE)))-$(1)-$(HEIGHT)p.mp4

transcode-h264: build
	./$(BINARY_HOST) \
	  --input  "$(SAMPLE)" \
	  --output "$(call _output,h264)" \
	  --codec  h264 \
	  --width  $(WIDTH) --height $(HEIGHT) \
	  --bitrate-kbps $(BITRATE_KBPS)

transcode-h265: build
	./$(BINARY_HOST) \
	  --input  "$(SAMPLE)" \
	  --output "$(call _output,h265)" \
	  --codec  h265 \
	  --width  $(WIDTH) --height $(HEIGHT) \
	  --bitrate-kbps $(BITRATE_KBPS)

transcode-av1: build
	./$(BINARY_HOST) \
	  --input  "$(SAMPLE)" \
	  --output "$(call _output,av1)" \
	  --codec  av1 \
	  --width  $(WIDTH) --height $(HEIGHT) \
	  --bitrate-kbps $(BITRATE_KBPS)

transcode-vp9: build
	./$(BINARY_HOST) \
	  --input  "$(SAMPLE)" \
	  --output "$(call _output,vp9)" \
	  --codec  vp9 \
	  --width  $(WIDTH) --height $(HEIGHT) \
	  --bitrate-kbps $(BITRATE_KBPS)

transcode-vvc: build
	./$(BINARY_HOST) \
	  --input  "$(SAMPLE)" \
	  --output "$(call _output,vvc)" \
	  --codec  vvc \
	  --width  $(WIDTH) --height $(HEIGHT) \
	  --bitrate-kbps $(BITRATE_KBPS)

# ─── benchmark multi-resolução ───────────────────────────────────────────────
benchmark:
	python3 scripts/run_resolution_benchmark.py \
	  --input      "$(SAMPLE)" \
	  --codec      $(CODEC) \
	  --report-dir $(REPORT_DIR) \
	  --output-dir $(OUTPUT_DIR) \
	  --runtime    $(RUNTIME)

benchmark-h264:
	$(MAKE) benchmark CODEC=h264

benchmark-h265:
	$(MAKE) benchmark CODEC=h265

benchmark-av1:
	$(MAKE) benchmark CODEC=av1

benchmark-vp9:
	$(MAKE) benchmark CODEC=vp9

benchmark-vvc:
	$(MAKE) benchmark CODEC=vvc

benchmark-all:
	$(MAKE) benchmark-h264
	$(MAKE) benchmark-h265
	$(MAKE) benchmark-av1
	$(MAKE) benchmark-vp9
	$(MAKE) benchmark-vvc

# ─── pipeline de referência (MP4 → Y4M) ─────────────────────────────────────
pipeline-reference:
	python3 scripts/build_reference_pipeline.py \
	  --input-dir  dataset/pre-selecao-top4-bitrate \
	  --output-dir outputs/reference-pipeline-preselecao-y4m

# ─── docker ──────────────────────────────────────────────────────────────────
docker-run: build-linux build-image
	INPUT="$(SAMPLE)" \
	OUTPUT="outputs/docker/$(notdir $(basename $(SAMPLE)))-$(CODEC)-$(HEIGHT)p.mp4" \
	WIDTH=$(WIDTH) HEIGHT=$(HEIGHT) BITRATE_KBPS=$(BITRATE_KBPS) \
	TRANSCODE_CODEC=$(CODEC) FFMPEG_PRESET=$(PRESET) \
	docker compose -f compose.yaml run --rm transcode-local

docker-run-av1: build-linux build-image
	INPUT="$(SAMPLE)" \
	OUTPUT="outputs/docker/$(notdir $(basename $(SAMPLE)))-av1-720p.mp4" \
	WIDTH=1280 HEIGHT=720 BITRATE_KBPS=3000 \
	TRANSCODE_CODEC=av1 FFMPEG_PRESET=$(PRESET) \
	docker compose -f compose.yaml run --rm transcode-local

docker-run-h264: build-linux build-image
	INPUT="$(SAMPLE)" \
	OUTPUT="outputs/docker/$(notdir $(basename $(SAMPLE)))-h264-720p.mp4" \
	WIDTH=1280 HEIGHT=720 BITRATE_KBPS=3000 \
	TRANSCODE_CODEC=h264 FFMPEG_PRESET=$(PRESET) \
	docker compose -f compose.yaml run --rm transcode-local

# ─── relatórios ──────────────────────────────────────────────────────────────
reports:
	@echo "=== Relatórios em $(REPORT_DIR)/ ==="
	@ls -lhrt $(REPORT_DIR)/ 2>/dev/null || echo "(nenhum relatório encontrado)"

# ─── limpeza ─────────────────────────────────────────────────────────────────
clean:
	rm -rf dist/ outputs/ coverage.out
