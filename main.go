package main

import (
	"os"
	"github.com/ninjasphere/go-ninja"

)

var drivername = "driver-lifx"
var log = ninja.GetLogger(drivername)

func main() {

	os.Exit(realMain())
}

func realMain() int {
	// Get the command line args. We shortcut "--version" and "-v" to
	// just show the version.
	args := os.Args[1:]
	for _, arg := range args {
		if arg == "-v" || arg == "--version" {
			newArgs := make([]string, len(args)+1)
			newArgs[0] = "version"
			copy(newArgs[1:], args)
			args = newArgs
			break
		}
	}
	exitcode := run()
	return exitcode
}
