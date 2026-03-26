ifndef MAKEFILES_LOCAL_MK
MAKEFILES_LOCAL_MK := 1

THIS_MK      := $(abspath $(lastword $(MAKEFILE_LIST)))
MAKEFILES_DIR := $(dir $(THIS_MK))
REPO_ROOT    := $(abspath $(MAKEFILES_DIR)/..)

include $(MAKEFILES_DIR)/variables/vars.mk

# ── Configurable defaults
LOCAL_MONGO_CONTAINER ?= mongork-local
LOCAL_MONGO_PORT      ?= 27017
LOCAL_MONGO_DATABASE  ?= stackit
LOCAL_MONGO_URL       ?= mongodb://localhost:$(LOCAL_MONGO_PORT)
LOCAL_MCP_LISTEN      ?= 127.0.0.1:8080
LOCAL_MCP_BASE        ?= /mcp
LOCAL_PID_FILE        ?= $(REPO_ROOT)/.local.pids

.PHONY: local-db local-db-down local-mcp local-mcp-stop \
        local-desktop local-all local-down local-status local-info

# ── Helpers ──────────────
define PRINT_BOX
	@echo "$(GREEN)┌┐$(NC)"
	@echo "$(GREEN)│$(NC)  $1"
	@echo "$(GREEN)└┘$(NC)"
endef

define PRINT_CONFIG
	@echo ""
	@echo "$(GREEN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(NC)"
	@echo "$(GREEN) Local Dev Configuration$(NC)"
	@echo "$(GREEN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(NC)"
	@echo ""
	@echo "  $(YELLOW)MongoDB$(NC)"
	@echo "    URL      :  $(LOCAL_MONGO_URL)"
	@echo "    Database :  $(LOCAL_MONGO_DATABASE)"
	@echo "    Username :  (none — no auth)"
	@echo "    Password :  (none — no auth)"
	@echo ""
	@echo "  $(YELLOW)MCP Server$(NC)"
	@echo "    Endpoint :  http://$(LOCAL_MCP_LISTEN)$(LOCAL_MCP_BASE)"
	@echo "    Transport:  http"
	@echo ""
	@echo "  $(YELLOW)Desktop App — enter these in the Connection tab$(NC)"
	@echo "    URL      :  $(LOCAL_MONGO_URL)"
	@echo "    Database :  $(LOCAL_MONGO_DATABASE)"
	@echo "    Username :  (leave blank)"
	@echo "    Password :  (leave blank)"
	@echo ""
	@echo "$(GREEN)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(NC)"
	@echo ""
endef

# ── Database ─────────────
local-db: ## Start a local MongoDB container on port 27017 (no auth)
	@echo "$(GREEN)Starting local MongoDB container '$(LOCAL_MONGO_CONTAINER)'...$(NC)"
	@docker start $(LOCAL_MONGO_CONTAINER) 2>/dev/null \
	  || docker run --name $(LOCAL_MONGO_CONTAINER) \
	       -p $(LOCAL_MONGO_PORT):27017 \
	       -d mongo:8.0 --bind_ip_all
	@echo -n "  Waiting for MongoDB to be ready "
	@for i in $$(seq 1 30); do \
	  docker exec $(LOCAL_MONGO_CONTAINER) \
	    mongosh --quiet --eval "db.adminCommand('ping').ok" 2>/dev/null | grep -q "1" \
	  && break || (printf "."; sleep 1); \
	done
	@echo " ready."
	@echo ""
	@echo "  $(GREEN)MongoDB is up$(NC)"
	@echo "  URL      : $(LOCAL_MONGO_URL)"
	@echo "  Database : $(LOCAL_MONGO_DATABASE)"
	@echo "  Auth     : none"

local-db-down: ## Stop and remove the local MongoDB container
	@echo "$(YELLOW)Stopping MongoDB container '$(LOCAL_MONGO_CONTAINER)'...$(NC)"
	@docker rm -f $(LOCAL_MONGO_CONTAINER) 2>/dev/null || true
	@echo "  done."

