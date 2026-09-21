package eg4

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/eg4/battery/monitor/internal"
	serialcan "github.com/eg4/battery/monitor/internal/serial-can"
)

// GetBatteryInfo initializes a connection to the CAN bus and processes battery data with the specified serial configuration.
func GetBatteryInfo(config SerialConfig) error {

	frames := make(chan serialcan.Frame, 256)
	go serialcan.HardwareSource(frames, config.Channel, config.Bitrate, config.SerialBaud, config.Mode)

	//Start the battery monitoring process
	var wg sync.WaitGroup
	wg.Go(func() {
		run(frames, time.Duration(config.IntervalSec*float64(time.Second)))
	})

	return nil

}

// run processes incoming CAN frames from the channel, updates state, and periodically publishes summarized data.
func run(frames <-chan serialcan.Frame, interval time.Duration) {
	var state State

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
func publish(row Row) {

	out := struct {
		Timestamp string `json:"timestamp"`
		Row
	}{Timestamp: time.Now().Format("2006-01-02T15:04:05"), Row: row}
	b, _ := json.Marshal(out)
	//fmt.Println(string(b))

	//Send Success message to the front end.
	internal.EventBus.Publish("batteryStatus", string(b))

}

func dispatch(f serialcan.Frame, s *State) {
	switch f.Id {
	case canLimits:
		decodeLimits(f.Data, s)
	case canSOCSOH:
		decodeSOCSOH(f.Data, s)
	case canMeasure:
		decodeMeasure(f.Data, s)
	case canAlarms:
		decodeAlarms(f.Data, s)
	case canReqFlags:
		decodeReqFlags(f.Data, s)
	case canMfr:
		decodeMfr(f.Data, s)
	}
}

func buildRow(s *State) Row {
	var power *int
	if s.PackV != nil && s.PackA != nil {
		p := int(math.Round(*s.PackV * *s.PackA))
		power = &p
	}
	alarms := append(append([]string{}, s.Protections...), s.Warnings...)
	return Row{
		SOC: s.SOC, SOH: s.SOH, PackV: s.PackV, PackA: s.PackA,
		PowerW: power, TempC: s.TempC, ChargeEn: s.ChargeEn,
		DischargeEn: s.DischargeEn, ChgVLimit: s.ChgVLimit,
		ChgALimit: s.ChgALimit, DisALimit: s.DisALimit, DisVLimit: s.DisVLimit,
		Mfr: s.Mfr, Alarms: strings.Join(alarms, ";"),
	}
}
