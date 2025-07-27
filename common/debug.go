package common

import "log"

var DebugFlag bool = false

func Debug(args ...any) {
	if DebugFlag {
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
	if DebugFlag {
		log.Printf(fmt, args...)
	}
}