# ── MCP server ───────────
local-mcp: build ## Build + start the MCP HTTP server (background) pointing at local MongoDB
	@echo "$(GREEN)Starting MCP server...$(NC)"
	@mkdir -p $$(dirname $(LOCAL_PID_FILE))
	@MONGO_URL="$(LOCAL_MONGO_URL)" \
	 MONGO_DATABASE="$(LOCAL_MONGO_DATABASE)" \
	 $(BUILD_DIR)/$(BINARY_NAME) mcp \
	   --transport http \
	   --listen $(LOCAL_MCP_LISTEN) \
	   --base-path $(LOCAL_MCP_BASE) \
	 > /tmp/mongork-mcp.log 2>&1 & \
	 MCP_PID=$$!; \
	 echo "$$MCP_PID" >> $(LOCAL_PID_FILE); \
	 sleep 1; \
	 if kill -0 $$MCP_PID 2>/dev/null; then \
	   echo "  $(GREEN)MCP server running$(NC)  (PID $$MCP_PID)"; \
	   echo "  Endpoint : http://$(LOCAL_MCP_LISTEN)$(LOCAL_MCP_BASE)"; \
	   echo "  Log file : /tmp/mongork-mcp.log"; \
	 else \
	   echo "  $(RED)MCP server failed to start — check /tmp/mongork-mcp.log$(NC)"; \
	   exit 1; \
	 fi

local-mcp-stop: ## Stop the background MCP server
	@echo "$(YELLOW)Stopping MCP server...$(NC)"
	@if [ -f "$(LOCAL_PID_FILE)" ]; then \
	  while read pid; do \
	    kill "$$pid" 2>/dev/null && echo "  killed PID $$pid" || true; \
	  done < "$(LOCAL_PID_FILE)"; \
	  rm -f "$(LOCAL_PID_FILE)"; \
	fi
	@echo "  done."

# ── Desktop app ──────────
local-desktop: ## Start the desktop app in Wails dev mode
	@which wails > /dev/null 2>&1 \
	  || (echo "$(RED)wails not found. Install: go install github.com/wailsapp/wails/v2/cmd/wails@latest$(NC)" && exit 1)
	$(PRINT_CONFIG)
	@echo "$(GREEN)Starting desktop app (Wails dev mode)...$(NC)"
	@echo "$(YELLOW)Hot-reload is active. Press Ctrl+C to stop.$(NC)"
	@echo ""
	cd $(REPO_ROOT)/desktop-app && wails dev

# ── All-in-one ───────────
local-all: local-db local-mcp ## Start MongoDB + MCP (background) then launch the desktop app
	$(PRINT_CONFIG)
	@echo "$(GREEN)Launching desktop app...$(NC)"
	@echo "$(YELLOW)Ctrl+C stops the desktop. Run 'make local-down' afterwards to clean up.$(NC)"
	@echo ""
	@cd $(REPO_ROOT)/desktop-app && wails dev

# ── Teardown ─────────────
local-down: local-mcp-stop local-db-down ## Stop all local services (MCP + MongoDB)
	@echo "$(GREEN)All local services stopped.$(NC)"

# ── Status / info ────────
local-status: ## Show status of all local dev services
	@echo ""
	@echo "$(GREEN)Local service status$(NC)"
	@echo "─────────────────────────────────────"
	@printf "  %-12s" "MongoDB:"; \
	  STATUS=$$(docker inspect -f '{{.State.Status}}' $(LOCAL_MONGO_CONTAINER) 2>/dev/null); \
	  if [ "$$STATUS" = "running" ]; then \
	    echo "$(GREEN)running$(NC)  ($(LOCAL_MONGO_URL))"; \
	  elif [ -n "$$STATUS" ]; then \
	    echo "$(YELLOW)$$STATUS$(NC)"; \
	  else \
	    echo "$(RED)not found$(NC)"; \
	  fi
	@printf "  %-12s" "MCP:"; \
	  if [ -f "$(LOCAL_PID_FILE)" ]; then \
	    ALIVE=0; \
	    while read pid; do \
	      kill -0 "$$pid" 2>/dev/null && ALIVE=1 && echo "$(GREEN)running$(NC)  (PID $$pid — http://$(LOCAL_MCP_LISTEN)$(LOCAL_MCP_BASE))" && break; \
	    done < "$(LOCAL_PID_FILE)"; \
	    [ "$$ALIVE" -eq 0 ] && echo "$(RED)stopped$(NC) (stale PID file)"; \
	  else \
	    echo "$(RED)not running$(NC)"; \
	  fi
	@echo ""

local-info: ## Print all connection strings and credentials for copy-paste
	$(PRINT_CONFIG)

endif # MAKEFILES_LOCAL_MK