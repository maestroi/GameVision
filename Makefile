# GameVision local commands.
#
# Override anything:  make run ROM=/path/to/red.gb MODEL=Qwen3-VL-4B-Instruct
# Extra CLI flags:    make run ARGS='--paused --max-decisions 50'

GO      ?= go
BIN     ?= gamevision
CMD     := ./cmd/gamevision

# roms/ is gitignored. Prefer a checkout copy, then the shared PokePilot ROM.
ROM ?= $(firstword $(wildcard $(CURDIR)/roms/pokemon_red.gb $(HOME)/.config/pokepilot/pokemon_red.gb) $(CURDIR)/roms/pokemon_red.gb)
export POKEMON_RED_ROM ?= $(ROM)

BASE_URL   ?= http://localhost:8002/v1
MODEL      ?= Qwen3-VL-2B-Instruct
MODEL_2B   ?= Qwen3-VL-2B-Instruct
MODEL_4B   ?= Qwen3-VL-4B-Instruct
GOAL       ?= Start Pokémon Red and progress as far as possible.
SCENARIO   ?= scenarios/pokemon-red/title-screen.json
SCALE      ?= 2
HTTP       ?= localhost:8099
ARGS       ?=

.DEFAULT_GOAL := help

.PHONY: all build test vet fmt check run run-2b run-4b run-title \
	bench models models-switch models-unload models-2b models-4b models-restore \
	clean help

require-rom = @test -f "$(ROM)" || { \
	echo "ROM not found: $(ROM)"; \
	echo "set ROM=/path/to/pokemon-red.gb or POKEMON_RED_ROM"; \
	exit 1; \
}

all: build

build:
	$(GO) build -o $(BIN) $(CMD)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

check: vet test

run: build
	$(require-rom)
	./$(BIN) \
		--game pokemon-red \
		--rom "$(ROM)" \
		--base-url "$(BASE_URL)" \
		--model "$(MODEL)" \
		--goal "$(GOAL)" \
		--vision-scale $(SCALE) \
		--http "$(HTTP)" \
		$(ARGS)

run-2b:
	$(MAKE) run MODEL="$(MODEL_2B)"

run-4b:
	$(MAKE) run MODEL="$(MODEL_4B)"

run-title:
	$(MAKE) run ARGS='--scenario $(SCENARIO) $(ARGS)'

bench: build
	$(require-rom)
	./$(BIN) bench \
		--rom "$(ROM)" \
		--base-url "$(BASE_URL)" \
		--scenario "$(SCENARIO)" \
		--models "$(MODEL_2B),$(MODEL_4B)" \
		--vision-scale $(SCALE) \
		$(ARGS)

models: build
	./$(BIN) models --base-url "$(BASE_URL)" $(ARGS)

models-switch: build
	./$(BIN) models switch "$(MODEL)" --base-url "$(BASE_URL)" $(ARGS)

models-unload: build
	./$(BIN) models unload --base-url "$(BASE_URL)" $(ARGS)

models-2b:
	$(MAKE) models-switch MODEL="$(MODEL_2B)"

models-4b:
	$(MAKE) models-switch MODEL="$(MODEL_4B)"

models-restore: build
	./$(BIN) models switch qwen3.8-27b --base-url "$(BASE_URL)" $(ARGS)

clean:
	rm -f $(BIN)

help:
	@echo "GameVision"
	@echo "  make build           ./gamevision"
	@echo "  make test            go test ./..."
	@echo "  make check           vet + test"
	@echo "  make run             play Pokémon Red with MODEL"
	@echo "  make run-2b          run Qwen3-VL-2B-Instruct"
	@echo "  make run-4b          run Qwen3-VL-4B-Instruct"
	@echo "  make run-title       title-screen scenario"
	@echo "  make bench           2B vs 4B (unloads/loads between runs)"
	@echo "  make models          list server models"
	@echo "  make models-switch   unload current VLM, load MODEL"
	@echo "  make models-2b       switch to 2B"
	@echo "  make models-4b       switch to 4B (stops qwen38-solo)"
	@echo "  make models-restore  put qwen3.8-27b back on :8002"
	@echo "  make models-unload   unload whatever is in VRAM"
	@echo "  make clean           remove ./gamevision"
	@echo
	@echo "  ROM=$(ROM)"
	@echo "  BASE_URL=$(BASE_URL)"
	@echo "  MODEL=$(MODEL)"
	@echo "  GOAL=$(GOAL)"
	@echo "  ARGS=$(ARGS)"
