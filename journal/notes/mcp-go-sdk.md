# Go SDK for an Anam MCP adapter

Research date: 2026-10-06. This is an adapter recommendation, not an implementation commitment. Sources below describe the current upstream documentation; pin a released SDK version and verify its client compatibility before implementing.

The official Go SDK is a suitable transport boundary. Its `mcp` package supplies server/client APIs, while separate packages handle OAuth primitives. Upstream currently declares Go 1.25.0, matching Anam's `go.mod`. [SDK overview](https://github.com/modelcontextprotocol/go-sdk), [upstream go.mod](https://github.com/modelcontextprotocol/go-sdk/blob/main/go.mod).

## Relevant capabilities

- **Typed tools:** generic `mcp.AddTool` infers JSON schemas from Go types, validates arguments and outputs, and converts ordinary handler errors to tool errors. Typed output becomes `StructuredContent` and text content. Use explicit snake_case DTOs so transport schemas do not become domain types. [Server tools](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/server.md#tools).
- **Resources:** individual URIs and URI templates have registered read handlers. This fits stable projection definitions and receipt-addressed artifacts. URI matching is routing, not application authorization. [Resources](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/server.md#resources).
- **Transports:** `StdioTransport` carries newline-delimited JSON over stdin/stdout; `NewStreamableHTTPHandler` supplies HTTP transport. Start with stdio for a local process and reserve stdout for protocol traffic. HTTP is a separate deployment decision. [Transport API](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#StdioTransport), [HTTP handler](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#NewStreamableHTTPHandler).
- **Testing:** `NewInMemoryTransports` connects a real SDK server and client, with the server connected first. Use this with Anam's real temporary ledger and operational files to test discovery, schema validation, tool results, and cancellation without networking or protocol mocks. [In-memory transport](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#NewInMemoryTransports).
- **Cancellation:** context cancellation terminates an RPC and sends peer cancellation; the RPC returning does not guarantee the peer has observed it. Handlers must propagate context and enforce server-side deadlines. Cancellation cannot promise rollback of an already completed publication. [Protocol cancellation](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md#cancellation).

## Recommended first slice

Keep the SDK in the adapter and call the existing application operations. Offer bounded projection discovery and receipt-backed projection retrieval, with receipt inspection/verification as follow-ups. Return the projection data with its actual provenance and watermark. Preserve the distinction between a whole receipted projection and a displayed page: do not claim an existing whole-projection receipt proves the exact bytes of an independently paginated response.

This recommendation follows Anam's boundary: the MCP layer translates requests and results; the core owns selection, historical meaning, receipts, and gate invariants. Avoid arbitrary SQL, generic filesystem access, and a second implementation of the fold. Mutation tools are unnecessary to establish the initial utility.

## Caveats before implementation

The SDK handles protocol schemas, not Anam's domain constraints or response-size policy. Explicitly bound query inputs and output size, retain whole facets, and return errors rather than silently truncating evidence. These are adapter design recommendations.

HTTP configuration is version-sensitive: current documentation says the 2026-07-28 protocol requires stateless HTTP; legacy protocol versions retain session behavior. Stateless mode restricts server-initiated requests. Select the actual target clients before adding HTTP, subscriptions, or elicitation. [HTTP options](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#StreamableHTTPOptions), [protocol support](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#StreamableServerTransport.SupportsProtocolVersion).
