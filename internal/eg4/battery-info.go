package eg4

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/eg4/battery/monitor/internal"
	caninterface "github.com/eg4/battery/monitor/internal/can-interface"
	structs "github.com/eg4/battery/monitor/internal/data-structures"
	serialcan "github.com/eg4/battery/monitor/internal/serial-can"
)

// ConnectToBattery initializes a connection to the CAN bus and processes battery data with the specified serial configuration.
func ConnectToBattery(config structs.SerialConfig) (isconnected bool, err error) {

	connectedChannel := make(chan bool)

	//Try Serial Port first
	//TODO: loop thru the first 4 USB ports at least
	//Collect data from the serial port
	frames := make(chan structs.Frame, 256)
	go serialcan.HardwareSource(frames, config.Channel, config.Bitrate, config.SerialBaud, config.Mode, connectedChannel)

	isconnected = <-connectedChannel

	//Try the can interface
	if !isconnected {
		go caninterface.HardwareSource(frames, config.IFace, connectedChannel)
	}

	if !isconnected {
		return false, errors.New("failed to connect to battery")
	}

	//Start the battery monitoring process
	var wg sync.WaitGroup
	wg.Go(func() {
		run(frames, time.Duration(config.IntervalSec*float64(time.Second)))
	})

	return true, nil

}

// run processes incoming CAN frames from the channel, updates state, and periodically publishes summarized data.
func run(frames <-chan structs.Frame, interval time.Duration) {
	var state structs.State

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				return
			}
			dispatch(frame, &state)
		case <-ticker.C:
			publish(buildRow(&state))
		}
	}
}

// publish sends summarized data to the front end. Posting the data is done via the internal.EventBus.
func publish(row structs.Row) {
	internal.EventBus.Publish("batteryStatus", row)
}

func dispatch(f structs.Frame, s *structs.State) {
	switch f.Id {
	case structs.CanLimits:
		decodeLimits(f.Data, s)
	case structs.CanSOCSOH:
		decodeSOCSOH(f.Data, s)
	case structs.CanMeasure:
		decodeMeasure(f.Data, s)
	case structs.CanAlarms:
		decodeAlarms(f.Data, s)
	case structs.CanReqFlags:
		decodeReqFlags(f.Data, s)
	case structs.CanMfr:
		decodeMfr(f.Data, s)
	}
}

func buildRow(s *structs.State) structs.Row {

	// Compute the wattage (Power)
	var power *int
	if s.PackV != nil && s.PackA != nil {
		p := int(math.Round(*s.PackV * *s.PackA))
		power = &p
	}
	alarms := append(append([]string{}, s.Protections...), s.Warnings...)

	//Compute mode
	mode := ""

	if s.PackA != nil && *s.PackA > 0 {
		mode = "Charging"
	}

	if s.PackA != nil && *s.PackA < 0 {
		mode = "Discharging"
	}

	if s.PackA != nil && *s.PackA == 0 {
		mode = "Standby"
	}

	return structs.Row{
		SOC: s.SOC, SOH: s.SOH, PackV: s.PackV, PackA: s.PackA,
		PowerW: power, TempC: s.TempC, ChargeEn: s.ChargeEn,
		DischargeEn: s.DischargeEn, ChgVLimit: s.ChgVLimit,
		ChgALimit: s.ChgALimit, DisALimit: s.DisALimit, DisVLimit: s.DisVLimit,
		Mfr: s.Mfr, Alarms: strings.Join(alarms, ";"), Mode: &mode,
	}
}
