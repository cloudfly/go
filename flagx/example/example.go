package main

import "github.com/cloudfly/go/flagx"

var (
	logFile    = flagx.NewString("log.file", "", "the log file")
	logLevel   = flagx.NewBool("log.level", false, "the log level, default is bool")
	serverHost = flagx.NewString("server.addr", ":7070", "the address http will serve on")
	serverPort = flagx.NewInt("server.port", 8080, "the tcp port will listen on")
	secs       = flagx.NewDuration("secs", "30s", "the duration flag, 30 secodns")
	secs2      = flagx.NewDuration("days", "30d", "the duration flag, 30 days")
	secs3      = flagx.NewDuration("weeks", "2w", "the duration flag, 2 weeks")
	bytes      = flagx.NewBytes("bytes", 128, "the bytes flag, 128 Byte")
	kbs        = flagx.NewBytes("kbytes", 64, "the bytes flag, 64KB")
	mbs        = flagx.NewBytes("mbytes", 64, "the bytes flag, 64KB")
	gbs        = flagx.NewBytes("gbytes", 64, "the bytes flag, 64KB")
	strs       = flagx.NewArrayString("array.str", "the string array flag, default is empty")
	ints       = flagx.NewArrayInt("array.int", "the string array flag, default is empty")
)

func main() {
	flagx.Parse()
	println(*kbs)
}
