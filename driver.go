package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/bitly/go-simplejson"
	"github.com/davecgh/go-spew/spew"
	"github.com/ninjasphere/go-ninja"
	"github.com/ninjasphere/go-ninja/channels"
	"github.com/ninjasphere/go-ninja/devices"
	"github.com/wolfeidau/lifx"
)

//---------------------------------------------------------------[Bulb]----------------------------------------------------------------

func NewLight(bus *ninja.DriverBus, client *lifx.Client, bulb *lifx.Bulb) (*devices.LightDevice, error) { //TODO cut this down!

	log.Infof("Making light with ID: %s Label: %s", bulb.GetLifxAddress(), bulb.GetLabel())

	sigs, _ := simplejson.NewJson([]byte(`{
			"ninja:manufacturer": "Lifx",
			"ninja:productName": "Lifx Bulb",
			"ninja:productType": "Light",
			"ninja:thingType": "light"
	}`))

	deviceBus, err := bus.AnnounceDevice(bulb.GetLifxAddress(), "light", bulb.GetLabel(), sigs)
	if err != nil {
		log.FatalError(err, "Failed to create light device bus")
	}

	light, err := devices.CreateLightDevice(bulb.GetLabel(), deviceBus)
	if err != nil {
		log.FatalError(err, "Failed to create light device")
	}

	light.EnableOnOffChannel()
	light.EnableBrightnessChannel()
	light.EnableColorChannel()
	light.EnableTransitionChannel()

	light.ApplyOnOff = func(state bool) error {
		var err error
		if state {
			err = client.LightOn(bulb)
		} else {
			err = client.LightOff(bulb)
		}
		if err != nil {
			return fmt.Errorf("Failed to set on-off state: %s", err)
		}
		return nil
	}

	light.ApplyLightState = func(state *devices.LightDeviceState) error {
		jsonState, _ := json.Marshal(state)
		log.Debugf("Sending light state to lifx bulb: %s", jsonState)

		if state.OnOff != nil {
			err := light.ApplyOnOff(*state.OnOff)
			if err != nil {
				return err
			}
		}

		if state.Color != nil || state.Brightness != nil || state.Transition != nil {
			if state.Color == nil {
				return fmt.Errorf("Color value missing from batch set")
			}

			if state.Brightness == nil {
				return fmt.Errorf("Brightness value missing from batch set")
			}

			if state.Transition == nil {
				return fmt.Errorf("Transition value missing from batch set")
			}

			switch state.Color.Mode {
			case "hue":
				return client.LightColour(
					bulb,
					uint16(*state.Color.Hue*math.MaxUint16),
					uint16(*state.Color.Saturation*math.MaxUint16),
					uint16(*state.Brightness*math.MaxUint16),
					0,
					uint32(*state.Transition),
				)

			case "xy":
				//TODO: Lifx does not support XY color
				return fmt.Errorf("XY color mode is not yet supported in the Lifx driver")

			case "temperature":
				client.LightColour(
					bulb,
					0,
					0,
					uint16(*state.Brightness*math.MaxUint16),
					uint16(*state.Color.Temperature),
					uint32(*state.Transition),
				)

			default:
				return fmt.Errorf("Unknown color mode %s", state.Color.Mode)
			}

		}

		return nil
	}

	// TODO: This actually needs to be called when the bulb state changes
	onStateChanged := func() {

		bulbState := bulb.GetState()

		log.Infof("Bulb state changed")

		spew.Dump(bulbState)

		state := &devices.LightDeviceState{}

		onOff := int(bulbState.Power) > 0
		state.OnOff = &onOff

		brightness := float64(bulbState.Brightness) / math.MaxUint16
		state.Brightness = &brightness

		color := &channels.ColorState{}
		if bulbState.Saturation == 0 {
			color.Mode = "temperature"

			temperature := int(bulbState.Kelvin)
			color.Temperature = &temperature

		} else {
			color.Mode = "hue"

			hue := float64(bulbState.Hue) / float64(math.MaxUint16)
			color.Hue = &hue

			saturation := float64(bulbState.Saturation) / float64(math.MaxUint16)
			color.Saturation = &saturation
		}

		state.Color = color

		light.SetLightState(state)
	}

	onStateChanged()

	/*sub := client.Subscribe()

	go func() {
		for {
			event := <-sub.Events
			spew.Dump("Got event", event)
		}
	}()*/

	return light, nil
}

//---------------------------------------------------------------[Utils]----------------------------------------------------------------

func isUnique(newbulb *lifx.Bulb) bool {
	ret := true
	for _, bulb := range seenlights {
		if bulb.LifxAddress == newbulb.LifxAddress {
			ret = false
		}
	}
	return ret
}
