package main

import (
	"fmt"

	"math"
	"os"
	"os/signal"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/bjeanes/go-lifx/client"
	"github.com/ninjasphere/go-ninja"
)

type Light struct {
	Id            string
	Name          string
	Bus           *ninja.DeviceBus
	OnOffBus      *ninja.ChannelBus
	colorBus      *ninja.ChannelBus
	brightnessBus *ninja.ChannelBus
	Batch         bool
	batchBus      *ninja.ChannelBus
	LightState    *LightState
	lightClient   *client.Light
}

// https://github.com/LIFX/lifx-gem/blob/master/lib/lifx/protocol/light.rb
type LightState struct {
	On             *bool
	Brightness     *uint16
	Hue            *uint16
	Saturation     *uint16
	TransitionTime *uint16
	XY             []float64
	ColorTemp      *uint16
}

func (l *Light) GetJsonLightState() *simplejson.Json {
	st := l.LightState
	js := simplejson.New()
	js.Set("on", st.On)
	js.Set("bri", st.Brightness)
	js.Set("sat", st.Saturation)
	js.Set("hue", st.Hue)
	js.Set("ct", st.ColorTemp)
	js.Set("transitionTime", st.TransitionTime)
	js.Set("xy", st.XY)

	return js
}


func getOnOffBus(light *Light) *ninja.ChannelBus {
	methods := []string{"turnOn", "turnOff", "set"}
	events := []string{"state"}
	onOffBus, _ := light.Bus.AnnounceChannel("on-off", "on-off", methods, events, func(method string, payload *simplejson.Json) {
		if light.Batch == true {
			return
		}
		switch method {
		case "turnOn":
			light.turnOnOff(true)
		case "turnOff":
			light.refreshLightState()
			light.turnOnOff(false)
		case "set":
			state, _ := payload.GetIndex(0).Bool()
			light.turnOnOff(state)
		default:
			log.Criticalf("On-off got an unknown method %s", method)
			return
		}
	})

	return onOffBus
}

func getBrightBus(light *Light) *ninja.ChannelBus {
	methods := []string{"set"}
	events := []string{"state"}
	brightnessBus, _ := light.Bus.AnnounceChannel("brightness", "brightness", methods, events, func(method string, payload *simplejson.Json) {

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

	return brightnessBus

}

func getColorBus(light *Light) *ninja.ChannelBus {
	methods := []string{"set"}
	events := []string{"state"}
	colorBus, _ := light.Bus.AnnounceChannel("color", "color", methods, events, func(method string, payload *simplejson.Json) {
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

	return colorBus
}

func NewLight(bus *ninja.DriverBus, client *client.Light) (*Light, error) { //TODO cut this down!
	lightID := "1"
	lightState := createLightState()

	light := &Light{ //TODO fix when lib gets updated
		Id:         lightID,
		Name:       "LiFX Bulb",
		LightState: &lightState,
		Batch:      false,
		lightClient:			client,
	}

	sigs, _ := simplejson.NewJson([]byte(`{
      "ninja:manufacturer": "Lifx",
      "ninja:productName": "Lifx",
      "manufacturer:productModelId": "Lifx",
      "ninja:productType": "Light",
      "ninja:thingType": "light"
  }`))

	deviceBus, _ := bus.AnnounceDevice(lightID, "light", "LiFX Bulb", sigs) //TODO fix when lib gets updated
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
	if(state == true) {
		l.lightClient.TurnOn()
	} else {
		l.lightClient.TurnOff()
	}

}

func (l *Light) setBrightness(fbrightness float64) {//TODO

}

func (l *Light) setColor(payload *simplejson.Json, mode string) {
	l.refreshLightState()
	switch mode {
	case "hue": //TODO less verbose plz
		if trans, e := payload.Get("transition").Int(); e == nil {
			l.setTransition(trans)
		}
		fhue, _ := payload.Get("hue").Float64()
		hue := uint16(fhue * math.MaxUint16)
		fsaturation, _ := payload.Get("saturation").Float64()
		saturation := uint16(fsaturation * math.MaxUint16)
		on := bool(true)
		l.LightState.Hue = &hue
		l.LightState.Saturation = &saturation
		l.LightState.On = &on
	case "xy":
		if trans, e := payload.Get("transition").Int(); e == nil {
			l.setTransition(trans)
		}
		x, _ := payload.Get("x").Float64()
		y, _ := payload.Get("y").Float64()
		xy := []float64{x, y}
		l.LightState.XY = xy
		l.LightState.Hue = nil
		l.LightState.Saturation = nil
		l.LightState.ColorTemp = nil
	case "temperature":
		if trans, e := payload.Get("transition").Int(); e == nil {
			l.setTransition(trans)
		}
		temp, _ := payload.Get("temperature").Float64()
		utemp := uint16(math.Floor(1000000 / temp))
		l.LightState.ColorTemp = &utemp
		l.LightState.XY = nil
		l.LightState.Hue = nil
		l.LightState.Saturation = nil
	default:
		log.Criticalf("Bad color mode: %s", mode)
		return
	}

	if !l.Batch {
		// l.User.SetLightState(l.Id, l.LightState) //TODO
		l.colorBus.SendEvent("state", l.GetJsonLightState())
	}

}

func (l *Light) setTransition(transTime int) {
	transTime = transTime / 1000 //LIFX transition time in seconds
	utranstime := uint16(transTime)
	l.LightState.TransitionTime = &utranstime
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
		l.setTransition(transition)
	}

	l.EndBatch()
}


func createLightState() LightState {

	on := bool(false)
	brightness := uint16(0)
	saturation := uint16(0)
	hueVal := uint16(0)
	transitionTime := uint16(0)

	lightState := LightState{
		On:             &on,
		Brightness:     &brightness,
		Saturation:     &saturation,
		Hue:            &hueVal,
		TransitionTime: &transitionTime,
	}

	return lightState
}

func getCurDir() string {
	pwd, _ := os.Getwd()
	return pwd + "/"
}

func (l *Light) sendLightState() {

	// l.User.SetLightState(l.Id, l.LightState)
	l.OnOffBus.SendEvent("state", l.GetJsonLightState())
}

func (l *Light) refreshLightState() { //TODO

}

func blinkAllLights() {
	c := client.New()
	c.Discover()

	for i:=0; i<3; i++ {
		for _, l := range c.Lights {
			l.TurnOff()
		}
		time.Sleep(1 * time.Second)
		for _, l := range c.Lights {
			l.TurnOn()
		}
		time.Sleep(1 * time.Second)
	}
}

func getBatchBus(light *Light) *ninja.ChannelBus {
	methods := []string{"setBatch"}
	events := []string{"state"}
	batchBus, _ := light.Bus.AnnounceChannel("core.batching", "core.batching", methods, events, func(method string, payload *simplejson.Json) {
		switch method {
		case "setBatch":
			light.setBatchColor(payload.GetIndex(0))
		default:
			log.Criticalf("Color got an unknown method %s", method)
			return
		}
	})

	return batchBus
}

func run() int {
	log.Infof("Starting " + drivername)

	conn, err := ninja.Connect("com.ninjablocks.lifx")
	bus, err := conn.AnnounceDriver("com.ninjablocks.hue", drivername, getCurDir())
	if err != nil {
		log.HandleError(err, "Could not get driver bus")
	}

	lightClient := client.New()
	lightClient.Discover()

	for _,l := range lightClient.Lights {
		_, err := NewLight(bus,&l)
		if err != nil {
			log.Criticalf("Error creating light instance",err)
		}

	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	// Block until a signal is received.
	s := <-c
	fmt.Println("Got signal:", s)

	return 0
}
