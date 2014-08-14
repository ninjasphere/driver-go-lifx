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
	batchBus      *ninja.ChannelBus
	Client        *lifx.Client
	Batch         bool            // are we caching changes?
	Bulb          *lifx.Bulb      // keep a reference to the lifx bulb
	BatchState    *lifx.BulbState // used for batching together changes
	Timing        uint32          // the timing used for the transition in the batch
}

//---------------------------------------------------------------[Busses]----------------------------------------------------------------

func getOnOffBus(light *Light) *ninja.ChannelBus {
	methods := []string{"turnOn", "turnOff", "set"}
	events := []string{"state"}
	onOffBus, err := light.Bus.AnnounceChannel("on-off", "on-off", methods, events, func(method string, payload *simplejson.Json) {
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
			light.setColor(payload)
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

//---------------------------------------------------------------[Bulb]----------------------------------------------------------------

func NewLight(bus *ninja.DriverBus, client *lifx.Client, bulb *lifx.Bulb) (*Light, error) { //TODO cut this down!

	log.Infof("Making light with ID: %s Label:", bulb.GetLifxAddress(), bulb.GetLabel())

	light := &Light{
		Batch:      false,
		Client:     client,
		Bulb:       bulb,
		BatchState: &lifx.BulbState{}, // create an empty batch state
	}

	sigs, _ := simplejson.NewJson([]byte(`{
      "ninja:manufacturer": "Lifx",
      "ninja:productName": "Lifx",
      "manufacturer:productModelId": "Lifx",
      "ninja:productType": "Light",
      "ninja:thingType": "light"
  }`))

	deviceBus, _ := bus.AnnounceDevice(bulb.GetLifxAddress(), "light", bulb.GetLabel(), sigs)
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
	l.sendLightState()
	l.BatchState = &lifx.BulbState{} // create an empty batch state
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
	bri := fbrightness * math.MaxUint16
	l.BatchState.Brightness = uint16(bri)

	if !l.Batch {
		l.sendLightState()
	}
	// l.Client.LightColour(l.Bulb, nil, nil, &bri, nil, nil)

}

func (l *Light) setColor(payload *simplejson.Json) {

	// colorpayload := payload.Get("color")
	mode, err := payload.Get("mode").String()
	if err != nil {
		log.Warningf("No mode sent to color bus: %s", err)
		spew.Dump(payload)
	}

	switch mode {
	case "hue":
		fhue, err := payload.Get("hue").Float64()
		if err != nil {
			log.Warningf("No hue sent to color bus :%s", err)
			spew.Dump(payload)
			return
		}
		hue := uint16(fhue * math.MaxUint16)
		fsaturation, err := payload.Get("saturation").Float64()
		if err != nil {
			log.Warningf("No saturation sent to color bus :%s", err)
			spew.Dump(payload)
			return
		}
		saturation := uint16(fsaturation * math.MaxUint16)
		l.BatchState.Hue = hue
		l.BatchState.Saturation = saturation
		l.BatchState.Kelvin = 0

	case "xy":
		//TODO: Lifx does not support XY color

	case "temperature":
		temp, err := payload.Get("temperature").Float64()
		if err != nil {
			log.Warningf("No temperature sent to color bus :%s", err)
			spew.Dump(payload)
			return
		}
		l.BatchState.Hue = 0
		l.BatchState.Saturation = 0
		l.BatchState.Kelvin = uint16(temp)
		log.Infof("Setting temperature: %d", temp)

	default:
		log.Criticalf("Bad color mode: %s", mode)
		return
	}

	if !l.Batch {
		l.sendLightState()
	}

}

func (l *Light) setTransition(trans int) {
	l.Timing = uint32(trans)
}

func (l *Light) setBatchColor(payload *simplejson.Json) {
	log.Infof("got batch")
	spew.Dump(payload)
	l.StartBatch()
	color := payload.Get("color")
	if color != nil {
		l.setColor(color)
	}
	if brightness, err := payload.Get("brightness").Float64(); err == nil {
		l.setBrightness(brightness)
	}
	if onoff, err := payload.Get("on-off").Bool(); err == nil {
		l.turnOnOff(onoff)
	}
	if transition, err := payload.Get("transition").Int(); err == nil {
		l.setTransition(transition)
	}
	l.EndBatch()
}

func (l *Light) sendLightState() {
	s := l.BatchState
	log.Infof("Sending bulb state: ")
	spew.Dump(s)
	l.Client.LightColour(l.Bulb, s.Hue, s.Saturation, s.Brightness, s.Kelvin, l.Timing)
	l.OnOffBus.SendEvent("state", l.GetJsonLightState())
}

//---------------------------------------------------------------[Utils]----------------------------------------------------------------

func getCurDir() string {
	pwd, _ := os.Getwd()
	return pwd + "/"
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

//---------------------------------------------------------------------------------------------------------------------------------------

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
				seenlights = append(seenlights, bulb) //TODO remove bulbs that haven't been seen in a while?
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
