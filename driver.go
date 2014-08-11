package main

import (
	"fmt"
	"math"
	"time"

	"os"
	"os/signal"

	"github.com/bitly/go-simplejson"
	"github.com/davecgh/go-spew/spew"
	"github.com/ninjasphere/go-ninja"
	"github.com/ninjasphere/go-ninja/logger"
	"github.com/wolfeidau/lifx"
)

var drivername = "driver-lifx"
var log = logger.GetLogger(drivername)
var seenlights []*lifx.Bulb

type Light struct {
	Bus           *ninja.DeviceBus
	OnOffBus      *ninja.ChannelBus
	colorBus      *ninja.ChannelBus
	brightnessBus *ninja.ChannelBus
	Batch         bool
	batchBus      *ninja.ChannelBus
	Bulb          *lifx.Bulb
	Client        *lifx.Client
}

func (l *Light) GetJsonLightState() *simplejson.Json {
	js := simplejson.New()
	// js.Set("on", st.On)
	// js.Set("bri", st.Brightness)
	// js.Set("sat", st.Saturation)
	// js.Set("hue", st.Hue)
	// js.Set("ct", st.ColorTemp)
	// js.Set("transitionTime", st.TransitionTime)
	// js.Set("xy", st.XY)

	return js
}

func getOnOffBus(light *Light) *ninja.ChannelBus {
	methods := []string{"turnOn", "turnOff", "set"}
	events := []string{"state"}
	onOffBus, err := light.Bus.AnnounceChannel("on-off", "on-off", methods, events, func(method string, payload *simplejson.Json) {
		log.Infof("got actuation, method is %s", method)
		spew.Dump(payload)
		if light.Batch == true {
			return
		}
		switch method {
		case "turnOn":
			light.turnOnOff(true)
		case "turnOff":
			light.turnOnOff(false)
		case "set":
			state, _ := payload.GetIndex(0).Bool()
			light.turnOnOff(state)
		default:
			log.Criticalf("On-off got an unknown method %s", method)
			return
		}
	})

	if err != nil {
		log.HandleError(err, "Could not announce on/off channel")
	}

	return onOffBus
}

func getBrightBus(light *Light) *ninja.ChannelBus {
	methods := []string{"set"}
	events := []string{"state"}
	brightnessBus, err := light.Bus.AnnounceChannel("brightness", "brightness", methods, events, func(method string, payload *simplejson.Json) {

		if light.Batch == true {
			return
		}

		switch method {
		case "set":
			brightness, _ := payload.GetIndex(0).Float64()
			light.setBrightness(brightness)

		default:
			log.Criticalf("Brightness got an unknown method %s", method)
			return
		}

	})

	if err != nil {
		log.HandleError(err, "Could not announce brightness channel")
	}

	return brightnessBus

}

func getColorBus(light *Light) *ninja.ChannelBus {
	methods := []string{"set"}
	events := []string{"state"}
	colorBus, err := light.Bus.AnnounceChannel("color", "color", methods, events, func(method string, payload *simplejson.Json) {
		if light.Batch == true {
			return
		}
		switch method {
		case "set":
			mode, err := payload.Get("mode").String()
			if err != nil {
				log.Criticalf("No mode sent to color bus: %s", err)
			}
			light.setColor(payload, mode)
		default:
			log.Criticalf("Color got an unknown method %s", method)
		}
	})

	if err != nil {
		log.HandleError(err, "Could not announce color channel")
	}

	return colorBus
}

func getBatchBus(light *Light) *ninja.ChannelBus {
	methods := []string{"setBatch"}
	events := []string{"state"}
	batchBus, err := light.Bus.AnnounceChannel("core.batching", "core.batching", methods, events, func(method string, payload *simplejson.Json) {
		switch method {
		case "setBatch":
			light.setBatchColor(payload.GetIndex(0))
		default:
			log.Criticalf("Color got an unknown method %s", method)
			return
		}
	})

	if err != nil {
		log.HandleError(err, "Could not announce brightness channel")
	}

	return batchBus
}

