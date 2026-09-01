# Flagx

A Go command-line flag parsing library with environment variable support. Command-line flags take priority over environment variables.

## Features

- Built on top of the standard `flag` package, with consistent API and low learning curve
- Automatic environment variable support — env var names are derived from flag names
- Supports `--env.prefix` for a unified environment variable prefix
- Rich type support: Duration, Bytes, Text, arrays, and more
- Secret flag masking to prevent sensitive information leakage
- Required flag validation
- Post-parse callbacks (AfterParse)

## Installation

```bash
go get github.com/cloudfly/flagx
```

## Quick Start

```go
package main

import "github.com/cloudfly/flagx"

var (
    logFile    = flagx.NewString("log.file", "", "path to log file")
    logLevel   = flagx.NewBool("log.level", false, "enable debug logging")
    serverAddr = flagx.NewString("server.addr", ":7070", "HTTP server listen address")
    serverPort = flagx.NewInt("server.port", 8080, "TCP listen port")
)

func main() {
    flagx.Parse()
    // use *logFile, *logLevel, etc.
}
```

Usage:

```bash
# Command-line flags
./app --server.addr=:8080 --server.port=9090

# Environment variables
SERVER_ADDR=:8080 SERVER_PORT=9090 ./app

# Mixed usage (command-line takes priority)
SERVER_ADDR=:8080 ./app --server.port=9090
```

## Flag Types

### Basic Types

```go
// Boolean
debug := flagx.NewBool("debug", false, "enable debug mode")

// String
name := flagx.NewString("name", "default", "name of the service")

// Integer
port := flagx.NewInt("port", 8080, "listen port")
port64 := flagx.NewInt64("port64", 8080, "64-bit listen port")

// Float
ratio := flagx.NewFloat("ratio", 0.5, "ratio value")
```

### Duration

Supported time unit suffixes: `ms` (milliseconds), `s` (seconds), `m` (minutes), `h` (hours), `d` (days), `w` (weeks), `y` (years). Defaults to seconds when no suffix is provided.

Supports combined notation, e.g. `2h5m`, `1d6h`.

```go
timeout := flagx.NewDuration("timeout", "30s", "request timeout")
days := flagx.NewDuration("retention", "30d", "data retention period in days")
weeks := flagx.NewDuration("interval", "2w", "execution interval")
```

Usage:

```go
fmt.Println(timeout.Msecs) // milliseconds
```

### Bytes

Supported byte unit suffixes:

| Suffix | Meaning       | Base |
|--------|---------------|------|
| KB     | Kilobyte      | 1000 |
| MB     | Megabyte      | 1000 |
| GB     | Gigabyte      | 1000 |
| TB     | Terabyte      | 1000 |
| KiB    | Kibibyte      | 1024 |
| MiB    | Mebibyte      | 1024 |
| GiB    | Gibibyte      | 1024 |
| TiB    | Tebibyte      | 1024 |

```go
cacheSize := flagx.NewBytes("cache.size", 128, "cache size in bytes")
maxUpload := flagx.NewBytes("upload.max", 64, "max upload size", flagx.Env("MAX_UPLOAD_SIZE"))
```

Usage:

```go
fmt.Println(cacheSize.N)      // bytes as int64
fmt.Println(cacheSize.IntN()) // bytes as int (auto-truncated)
```

### Text

Used to hold complex content such as JSON or configuration text. The value passed in must be a base64-encoded string, which is automatically decoded during parsing.

```go
config := flagx.NewText("config", "", "configuration content, base64 encoded")
```

Usage:

```go
// Pass base64-encoded JSON via command line
// echo -n '{"key":"value"}' | base64
./app --config=eyJrZXkiOiJ2YWx1ZSJ9

fmt.Println(config.Value) // decoded original string: {"key":"value"}
```

### Array Types

Array types support two ways of passing values:

1. Specify the same flag multiple times: `--items=a --items=b --items=c`
2. Comma-separated: `--items=a,b,c`

Supported array types:

```go
// String array
tags := flagx.NewArrayString("tags", "list of tags")

// Integer array
ports := flagx.NewArrayInt("ports", "list of ports")

// Boolean array
flags := flagx.NewArrayBool("flags", "list of boolean flags")

// Duration array
timeouts := flagx.NewArrayDuration("timeouts", "list of timeout values")

// Bytes array
sizes := flagx.NewArrayBytes("sizes", "list of sizes")

// Text array
configs := flagx.NewArrayText("configs", "list of configurations")
```

Usage:

```go
for _, tag := range *tags {
    fmt.Println(tag)
}

// Convenience method: get value at index, return default if out of bounds
port := ports.GetOptionalArgOrDefault(0, 8080)
```

## Options

