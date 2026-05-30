// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	qt "github.com/frankban/quicktest"
)

// newTestHTTPClient starts an initialized Streamable HTTP MCP client against
// the given test server. Each call performs its own initialize, so in stateful
// mode each returned client has a distinct MCP session.
func newTestHTTPClient(ctx context.Context, t *testing.T, url string) *client.Client {
	t.Helper()
	c, err := client.NewStreamableHttpClient(url)
	qt.Assert(t, err, qt.IsNil)
	t.Cleanup(func() { _ = c.Close() })

	err = c.Start(ctx)
	qt.Assert(t, err, qt.IsNil)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.0.0"}
	_, err = c.Initialize(ctx, initReq)
	qt.Assert(t, err, qt.IsNil)
	return c
}

// evalViaHTTP calls the eval tool and returns (text, isError).
func evalViaHTTP(ctx context.Context, t *testing.T, c *client.Client, code string) (string, bool) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = "eval"
	req.Params.Arguments = map[string]any{"code": code}
	res, err := c.CallTool(ctx, req)
	qt.Assert(t, err, qt.IsNil)
	qt.Assert(t, len(res.Content), qt.Not(qt.Equals), 0)
	tc, ok := mcp.AsTextContent(res.Content[0])
	qt.Assert(t, ok, qt.IsTrue)
	return tc.Text, res.IsError
}

// The eval tool must work over the Streamable HTTP transport.
func TestHTTP_EvalReturnsResult(t *testing.T) {
	ctx := context.Background()
	ms := &mcpServer{}
	// Cleanups run LIFO after client cleanups (registered later in
	// newTestHTTPClient), so clients close before the server and engines.
	t.Cleanup(ms.closeAll)

	httpSrv, err := ms.newStreamableHTTPServer()
	qt.Assert(t, err, qt.IsNil)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	c := newTestHTTPClient(ctx, t, ts.URL)
	text, isErr := evalViaHTTP(ctx, t, c, "(+ 1 2)")
	qt.Assert(t, isErr, qt.IsFalse)
	qt.Assert(t, text, qt.Equals, "3")
}

// State defined in one client's session must not be visible in another's —
// the per-session engine isolation guarantee.
func TestHTTP_SessionStateIsolation(t *testing.T) {
	ctx := context.Background()
	ms := &mcpServer{}
	t.Cleanup(ms.closeAll)

	httpSrv, err := ms.newStreamableHTTPServer()
	qt.Assert(t, err, qt.IsNil)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	a := newTestHTTPClient(ctx, t, ts.URL)
	b := newTestHTTPClient(ctx, t, ts.URL)

	// Define a binding in A and confirm A sees it.
	_, isErr := evalViaHTTP(ctx, t, a, "(define wile-goast-test-x 42)")
	qt.Assert(t, isErr, qt.IsFalse)
	textA, isErr := evalViaHTTP(ctx, t, a, "wile-goast-test-x")
	qt.Assert(t, isErr, qt.IsFalse)
	qt.Assert(t, textA, qt.Equals, "42")

	// B is a separate session: the binding must be unbound there.
	_, isErrB := evalViaHTTP(ctx, t, b, "wile-goast-test-x")
	qt.Assert(t, isErrB, qt.IsTrue)
}
