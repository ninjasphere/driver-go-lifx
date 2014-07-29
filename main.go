package main

import (
	"os"
	"github.com/ninjasphere/go-ninja"

)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	return run()
}
