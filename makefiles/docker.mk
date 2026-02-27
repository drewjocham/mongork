THIS_MK := $(abspath $(lastword $(MAKEFILE_LIST)))
MAKEFILES_DIR := $(dir $(THIS_MK))
REPO_ROOT := $(abspath $(MAKEFILES_DIR)/..)

include $(MAKEFILES_DIR)/variables/vars.mk

DOCKER_TAG ?= v0.1.0
COMPOSE_CMD ?= docker compose

.PHONY: help docker-build docker-up up-build docker-down start security-scan db-up db-down

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

docker-build:
	@echo "$(GREEN)Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)...$(NC)"
	$(COMPOSE_CMD) --profile build build

up-build: docker-build
	@echo "$(GREEN)Running Docker container via profile...$(NC)"
	$(COMPOSE_CMD) --profile build up -d

up:
	@echo "$(GREEN)Starting services...$(NC)"
	$(COMPOSE_CMD) --profile build service up -d --remove-orphans
down:
	@echo "$(YELLOW)Stopping services...$(NC)"
	$(COMPOSE_CMD) down --remove-orphans -v

start:
	@echo "$(GREEN)Running Docker container manually...$(NC)"
	docker run --rm -it \
	   -e "MONGO_URL=$(MONGO_URL)" \
	   -e "MDB_MCP_CONNECTION_STRING=$(MDB_MCP_CONNECTION_STRING)" \
	   $(DOCKER_IMAGE):$(DOCKER_TAG)

security-scan:
	@echo "$(GREEN)Running security scan...$(NC)"
	docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
	   -v $(shell pwd):/src aquasec/trivy image $(DOCKER_IMAGE):$(DOCKER_TAG)

db-up:
	@echo "$(GREEN)Starting local MongoDB...$(NC)"
	@docker start mongork-test 2>/dev/null || \
	docker run --name mongork-test -p 27017:27017 -d mongo:8.0

db-down:
	@echo "$(YELLOW)Removing local MongoDB...$(NC)"
	@docker rm -f mongork-test || true
