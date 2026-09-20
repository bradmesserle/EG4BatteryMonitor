package eg4

import (
	"encoding/binary"
	"strings"
)

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

// small pointer constructors
func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }
func bp(v bool) *bool       { return &v }
func sp(v string) *string   { return &v }

func bits(d []byte, table []bitLabel) []string {
	out := []string{}
	for _, b := range table {
		if len(d) > b.idx && d[b.idx]&b.mask != 0 {
			out = append(out, b.name)
		}
	}
	return out
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
