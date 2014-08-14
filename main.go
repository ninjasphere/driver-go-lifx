package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/ninjasphere/go-ninja"
	"github.com/wolfeidau/lifx"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {

	log.Infof("Starting " + drivername)

	conn, err := ninja.Connect("com.ninjablocks.lifx")
	if err != nil {
		log.FatalErrorf(err, "Could not connect to MQTT Broker")
	}

	bus, err := conn.AnnounceDriver("com.ninjablocks.lifx", drivername, getCurDir())
	if err != nil {
		log.FatalErrorf(err, "Could not get driver bus")
	}

	client := lifx.NewClient()
	log.Infof("Attempting to discover new lifx bulbs")
	err = client.StartDiscovery()
	if err != nil {
		log.HandleErrorf(err, "Can't discover bulbs")
	}

	go func() {

		sub := client.Subscribe()

		for {

			event := <-sub.Events

			switch bulb := event.(type) {
			case lifx.Bulb:
				if isUnique(&bulb) {
					log.Infof("creating new light")
					_, err := NewLight(bus, client, &bulb)
					if err != nil {
						log.HandleErrorf(err, "Error creating light instance")
					}
					seenlights = append(seenlights, &bulb) //TODO remove bulbs that haven't been seen in a while?
				}
			default:
				log.Infof("Event %v", event)
			}

		}

	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	// Block until a signal is received.
	s := <-c
	fmt.Println("Got signal:", s)
	return 0
}
