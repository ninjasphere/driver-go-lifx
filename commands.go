package main

import (
	"github.com/ninjasphere/go-ninja"
)

func run() int {
	drivername := "driver-go-lifx"
	log := ninja.GetLogger(drivername)
	log.Debugf("Starting " + drivername)

	return 0
}
