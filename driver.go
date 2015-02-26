package main

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/channels"
	"github.com/ninjasphere/go-ninja/devices"
	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/go-ninja/model"
	"github.com/wolfeidau/lifx"
)

var info = ninja.LoadModuleInfo("./package.json")

// Default values if we are only setting partial state.
// TODO: @wolfidau This should be coming from the bulb
var defaultTransition = 300
var defaultBrightness float64 = 1
var defaultSaturation float64 = 1
var defaultHue = 0

type LifxDriver struct {
	log       *logger.Logger
	config    *LifxDriverConfig
	conn      *ninja.Connection
	client    *lifx.Client
	sendEvent func(event string, payload interface{}) error

	devices map[string]*lifxDevice
}

// this struct holds channels which are specific to the lifx bulb
type lifxDevice struct {
	illuminance *channels.IlluminanceChannel
}

func NewLifxDriver() {
	d := &LifxDriver{
		log:     logger.GetLogger(info.Name),
		client:  lifx.NewClient(),
		devices: make(map[string]*lifxDevice),
	}

	conn, err := ninja.Connect(info.ID)
	if err != nil {
		d.log.Fatalf("Failed to connect to MQTT: %s", err)
	}

	err = conn.ExportDriver(d)

	if err != nil {
		d.log.Fatalf("Failed to export driver: %s", err)
	}

	go func() {

		sub := d.client.Subscribe()

		for {

			event := <-sub.Events

			switch evnt := event.(type) {
			case *lifx.Bulb:
				if isUnique(evnt) {
					d.log.Infof("creating new light")
					_, err := d.newLight(evnt)
					if err != nil {
						d.log.HandleErrorf(err, "Error creating light instance")
					}
					seenlights = append(seenlights, evnt) //TODO remove bulbs that haven't been seen in a while?
					err = d.client.GetBulbState(evnt)

					if err != nil {
						d.log.Warningf("unable to intiate bulb state request %s", err)
					}
				}
			case *lifx.LightSensorState:
				// handle these vents for each bulb
				if d.devices[evnt.GetLifxAddress()] != nil {
					d.devices[evnt.GetLifxAddress()].illuminance.SendState(float64(evnt.Lux))
				}

			default:
				d.log.Infof("Event %v", event)
			}

		}

	}()

	go d.publishSensorData()

	d.conn = conn
}

type LifxDriverConfig struct {
}

func (d *LifxDriver) Start(config *LifxDriverConfig) error {
	d.log.Infof("Starting with config %v", config)
	d.config = config

	err := d.client.StartDiscovery()
	if err != nil {
		err = fmt.Errorf("Failed to discover bulbs : %s", err)
	}
	return err
}

func (d *LifxDriver) Stop() error {
	return nil
}

func (d *LifxDriver) GetModuleInfo() *model.Module {
	return info
}

func (d *LifxDriver) SetEventHandler(sendEvent func(event string, payload interface{}) error) {
	d.sendEvent = sendEvent
}

//---------------------------------------------------------------[Bulb]----------------------------------------------------------------

func (d *LifxDriver) newLight(bulb *lifx.Bulb) (*devices.LightDevice, error) { //TODO cut this down!

	name := bulb.GetLabel()

	d.log.Infof("Making light with ID: %s Label: %s", bulb.GetLifxAddress(), name)

	light, err := devices.CreateLightDevice(d, &model.Device{
		NaturalID:     bulb.GetLifxAddress(),
		NaturalIDType: "lifx",
		Name:          &name,
		Signatures: &map[string]string{
			"ninja:manufacturer": "Lifx",
			"ninja:productName":  "Lifx Bulb",
			"ninja:productType":  "Light",
			"ninja:thingType":    "light",
		},
	}, d.conn)

	if err != nil {
		d.log.FatalError(err, "Could not create light device")
	}

	light.ApplyOnOff = func(state bool) error {
		var err error
		if state {
			err = d.client.LightOn(bulb)
		} else {
			err = d.client.LightOff(bulb)
		}
		if err != nil {
			return fmt.Errorf("Failed to set on-off state: %s", err)
		}
		return nil
	}

	light.ApplyLightState = func(state *devices.LightDeviceState) error {
		jsonState, _ := json.Marshal(state)
		d.log.Debugf("Sending light state to lifx bulb: %s", jsonState)

		if state.OnOff != nil {
			err := light.ApplyOnOff(*state.OnOff)
			if err != nil {
				return err
			}
		}

		if state.Color != nil || state.Brightness != nil || state.Transition != nil {

			if state.Brightness == nil {
				state.Brightness = &defaultBrightness
			}

			if state.Color == nil {
				state.Color = &channels.ColorState{
					Mode:       "hue",
					Saturation: &defaultSaturation,
				}
			}

			if state.Transition == nil {
				state.Transition = &defaultTransition
			}

			/*if state.Color == nil {
				return fmt.Errorf("Color value missing from batch set")
			}

			if state.Brightness == nil {
				return fmt.Errorf("Brightness value missing from batch set")
			}*/

			switch state.Color.Mode {
			case "hue":
				return d.client.LightColour(
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
				d.client.LightColour(
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

	bulb.SetStateHandler(buildStateHandler(d, bulb, light))

	if err := light.EnableOnOffChannel(); err != nil {
		d.log.FatalError(err, "Could not enable lifx on-off channel")
	}
	if err := light.EnableBrightnessChannel(); err != nil {
		d.log.FatalError(err, "Could not enable lifx brightness channel")
	}
	if err := light.EnableColorChannel("temperature", "hue"); err != nil {
		d.log.FatalError(err, "Could not enable lifx color channel")
	}
	if err := light.EnableTransitionChannel(); err != nil {
		d.log.FatalError(err, "Could not enable lifx transition channel")
	}

	// extra channels for sensors
	illuminance := channels.NewIlluminanceChannel(d)

	if err := d.conn.ExportChannel(light, illuminance, "illuminance"); err != nil {
		d.log.FatalError(err, "Could not enable lifx illuminance channel")
	}

	d.log.Debugf("register illuminance channel for %s", bulb.GetLifxAddress())

	d.devices[bulb.GetLifxAddress()] = &lifxDevice{illuminance}

	return light, nil
}

func buildStateHandler(driver *LifxDriver, bulb *lifx.Bulb, light *devices.LightDevice) lifx.StateHandler {

	return func(bulbState *lifx.BulbState) {

		jsonState, _ := json.Marshal(bulbState)
		driver.log.Debugf("Incoming state: %s", jsonState)

		state := &devices.LightDeviceState{}

		onOff := int(bulbState.Power) > 0
		state.OnOff = &onOff

		brightness := float64(bulbState.Brightness) / math.MaxUint16
		state.Brightness = &brightness

		color := &channels.ColorState{}
		if bulbState.Saturation == 0 {
			color.Mode = "temperature"

			temperature := float64(bulbState.Kelvin)
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

		// TODO use the state.Visible to indicate whether a globe is online/offline at the moment

	}
}

// this function publishes information from the sensors on the lifx bulb
// atm this is limited to illuminance
func (d *LifxDriver) publishSensorData() {
	c := time.Tick(30 * time.Second)
	for _ = range c {
		for _, bulb := range d.client.GetBulbs() {
			d.client.GetAmbientLight(bulb)
		}
	}
}

//---------------------------------------------------------------[Utils]----------------------------------------------------------------

var seenlights []*lifx.Bulb

func isUnique(newbulb *lifx.Bulb) bool {
	ret := true
	for _, bulb := range seenlights {
		if bulb.LifxAddress == newbulb.LifxAddress {
			ret = false
		}
	}
	return ret
}
