// eg4_can_cli.go
// ==============
// Read an EG4 (LiFePower4 / LL / WallMount, ...) battery that speaks the
// Pylontech-emulation CAN protocol through a Waveshare USB-CAN-A (Model A,
// STM32) adapter, and pipe the decoded values to stdout.
//
// The adapter is NOT SocketCAN: it enumerates as a serial port (/dev/ttyUSB0)
// and uses the Seeed/Waveshare "variable length" serial framing. This program
// speaks that wire protocol directly over the serial fd, so there are NO
// external Go modules -- just `go run eg4_can_cli.go` or `go build`.
//
// stdout carries ONLY the values (so `| jq`, `> log.csv`, `| grep` all work).
// Status, errors and the CSV note go to stderr.
//
// Linux only (uses termios2 ioctls for the 2 Mbaud serial link).
//
// Examples:
//   go build -o eg4can eg4_can_cli.go
//   ./eg4can --channel /dev/ttyUSB0                 # readable, one line/sec
//   ./eg4can --format csv > battery.csv             # log to disk
//   ./eg4can --format json | jq 'select(.alarms != "")'
//   ./eg4can --format raw                           # every frame + decode
//   ./eg4can --demo --format csv                    # no hardware
//   ./eg4can --selftest                             # decoder tests, exit
//
//go:build linux

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Protocol constants (classic Pylontech v1.2 set, all little-endian).
// Verify scaling against your own EG4: read pack V off the LCD, match 0x356.
// ---------------------------------------------------------------------------
const (
	canLimits   = 0x351
	canSOCSOH   = 0x355
	canMeasure  = 0x356
	canAlarms   = 0x359
	canReqFlags = 0x35C
	canMfr      = 0x35E
)

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

// small pointer constructors
func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }
func bp(v bool) *bool       { return &v }
func sp(v string) *string   { return &v }

// bounds-checked little-endian readers
func u16(d []byte, i int) (uint16, bool) {
	if len(d) < i+2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(d[i:]), true
}
func s16(d []byte, i int) (int16, bool) {
	v, ok := u16(d, i)
	return int16(v), ok
}

// ---------------------------------------------------------------------------
// Decoders: mutate *State only for fields present in this frame.
// ---------------------------------------------------------------------------
func decodeLimits(d []byte, s *State) {
	if v, ok := u16(d, 0); ok {
		s.ChgVLimit = fp(float64(v) / 10)
	}
	if v, ok := s16(d, 2); ok {
		s.ChgALimit = fp(float64(v) / 10)
	}
	if v, ok := s16(d, 4); ok {
		s.DisALimit = fp(float64(v) / 10)
	}
	if v, ok := u16(d, 6); ok {
		s.DisVLimit = fp(float64(v) / 10)
	}
}

func decodeSOCSOH(d []byte, s *State) {
	if v, ok := u16(d, 0); ok {
		s.SOC = ip(int(v))
	}
	if v, ok := u16(d, 2); ok {
		s.SOH = ip(int(v))
	}
}

func decodeMeasure(d []byte, s *State) {
	if v, ok := u16(d, 0); ok {
		s.PackV = fp(float64(v) / 100)
	}
	if v, ok := s16(d, 2); ok {
		s.PackA = fp(float64(v) / 10)
	}
	if v, ok := s16(d, 4); ok {
		s.TempC = fp(float64(v) / 10)
	}
}

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

func bits(d []byte, table []bitLabel) []string {
	out := []string{}
	for _, b := range table {
		if len(d) > b.idx && d[b.idx]&b.mask != 0 {
			out = append(out, b.name)
		}
	}
	return out
}

func decodeAlarms(d []byte, s *State) {
	s.Protections = bits(d, protBits)
	s.Warnings = bits(d, warnBits)
}

func decodeReqFlags(d []byte, s *State) {
	var b0 byte
	if len(d) > 0 {
		b0 = d[0]
	}
	s.ChargeEn = bp(b0&0x80 != 0)
	s.DischargeEn = bp(b0&0x40 != 0)
}

func decodeMfr(d []byte, s *State) {
	s.Mfr = sp(strings.TrimRight(string(d), " \x00"))
}

func dispatch(id uint32, d []byte, s *State) {
	switch id {
	case canLimits:
		decodeLimits(d, s)
	case canSOCSOH:
		decodeSOCSOH(d, s)
	case canMeasure:
		decodeMeasure(d, s)
	case canAlarms:
		decodeAlarms(d, s)
	case canReqFlags:
		decodeReqFlags(d, s)
	case canMfr:
		decodeMfr(d, s)
	}
}

