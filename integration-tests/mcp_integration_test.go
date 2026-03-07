//go:build integration && mcp

package integration_tests_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/drewjocham/mongork/internal/cli"
	"github.com/drewjocham/mongork/internal/jsonutil"
	"github.com/stretchr/testify/require"
)

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type mcpClient struct {
	t   *testing.T
	enc *jsonutil.Encoder
	dec *jsonutil.Decoder
}

func (c *mcpClient) call(method string, id int, params interface{}, target interface{}) {
	c.t.Helper()
	if err := c.enc.Encode(rpcRequest{"2.0", id, method, params}); err != nil {
		c.t.Fatalf("rpc encode failed: %v", err)
	}

	var resp rpcResponse
	if err := c.dec.Decode(&resp); err != nil {
		c.t.Fatalf("rpc decode failed: %v", err)
	}

	if resp.Error != nil {
		c.t.Fatalf("rpc error [%s]: %s (code: %d)", method, resp.Error.Message, resp.Error.Code)
	}

	if target != nil {
		raw, err := jsonutil.Marshal(resp.Result)
		if err != nil {
			c.t.Fatalf("failed to marshal rpc result: %v", err)
		}
		if err := jsonutil.Unmarshal(raw, target); err != nil {
			c.t.Fatalf("failed to unmarshal rpc result: %v", err)
		}
	}
}

func (c *mcpClient) callExpectError(method string, id int, params interface{}) (int, string) {
	c.t.Helper()
	if err := c.enc.Encode(rpcRequest{"2.0", id, method, params}); err != nil {
		c.t.Fatalf("rpc encode failed: %v", err)
	}

	var resp rpcResponse
	if err := c.dec.Decode(&resp); err != nil {
		c.t.Fatalf("rpc decode failed: %v", err)
	}

	if resp.Error == nil {
		c.t.Fatalf("expected rpc error for method %s, got none", method)
	}

	return resp.Error.Code, resp.Error.Message
}

func parseToolText(t *testing.T, res struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool returned empty content")
	}
	return res.Content[0].Text
}

func toolCallParams(name string, args map[string]any) map[string]any {
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	return params
}

func TestCLIMCPIntegration(t *testing.T) {
	env := setupIntegrationEnv(t, context.Background())

	env.RunCLI(t, "up")

	client, stopServer := startCLIMCPServer(t, env)
	t.Cleanup(stopServer)

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "Initialize",
			run: func(t *testing.T) {
				client.t = t
				client.call("initialize", 1, map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"clientInfo":      map[string]string{"name": "test-client", "version": "1.0"},
				}, nil)
			},
		},
		{
			name: "ListTools",
			run: func(t *testing.T) {
				client.t = t
				var res struct {
					Tools []struct {
						Name string `json:"name"`
					}
				}
				client.call("tools/list", 2, nil, &res)

				found := make(map[string]bool)
				for _, tool := range res.Tools {
					found[tool.Name] = true
				}

				for _, name := range []string{
					"migration_status",
					"migration_plan",
					"migration_up",
					"migration_down",
					"database_schema",
				} {
					if !found[name] {
						t.Errorf("missing tool: %s", name)
					}
				}
			},
		},
		{
			name: "Status shows applied migrations",
			run: func(t *testing.T) {
				client.t = t
				var res struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				}
				client.call("tools/call", 3, toolCallParams("migration_status", nil), &res)

				text := parseToolText(t, res)
				if !strings.Contains(text, "✓") && !strings.Contains(text, "✅") {
					t.Errorf("unexpected status output: %s", text)
				}
			},
		},
		{
			name: "Plan output and required down argument validation",
			run: func(t *testing.T) {
				client.t = t
				var planRes struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				}
				client.call("tools/call", 4, toolCallParams("migration_plan", nil), &planRes)
				planText := parseToolText(t, planRes)
				require.NotEmpty(t, strings.TrimSpace(planText))
				_, msg := client.callExpectError("tools/call", 5, toolCallParams("migration_down", nil))
				require.Contains(t, strings.ToLower(msg), "version is required")
			},
		},
		{
			name: "Schema output for recommendations",
			run: func(t *testing.T) {
				client.t = t
				var res struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				}
				client.call("tools/call", 6, toolCallParams("database_schema", nil), &res)

				text := parseToolText(t, res)
				require.Contains(t, text, "Database Schema")
				require.Contains(t, text, "Collection:")
			},
		},
	}

	for _, step := range steps {
		t.Run(step.name, step.run)
	}
}

func startCLIMCPServer(t *testing.T, env *TestEnv) (*mcpClient, func()) {
	t.Helper()

	clientToSrvR, clientToSrvW, err := os.Pipe()
	require.NoError(t, err)
	srvToClientR, srvToClientW, err := os.Pipe()
	require.NoError(t, err)

	oldArgs := os.Args
	oldIn := os.Stdin
	oldOut := os.Stdout

	os.Args = []string{"mt", "--config", env.ConfigPath, "mcp"}
	os.Stdin = clientToSrvR
	os.Stdout = srvToClientW

	errChan := make(chan error, 1)
	go func() { errChan <- cli.Execute() }()

	client := &mcpClient{
		t:   t,
		enc: jsonutil.NewEncoder(clientToSrvW),
		dec: jsonutil.NewDecoder(srvToClientR),
	}

	stop := func() {
		_ = clientToSrvW.Close()
		_ = srvToClientR.Close()
		_ = srvToClientW.Close()
		_ = clientToSrvR.Close()

		os.Args = oldArgs
		os.Stdin = oldIn
		os.Stdout = oldOut

		select {
		case err := <-errChan:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatalf("mcp server did not stop")
		}
	}

	return client, stop
}