All `New*` functions accept a variadic last parameter `opts ...Option` for configuring additional flag behavior.

### Env — Custom Environment Variable Name

Customize the environment variable name. Multiple names can be specified, and they are tried in order from left to right:

```go
// Default env var name is LOG_FILE
logFile := flagx.NewString("log.file", "", "log file path")

// Custom env var name CUSTOM_LOG_FILE
logFile := flagx.NewString("log.file", "", "log file path", flagx.Env("CUSTOM_LOG_FILE"))

// Multiple env vars, tried in order
logFile := flagx.NewString("log.file", "", "log file path",
    flagx.Env("CUSTOM_LOG_FILE"),
    flagx.Env("SERVER_LOG_PATH"),
    flagx.Env("LOG_PATH"),
)
```

### Required — Required Flag

Marks the flag as required. It must be set via command line or environment variable, otherwise the program exits with an error:

```go
dsn := flagx.NewString("mysql.dsn", "", "MySQL connection string", flagx.Required())
```

### AfterParse — Post-Parse Callback

Sets a function to be called after flag parsing is complete. Useful for post-processing based on the values of other flags.

```go
addr := flagx.NewString("addr", "", "server address", flagx.AfterParse(func(isSet bool, set func(string) error) {
    if !isSet {
        // If addr is not set, set its value via the set function
        set(":8080")
    }
}))
```

## Environment Variable Mechanism

### Default Rules

By default, environment variable names are derived from flag names using the following rules:

- Replace `.` with `_`
- Convert to uppercase

Examples:
- `log.file` → `LOG_FILE`
- `server.port` → `SERVER_PORT`

### Unified Prefix

Use the `--env.prefix` flag to set a unified prefix for all environment variables:

```bash
./app --env.prefix=MYAPP_
```

With this prefix, the env var names become:
- `log.file` → `MYAPP_LOG_FILE`
- `server.port` → `MYAPP_SERVER_PORT`

### Precedence

Command-line flags > Environment variables > Default values

## Secret Flag Masking

flagx automatically masks flags whose names contain `pass`, `key`, `secret`, or `token` to prevent sensitive information from leaking in logs or monitoring output.

You can also manually register secret flags:

```go
flagx.RegisterSecretFlag("mysql.dsn")
```

When iterating via `Visit` and `VisitAll`, secret flag values are automatically masked (keeping the first and last 1/4 of characters, replacing the middle with `*`).

## Utility Functions

### Parse

Parses command-line flags and environment variables. Must be called before using any flags:

```go
flagx.Parse()
```

### FlagEnvName

Gets the environment variable name corresponding to a flag:

```go
envName := flagx.FlagEnvName("log.file")
// returns "LOG_FILE" by default
// returns "MYAPP_LOG_FILE" when --env.prefix=MYAPP_ is set
```

### Visit / VisitAll

```go
// Iterate over all flags explicitly set via command line
flagx.Visit(func(name, value string) {
    fmt.Printf("%s = %s\n", name, value)
})

// Iterate over all flags (including unset ones)
flagx.VisitAll(func(name, value string) {
    fmt.Printf("%s = %s\n", name, value)
})
```

### WriteFlags

Writes all explicitly set flags to an `io.Writer`:

```go
flagx.WriteFlags(os.Stdout)
```

### Usage

Prints usage information:

```go
flagx.Usage("My App v1.0.0")
```

When `-h` or `-help` is passed, detailed descriptions of all flags are printed; otherwise, only a hint to use `-help` is shown.

## Full Example

```go
package main

import (
    "fmt"
    "github.com/cloudfly/flagx"
)

var (
    // Basic types
    debug    = flagx.NewBool("debug", false, "enable debug mode")
    appName  = flagx.NewString("app.name", "demo", "application name")
    port     = flagx.NewInt("port", 8080, "listen port", flagx.Env("PORT"))

    // Duration
    timeout  = flagx.NewDuration("timeout", "30s", "request timeout")

    // Bytes
    maxBody  = flagx.NewBytes("http.maxBody", 1024*1024, "max request body size")

    // Array
    allowIPs = flagx.NewArrayString("security.allowIPs", "list of allowed IPs")

    // Required flag
    dsn      = flagx.NewString("mysql.dsn", "", "MySQL connection string", flagx.Required())
)

func main() {
    flagx.Parse()

    fmt.Printf("App: %s\n", *appName)
    fmt.Printf("Port: %d\n", *port)
    fmt.Printf("Debug: %v\n", *debug)
    fmt.Printf("Timeout: %dms\n", timeout.Msecs)
    fmt.Printf("Max Body: %d bytes\n", maxBody.N)
    fmt.Printf("Allow IPs: %v\n", *allowIPs)
}
```