// decodeOne is used by raw mode to annotate a single frame.
func decodeOne(id uint32, d []byte) string {
	var s State
	dispatch(id, d, &s)
	var parts []string
	add := func(k string, v any) { parts = append(parts, fmt.Sprintf("%s=%v", k, v)) }
	switch id {
	case canLimits:
		if s.ChgVLimit != nil {
			add("chg_v", *s.ChgVLimit)
			add("chg_a", *s.ChgALimit)
			add("dis_a", *s.DisALimit)
			add("dis_v", *s.DisVLimit)
		}
	case canSOCSOH:
		if s.SOC != nil {
			add("soc", *s.SOC)
			add("soh", *s.SOH)
		}
	case canMeasure:
		if s.PackV != nil {
			add("v", *s.PackV)
			add("a", *s.PackA)
			add("t", *s.TempC)
		}
	case canReqFlags:
		add("chg_en", *s.ChargeEn)
		add("dis_en", *s.DischargeEn)
	case canMfr:
		add("mfr", *s.Mfr)
	case canAlarms:
		add("prot", s.Protections)
		add("warn", s.Warnings)
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Row: a flattened snapshot for output. JSON tags define column order.
// ---------------------------------------------------------------------------
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

var csvFields = []string{
	"soc_pct", "soh_pct", "pack_voltage_v", "pack_current_a", "power_w",
	"temperature_c", "charge_enable", "discharge_enable",
	"charge_voltage_limit_v", "charge_current_limit_a",
	"discharge_current_limit_a", "discharge_voltage_limit_v",
	"manufacturer", "alarms",
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

// ---------------------------------------------------------------------------
// Emitters
// ---------------------------------------------------------------------------
func f1(p *float64, dp int) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%.*f", dp, *p)
}

func emitHuman(r Row) {
	ts := time.Now().Format("15:04:05")
	soc, soh := "—", "—"
	if r.SOC != nil {
		soc = fmt.Sprintf("%d%%", *r.SOC)
	}
	if r.SOH != nil {
		soh = fmt.Sprintf("%d%%", *r.SOH)
	}
	pw := "—"
	if r.PowerW != nil {
		pw = fmt.Sprintf("%d", *r.PowerW)
	}
	chg, dis := "off", "off"
	if r.ChargeEn != nil && *r.ChargeEn {
		chg = "on"
	}
	if r.DischargeEn != nil && *r.DischargeEn {
		dis = "on"
	}
	line := fmt.Sprintf("%s  SOC %s  SOH %s  %sV  %sA  %sW  %s°C  chg:%s  dis:%s",
		ts, soc, soh, f1(r.PackV, 2), f1(r.PackA, 1), pw, f1(r.TempC, 1), chg, dis)
	if r.Alarms != "" {
		line += "  ALARM[" + r.Alarms + "]"
	}
	fmt.Println(line)
}

var csvHeaderWritten bool

func emitCSV(r Row) {
	if !csvHeaderWritten {
		fmt.Println("timestamp," + strings.Join(csvFields, ","))
		csvHeaderWritten = true
	}
	cellF := func(p *float64) string {
		if p == nil {
			return ""
		}
		return fmt.Sprintf("%.3f", *p)
	}
	cellI := func(p *int) string {
		if p == nil {
			return ""
		}
		return fmt.Sprintf("%d", *p)
	}
	cellB := func(p *bool) string {
		if p == nil {
			return ""
		}
		if *p {
			return "1"
		}
		return "0"
	}
	mfr := ""
	if r.Mfr != nil {
		mfr = *r.Mfr
	}
	alarms := r.Alarms
	if strings.ContainsAny(alarms, ",;") {
		alarms = `"` + alarms + `"`
	}
	vals := []string{
		time.Now().Format("2006-01-02T15:04:05"),
		cellI(r.SOC), cellI(r.SOH), cellF(r.PackV), cellF(r.PackA), cellI(r.PowerW),
		cellF(r.TempC), cellB(r.ChargeEn), cellB(r.DischargeEn),
		cellF(r.ChgVLimit), cellF(r.ChgALimit), cellF(r.DisALimit), cellF(r.DisVLimit),
		mfr, alarms,
	}
	fmt.Println(strings.Join(vals, ","))
}

func emitJSON(r Row) {
	out := struct {
		Timestamp string `json:"timestamp"`
		Row
	}{Timestamp: time.Now().Format("2006-01-02T15:04:05"), Row: r}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}

func emitRaw(f frame) {
	extra := decodeOne(f.id, f.data)
	if extra != "" {
		extra = "  " + extra
	}
	fmt.Printf("%s  %03X  [%d]  % x%s\n",
		time.Now().Format("15:04:05"), f.id, len(f.data), f.data, extra)
}

// ---------------------------------------------------------------------------
// Frame sources
// ---------------------------------------------------------------------------
type frame struct {
	id   uint32
	data []byte
}

func demoSource(out chan<- frame) {
	fmt.Fprintln(os.Stderr, "DEMO MODE (no hardware)")
	t0 := time.Now()
	soc := 62.0
	le := binary.LittleEndian
	pk := func(vals ...uint16) []byte {
		b := make([]byte, 2*len(vals))
		for i, v := range vals {
			le.PutUint16(b[2*i:], v)
		}
		return b
	}
	for {
		t := time.Since(t0).Seconds()
		current := 18.0 * math.Sin(t/7.0)
		vpack := 52.6 + current*0.01
		temp := 24.0 + 2.0*math.Sin(t/30.0)
		soc = math.Max(5, math.Min(100, soc+current*0.02))

		out <- frame{canLimits, pk(532, 370, 370, 450)}
		out <- frame{canSOCSOH, pk(uint16(soc), 100)}
		out <- frame{canMeasure, pk(uint16(vpack*100), uint16(int16(current*10)), uint16(int16(temp*10)))}
		alarm := []byte{0, 0, 0, 0, 4, 0x50, 0x4E, 0}
		if temp > 25.6 {
			alarm[2] = 0x08
		}
		out <- frame{canAlarms, alarm}
		out <- frame{canReqFlags, []byte{0xC0, 0}}
		out <- frame{canMfr, []byte("PYLON   ")}
		time.Sleep(1 * time.Second)
	}
}

// ---- Waveshare USB-CAN-A serial link (Linux termios2 for 2 Mbaud) ----------
const (
	tcgets2 = 0x802C542A // _IOR('T', 0x2A, struct termios2)
	tcsets2 = 0x402C542B // _IOW('T', 0x2B, struct termios2)
	bother  = 0x1000
	cbaud   = 0x100F
)

type termios2 struct {
	Iflag, Oflag, Cflag, Lflag uint32
	Line                       uint8
	Cc                         [19]uint8
	Ispeed, Ospeed             uint32
}

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// configureSerial puts the port in raw mode at an arbitrary baud via termios2.
func configureSerial(fd int, baud uint32) error {
	var t termios2
	if err := ioctl(fd, tcgets2, unsafe.Pointer(&t)); err != nil {
		return fmt.Errorf("TCGETS2: %w", err)
	}
	t.Iflag = 0
	t.Oflag = 0
	t.Lflag = 0
	t.Cflag &^= cbaud | syscall.CSIZE | syscall.PARENB | syscall.CSTOPB
	t.Cflag |= syscall.CS8 | syscall.CREAD | syscall.CLOCAL | bother
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	t.Ispeed = baud
	t.Ospeed = baud
	if err := ioctl(fd, tcsets2, unsafe.Pointer(&t)); err != nil {
		return fmt.Errorf("TCSETS2: %w", err)
	}
	return nil
}

// bitrate byte codes accepted by the adapter's config frame.
var bitrateCode = map[int]byte{
	1000000: 0x01, 800000: 0x02, 500000: 0x03, 400000: 0x04,
	250000: 0x05, 200000: 0x06, 125000: 0x07, 100000: 0x08,
	50000: 0x09, 20000: 0x0A, 10000: 0x0B, 5000: 0x0C,
}
var modeCode = map[string]byte{"normal": 0x00, "silent": 0x02}

// configFrame is the 20-byte 0xAA 0x55 0x12 ... init packet (matches python-can).
func configFrame(bitrate int, mode string) ([]byte, error) {
	bc, ok := bitrateCode[bitrate]
	if !ok {
		return nil, fmt.Errorf("unsupported CAN bitrate %d", bitrate)
	}
	mc := modeCode[mode]
	m := []byte{
		0xAA, 0x55, 0x12,
		bc,         // CAN baud
		0x01,       // frame type: STD
		0, 0, 0, 0, // filter id
		0, 0, 0, 0, // mask id
		mc,   // operation mode
		0x01, // "send once" flag, per vendor app
		0, 0, 0, 0,
	}
	var sum byte
	for _, b := range m[2:] {
		sum += b
	}
	return append(m, sum), nil
}

func hardwareSource(out chan<- frame, channel string, bitrate, serialBaud int, mode string) {
	fd, err := syscall.Open(channel, syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s failed: %v\n", channel, err)
		close(out)
		return
	}
	if err := configureSerial(fd, uint32(serialBaud)); err != nil {
		fmt.Fprintf(os.Stderr, "configure failed: %v\n", err)
		syscall.Close(fd)
		close(out)
		return
	}
	f := os.NewFile(uintptr(fd), channel)
	defer f.Close()

	// Send the adapter init/config frame (sets CAN bitrate + mode).
	cfg, err := configFrame(bitrate, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		close(out)
		return
	}
	if _, err := f.Write(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "write init frame failed: %v\n", err)
		close(out)
		return
	}
	fmt.Fprintf(os.Stderr, "connected: %s @ %d bps CAN (%s)\n", channel, bitrate, mode)

	r := bufio.NewReaderSize(f, 4096)
	for {
		// resync to start byte 0xAA
		b1, err := r.ReadByte()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			close(out)
			return
		}
		if b1 != 0xAA {
			continue
		}
		b2, err := r.ReadByte()
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			close(out)
			return
		}
		if b2 == 0x55 { // status frame: 0xAA 0x55 + 18 bytes, ignore
			io.CopyN(io.Discard, r, 18)
			continue
		}
		length := int(b2 & 0x0F)
		isExt := b2&0x20 != 0
		var id uint32
		if isExt {
			var idb [4]byte
			if _, err := io.ReadFull(r, idb[:]); err != nil {
				continue
			}
			id = binary.LittleEndian.Uint32(idb[:])
		} else {
			var idb [2]byte
			if _, err := io.ReadFull(r, idb[:]); err != nil {
				continue
			}
			id = uint32(binary.LittleEndian.Uint16(idb[:]))
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			continue
		}
		end, err := r.ReadByte()
		if err != nil || end != 0x55 {
			continue // framing error -> resync
		}
		out <- frame{id, data}
	}
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------
func run(frames <-chan frame, format string, interval time.Duration) {
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
			dispatch(f.id, f.data, &state)
		case <-ticker.C:
			emit(buildRow(&state))
		}
	}
}