func NewLight(bus *ninja.DriverBus, client *lifx.Client, bulb *lifx.Bulb) (*Light, error) { //TODO cut this down!
	id := string(bulb.LifxAddress[:6]) //Address is 6 bytes long
	log.Infof("Making light with ID: %s Label:", id, bulb.Label)
	light := &Light{
		Batch:  false,
		Client: client,
		Bulb:   bulb,
	}

	sigs, _ := simplejson.NewJson([]byte(`{
      "ninja:manufacturer": "Lifx",
      "ninja:productName": "Lifx",
      "manufacturer:productModelId": "Lifx",
      "ninja:productType": "Light",
      "ninja:thingType": "light"
  }`))

	deviceBus, _ := bus.AnnounceDevice(id, "light", bulb.Label, sigs) //TODO fix when lib gets updated
	light.Bus = deviceBus
	light.OnOffBus = getOnOffBus(light)
	light.brightnessBus = getBrightBus(light)
	light.colorBus = getColorBus(light)
	light.batchBus = getBatchBus(light)

	return light, nil
}

func (l *Light) StartBatch() {
	l.Batch = true
}

func (l *Light) EndBatch() {
	l.Batch = false
	// l.User.SetLightState(l.Id, l.LightState) //TODO send actual state
	l.OnOffBus.SendEvent("state", l.GetJsonLightState())
}

func (l *Light) turnOnOff(state bool) {
	log.Infof("turning %t", state)
	if state == true {
		l.Client.LightOn(l.Bulb)
	} else {
		l.Client.LightOff(l.Bulb)
	}

}

func (l *Light) setBrightness(fbrightness float64) {
	// bri := fbrightness * math.MaxUint16
	// l.Client.LightColour(l.Bulb, nil, nil, &bri, nil, nil)

}

// func (l *Light) setColor(payload *simplejson.Json, mode string) {
//
// 	spew.Dump(payload)
// 	spew.Dump(mode)
// }

// func (b *Client) LightColour(bulb *Bulb, hue uint16, sat uint16, lum uint16, kelvin uint16, timing uint32) error {
func (l *Light) setColor(payload *simplejson.Json, mode string) {
	log.Infof("setcolor called, payload barfing")
	spew.Dump(payload)
	var transition uint32
	if trans, e := payload.Get("transition").Int(); e == nil {
		trans /= 1000 //LIFX transition time is in seconds
		transition = uint32(trans)
	}

	switch mode {
	case "hue":
		fhue, _ := payload.Get("hue").Float64()
		hue := uint16(fhue * math.MaxUint16)
		fsaturation, _ := payload.Get("saturation").Float64()
		saturation := uint16(fsaturation * math.MaxUint16)
		l.Client.LightColour(l.Bulb, &hue, &saturation, nil, nil, &transition)

	case "xy":
		//TODO: Lifx does not support XY color

	case "temperature":
		temp, _ := payload.Get("temperature").Float64()
		utemp := uint16(math.Floor(1000000 / temp))
		l.Client.LightColour(l.Bulb, nil, nil, nil, &utemp, &transition)

	default:
		log.Criticalf("Bad color mode: %s", mode)
		return
	}

	if !l.Batch {
		l.colorBus.SendEvent("state", l.GetJsonLightState())
	}

}

func (l *Light) setBatchColor(payload *simplejson.Json) {
	l.StartBatch()
	color := payload.Get("color")
	if color != nil {
		l.setColor(color, "hue")
	}
	if brightness, err := payload.Get("brightness").Float64(); err == nil {
		l.setBrightness(brightness)
	}
	if onoff, err := payload.Get("on-off").Bool(); err == nil {
		l.turnOnOff(onoff)
	}
	if transition, err := payload.Get("transition").Int(); err == nil {
		log.Infof("setting transition %d", transition)
	}
	l.EndBatch()
}

func getCurDir() string {
	pwd, _ := os.Getwd()
	return pwd + "/"
}

func (l *Light) sendLightState() {

	// l.User.SetLightState(l.Id, l.LightState) #TODO
	l.OnOffBus.SendEvent("state", l.GetJsonLightState())
}

func isUnique(newbulb lifx.Bulb) bool {
	ret := true
	for _, bulb := range seenlights {
		if bulb.LifxAddress == newbulb.LifxAddress {
			ret = false
		}
	}
	return ret
}

func run() int {
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
		time.Sleep(5 * time.Second)
		for _, bulb := range client.GetBulbs() {
			if isUnique(*bulb) {
				log.Infof("creating new light")
				_, err := NewLight(bus, client, bulb)
				if err != nil {
					log.HandleErrorf(err, "Error creating light instance")
				}
				seenlights = append(seenlights, bulb)
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
