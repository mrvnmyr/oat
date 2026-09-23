package common

import (
	"log"
	"os"
)

var DebugFlag bool = false

func debugEnabled() bool {
	return DebugFlag || os.Getenv("DEBUG") != ""
}

func Debug(args ...any) {
	if debugEnabled() {
		log.Println(args...)
	}
}

func Debugf(args ...any) {
	fmtStr := args[0]
	fmt, ok := fmtStr.(string)
	if !ok {
		panic(ok)
	}

	args = args[1:]
	if debugEnabled() {
		log.Printf(fmt, args...)
	}
}
