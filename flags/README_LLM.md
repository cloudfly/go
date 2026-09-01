# Flagx

**flagx** extends the standard library `flag` package, it provide more convinient flag types and environment variables support.

# Features

1. support parse flag values from command line arguments and environment variables.
2. more rich flag types: bytes, duration, text, array of string, array of int.
3. support mark flag as required.
- string
- bool
- int
- duration
- bytes
- array of string
- array of int


# Example

```go
import (
	"github.com/cloudfly/flagx"
)

var (
	logFile    = flagx.NewString("log.file", "", "the log file")
	logLevel   = flagx.NewBool("log.level", false, "the log level, default is bool")
	serverHost = flagx.NewString("server.addr", ":7070", "the address http will serve on")
	serverPort = flagx.NewInt("server.port", 8080, "the tcp port will listen on")
	secs       = flagx.NewDuration("secs", "30s", "the duration flag, 30 secodns")
	bytes      = flagx.NewBytes("bytes", 128, "the bytes flag, 128 Byte")
	kbs        = flagx.NewArrayString("domain", "the domain array flag, default is empty")
)

func main() {
	flagx.Parse()
}

```

# Usage

`-h` or `--help` is builtin flag, when it is set, flagx will print usage and exit.

## Options

user can specify some options for each flag. such as:

```go
logFile := flagx.NewString("api.listen", "", "the log file", flagx.Env("LISTEN"), flagx.Required())
```

### Env(string)

customize environment variable name for flag. default is upper case of flag name and replace `.` with `_`.

such as `log.file` will be `LOG_FILE` in environment variables.

### Required()

mark flag as required. if flag is not set, it will print usage and exit.

### Secret()

mark flag as secret. 

## Basic Flags

### String
declare a string flag:
```go
flagx.NewString("log.file", "", "the log file")
```

more details see [flag.go](flag.go)

### Int
declare a int flag:
```go
flagx.NewInt("port", 8080, "the tcp port will listen on")
```
more details see [flag.go](flag.go)

### Int64
declare a int64 flag:
```go
flagx.NewInt64("bytes", 0, "the bytes flag, 0 Byte")
```
more details see [flag.go](flag.go)

### Float
declare a float flag:
```go
flagx.NewFloat("float", 0.0, "the float flag, 0.0")
```
more details see [flag.go](flag.go)

### Bool
declare a bool flag:
```go
flagx.NewBool("log.color", false, "the log color, default is false")
```
more details see [flag.go](flag.go)

### Bytes
declare a bytes flag. It supports the following optional suffixes for values: KB, MB, GB, TB, KiB, MiB, GiB, TiB.
```go
flagx.NewBytes("bytes", 0, "the bytes flag, 0 Byte")
```
more details see [bytes.go](bytes.go)

### Duration
declare a duration flag. It supports the following optional suffixes for values: s, m, h, d, w, M, y.
```go
flagx.NewDuration("retention", "30d", "the retention policy")
```
more details see [duration.go](duration.go)

### Text

Text is a flag that used to hold complex content, such as json.
the passed value should be base64-encoded string, it will be decoded before assignment.

```go
flagx.NewText("text", "", "the text flag, default is empty")
```
more details see [text.go](text.go)

## Array Flags

you can define a array of string flag, which let user use `-flag value1 -flag value2` or `-flag value1,value2` to pass multiple values.

### Array of String
declare a array of string flag:
```go
flagx.NewArrayString("domain", "the domain array flag, default is empty")
```

user can pass multiple values for array of string flag:
```
--domain example.com --domain example.org
```

### Array of Int
declare a array of int flag:
```go
flagx.NewArrayInt("port", "the port array flag, default is empty")
```

user can pass multiple values for array of int flag:
```
--port 8080 --port 8081
```

### Array of Duration
declare a array of duration flag. It supports the following optional suffixes for values: s, m, h, d, w, M, y.
```go
flagx.NewArrayDuration("retention", "the retention policy")
```

### Array of Bytes
declare a array of bytes flag. It supports the following optional suffixes for values: KB, MB, GB, TB, KiB, MiB, GiB, TiB.
```go
flagx.NewArrayBytes("bytes", "the bytes flag, 0 Byte")
```
user can pass multiple values for array of bytes flag:
```
--bytes 128 --bytes 64KB
```

### Array of Bool
declare a array of bool flag:
```go
flagx.NewArrayBool("bool", "the bool flag, false")
```
user can pass multiple values for array of bool flag:
```
--bool true --bool false
```

### Array of text
declare a array of text flag:
```go
flagx.NewArrayText("text", "the text flag, default is empty")
```
user can pass multiple values for array of text flag:
```
--text aGVsbG8= --text aGVsbG93b3JsZQ==
```

## Utils

### Visit()
visit flags that has been set.
```go
import (
	"fmt"
	"flag"
)

flagx.Visit(func(f *flag.Flag) {
	fmt.Printf("%s: %v\n", f.Name, f.Value)
})
```


### VisitAll()
visit all flags, no matter it is set or not.
```go
import (
	"fmt"
	"flag"
)

flagx.VisitAll(func(f *flag.Flag) {
	fmt.Printf("%s: %v\n", f.Name, f.Value)
})
```