package eg4

// ---------------------------------------------------------------------------
// State: merged accumulator. Pointer fields = "not yet seen" (nil).
// ---------------------------------------------------------------------------
type State struct {
	SOC         *int
	SOH         *int
	PackV       *float64
	PackA       *float64
	TempC       *float64
	ChargeEn    *bool
	DischargeEn *bool
	ChgVLimit   *float64
	ChgALimit   *float64
	DisALimit   *float64
	DisVLimit   *float64
	Mfr         *string
	Protections []string
	Warnings    []string
}

type Row struct {
	SOC         *int     `json:"soc_pct"`
	SOH         *int     `json:"soh_pct"`
	PackV       *float64 `json:"pack_voltage_v"`
	PackA       *float64 `json:"pack_current_a"`
	PowerW      *int     `json:"power_w"`
	TempC       *float64 `json:"temperature_c"`
	ChargeEn    *bool    `json:"charge_enable"`
	DischargeEn *bool    `json:"discharge_enable"`
	ChgVLimit   *float64 `json:"charge_voltage_limit_v"`
	ChgALimit   *float64 `json:"charge_current_limit_a"`
	DisALimit   *float64 `json:"discharge_current_limit_a"`
	DisVLimit   *float64 `json:"discharge_voltage_limit_v"`
	Mfr         *string  `json:"manufacturer"`
	Alarms      string   `json:"alarms"`
}

const (
	canLimits   = 0x351
	canSOCSOH   = 0x355
	canMeasure  = 0x356
	canAlarms   = 0x359
	canReqFlags = 0x35C
	canMfr      = 0x35E
)

type bitLabel struct {
	idx  int
	mask byte
	name string
}

var protBits = []bitLabel{
	{0, 0x02, "over_voltage"}, {0, 0x04, "under_voltage"},
	{0, 0x08, "over_temp"}, {0, 0x10, "under_temp"},
	{0, 0x80, "discharge_overcurrent"},
	{1, 0x01, "charge_overcurrent"}, {1, 0x08, "bms_internal"},
	{1, 0x10, "cell_imbalance"},
}
var warnBits = []bitLabel{
	{2, 0x02, "over_voltage_warn"}, {2, 0x04, "under_voltage_warn"},
	{2, 0x08, "over_temp_warn"}, {2, 0x10, "under_temp_warn"},
	{3, 0x01, "charge_overcurrent_warn"}, {3, 0x80, "discharge_overcurrent_warn"},
}

type SerialConfig struct {
	Channel     string
	Bitrate     int
	SerialBaud  int
	Mode        string
	IntervalSec float64
}
