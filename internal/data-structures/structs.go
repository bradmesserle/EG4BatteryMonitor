package data_structures

// State ---------------------------------------------------------------------------
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
	Mode        *string  `json:"mode"`
}

const (
	CanLimits   = 0x351
	CanSOCSOH   = 0x355
	CanMeasure  = 0x356
	CanAlarms   = 0x359
	CanReqFlags = 0x35C
	CanMfr      = 0x35E
)

type BitLabel struct {
	Idx  int
	Mask byte
	Name string
}

var ProtBits = []BitLabel{
	{0, 0x02, "over_voltage"}, {0, 0x04, "under_voltage"},
	{0, 0x08, "over_temp"}, {0, 0x10, "under_temp"},
	{0, 0x80, "discharge_overcurrent"},
	{1, 0x01, "charge_overcurrent"}, {1, 0x08, "bms_internal"},
	{1, 0x10, "cell_imbalance"},
}
var WarnBits = []BitLabel{
	{2, 0x02, "over_voltage_warn"}, {2, 0x04, "under_voltage_warn"},
	{2, 0x08, "over_temp_warn"}, {2, 0x10, "under_temp_warn"},
	{3, 0x01, "charge_overcurrent_warn"}, {3, 0x80, "discharge_overcurrent_warn"},
}

type SerialConfig struct {
	Channel     string
	IFace       string
	Bitrate     int
	SerialBaud  int
	Mode        string
	IntervalSec float64
}

type Frame struct {
	Id   uint32
	Data []byte
}