// ---------------------------------------------------------------------------
// Self-test
// ---------------------------------------------------------------------------
func selftest() int {
	ok := true
	check := func(name string, got, want any) {
		good := fmt.Sprintf("%v", got) == fmt.Sprintf("%v", want)
		status := "PASS"
		if !good {
			status = "FAIL"
			ok = false
		}
		fmt.Printf("  %s  %s: %v (want %v)\n", status, name, got, want)
	}
	dec := func(hex []byte, id uint32) State {
		var s State
		dispatch(id, hex, &s)
		return s
	}

	lim := dec([]byte{0x14, 0x02, 0x74, 0x0E, 0x74, 0x0E, 0xCC, 0x01}, canLimits)
	check("chg_v_limit", *lim.ChgVLimit, 53.2)
	check("chg_a_limit", *lim.ChgALimit, 370.0)
	check("dis_v_limit", *lim.DisVLimit, 46.0)

	ss := dec([]byte{0x1A, 0x00, 0x64, 0x00}, canSOCSOH)
	check("soc", *ss.SOC, 26)
	check("soh", *ss.SOH, 100)

	m := dec([]byte{0x4E, 0x13, 0xD2, 0xFF, 0x0A, 0x01}, canMeasure)
	check("volt", *m.PackV, 49.42)
	check("curr", *m.PackA, -4.6)
	check("temp", *m.TempC, 26.6)

	mf := dec([]byte("PYLON   "), canMfr)
	check("mfr", *mf.Mfr, "PYLON")

	fl := dec([]byte{0xC0, 0x00}, canReqFlags)
	check("chg_en", *fl.ChargeEn, true)
	check("dis_en", *fl.DischargeEn, true)

	al := dec([]byte{0, 0, 0x08, 0, 4, 0x50, 0x4E, 0}, canAlarms)
	found := false
	for _, w := range al.Warnings {
		if w == "over_temp_warn" {
			found = true
		}
	}
	check("warn", found, true)

	// config frame checksum sanity (500k, silent)
	cf, _ := configFrame(500000, "silent")
	check("cfg_len", len(cf), 20)

	if ok {
		fmt.Println("ALL PASS")
		return 0
	}
	fmt.Println("SOME FAILED")
	return 1
}

func main() {
	channel := flag.String("channel", "/dev/ttyUSB0", "adapter serial device")
	bitrate := flag.Int("bitrate", 500000, "CAN bitrate (EG4 = 500000)")
	serialBaud := flag.Int("serial-baud", 2000000, "adapter USB serial baud")
	mode := flag.String("mode", "silent", "silent = listen-only (safe); normal = also ACK")
	format := flag.String("format", "human", "output format: human|csv|json|raw")
	intervalSec := flag.Float64("interval", 1.0, "seconds between snapshots (ignored for raw)")
	demo := flag.Bool("demo", false, "simulate a battery, no hardware")
	doSelftest := flag.Bool("selftest", false, "run decoder tests and exit")
	flag.Parse()

	if *doSelftest {
		os.Exit(selftest())
	}
	switch *format {
	case "human", "csv", "json", "raw":
	default:
		fmt.Fprintf(os.Stderr, "unknown format %q\n", *format)
		os.Exit(2)
	}

	frames := make(chan frame, 256)
	if *demo {
		go demoSource(frames)
	} else {
		go hardwareSource(frames, *channel, *bitrate, *serialBaud, *mode)
	}

	run(frames, *format, time.Duration(*intervalSec*float64(time.Second)))
}
