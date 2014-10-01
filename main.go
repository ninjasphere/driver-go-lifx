package main

import (
	"fmt"
	"os"
	"os/signal"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {

	NewLifxDriver()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	// Block until a signal is received.
	s := <-c
	fmt.Println("Got signal:", s)
	return 0
}
