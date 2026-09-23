# sutl-go

Dependency-free Go implementation of [sUTL](https://sutl-bronzearch.house-of-ur.com/) 1.0, the sUTL Universal Transform Language (“subtle”).

This module implements the frozen [sUTL 1.0 contract](https://github.com/emlynoregan/sutl-language). It passes the 88-case conformance corpus and the 44 archived Studio fixtures.

## Install

Go modules are published from this public repository. A version tag is enough for `go get` and [pkg.go.dev](https://pkg.go.dev/github.com/emlynoregan/sutl-go).

```powershell
go get github.com/emlynoregan/sutl-go@v1.0.0
```

The command-line tool:

```powershell
go install github.com/emlynoregan/sutl-go/cmd/sutl@v1.0.0
```

Requires Go 1.22 or later.

## Evaluate

```go
package main

import (
	"fmt"

	"github.com/emlynoregan/sutl-go"
)

func main() {
	source := map[string]any{"name": "Ada"}
	fmt.Println(sutl.Evaluate(source, "^$.name", nil)) // Ada

	// Compile once when the same transform runs against many sources.
	program := sutl.Compile("^$.name", nil)
	fmt.Println(program.Run(source)) // Ada
}
```

`sutl.DecodeJSON` reads a MLSNBN value and keeps JSON object key order. Path wildcards (`*` and `**`) walk that order, matching the JavaScript and Python implementations and the Studio fixtures. `keys` and `values` still return keys in sorted order.

Maps built as `map[string]any` do not remember insertion order. Decode JSON when wildcard order matters.

Timings for `Evaluate` and `Compile` are in [PERFORMANCE.md](PERFORMANCE.md).

## CLI

Arguments are JSON literals. Prefix a path with `@`, or use `-` for stdin.

```powershell
sutl "{\"name\":\"Ada\"}" "\"^$.name\""
```

`--library` is a JSON object. `--pretty` indents the result. `--version` prints the package version.

## License

Apache-2.0. See [LICENSE](LICENSE).
