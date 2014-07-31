package main

import (
	"os"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	return run()
}
