package eg4

import (
	"math"
	"strings"
	"time"

	serialcan "github.com/eg4/battery/monitor/internal/serial-can"
)

func GetBatteryInfo(config *SerialConfig) (*Row, error) {

	frames := make(chan serialcan.Frame, 256)
	go serialcan.HardwareSource(frames, config.channel, config.bitrate, config.serialBaud, config.mode)

	return nil, nil

}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------
func run(frames <-chan serialcan.Frame, format string, interval time.Duration) {
	var state State
	emit := map[string]func(Row){"human": emitHuman, "csv": emitCSV, "json": emitJSON}[format]

	if format == "raw" {
		for f := range frames {
			emitRaw(f)
		}
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				return
			}
			dispatch(f, &state)
		case <-ticker.C:
			emit(buildRow(&state))
		}
	}
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
