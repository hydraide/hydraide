# Makefile for the HydrAIDE Go example tree.
#
# `make up`              — start the local HydrAIDE via docker compose.
# `make down`            — stop and remove the local HydrAIDE.
# `make quickstart`      — run the minimal connect/save/load/subscribe demo.
# `make recipe-<name>`   — run a focused recipe (see 02-recipes/).
# `make test-examples`   — run every example's integration tests.
#
# The targets honour HYDRA_HOST and HYDRA_CERT from .env.local if it exists,
# otherwise fall back to .env.example (the in-tree compose defaults).

SHELL := /bin/bash

ENV_FILE := $(if $(wildcard .env.local),.env.local,.env.example)

define run_with_env
	set -a; source $(ENV_FILE); set +a; $(1)
endef

.PHONY: up down quickstart test-examples \
	recipe-ttl-queue recipe-atomic-patch recipe-live-subscribe \
	recipe-profile-per-user recipe-catalog-list recipe-atomic-counter \
	recipe-business-lock recipe-advanced-filters \
	app-todo-api app-url-shortener app-multi-tenant-saas

up:
	docker compose up -d
	@echo "waiting for hydraide health"
	@for i in $$(seq 1 30); do \
		state=$$(docker inspect -f '{{.State.Health.Status}}' hydraide-examples 2>/dev/null || echo starting); \
		if [ "$$state" = "healthy" ]; then echo "hydraide is healthy"; exit 0; fi; \
		sleep 1; \
	done; \
	echo "hydraide did not become healthy in time"; exit 1

down:
	docker compose down -v

quickstart:
	$(call run_with_env, cd 01-quickstart && go run .)

recipe-ttl-queue:
	$(call run_with_env, cd 02-recipes/ttl-queue && go run .)

recipe-atomic-patch:
	$(call run_with_env, cd 02-recipes/atomic-patch && go run .)

recipe-live-subscribe:
	$(call run_with_env, cd 02-recipes/live-subscribe && go run .)

recipe-profile-per-user:
	$(call run_with_env, cd 02-recipes/profile-per-user && go run .)

recipe-catalog-list:
	$(call run_with_env, cd 02-recipes/catalog-list && go run .)

recipe-atomic-counter:
	$(call run_with_env, cd 02-recipes/atomic-counter && go run .)

recipe-business-lock:
	$(call run_with_env, cd 02-recipes/business-lock && go run .)

recipe-advanced-filters:
	$(call run_with_env, cd 02-recipes/advanced-filters && go run .)

app-todo-api:
	$(call run_with_env, cd 03-reference-apps/todo-api && go run .)

app-url-shortener:
	$(call run_with_env, cd 03-reference-apps/url-shortener && go run .)

app-multi-tenant-saas:
	$(call run_with_env, cd 03-reference-apps/multi-tenant-saas && go run .)

test-examples:
	$(call run_with_env, go test -tags=integration -count=1 ./...)
