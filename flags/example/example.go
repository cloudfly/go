package main

import "github.com/cloudfly/go/flags"

var (
	logFile    = flags.NewString("log.file", "", "the log file")
	logLevel   = flags.NewBool("log.level", false, "the log level, default is bool")
	serverHost = flags.NewString("server.addr", ":7070", "the address http will serve on")
	serverPort = flags.NewInt("server.port", 8080, "the tcp port will listen on")
	secs       = flags.NewDuration("secs", "30s", "the duration flag, 30 secodns")
	secs2      = flags.NewDuration("days", "30d", "the duration flag, 30 days")
	secs3      = flags.NewDuration("weeks", "2w", "the duration flag, 2 weeks")
	bytes      = flags.NewBytes("bytes", 128, "the bytes flag, 128 Byte")
	kbs        = flags.NewBytes("kbytes", 64, "the bytes flag, 64KB")
	mbs        = flags.NewBytes("mbytes", 64, "the bytes flag, 64KB")
	gbs        = flags.NewBytes("gbytes", 64, "the bytes flag, 64KB")
	strs       = flags.NewArrayString("array.str", "the string array flag, default is empty")
	ints       = flags.NewArrayInt("array.int", "the string array flag, default is empty")
)

func main() {
	flags.Parse()
	println(*kbs)
}
