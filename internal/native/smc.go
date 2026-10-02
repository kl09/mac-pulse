package native

// #include "native.h"
import "C"

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
)

const (
	smcTypeFloat = "flt "
	smcTypeUint8 = "ui8 "
)

type Fan struct {
	// RPM 0 is a stopped fan, which is normal on an idle MacBook.
	RPM, Min, Max float64
}

var smc smcClient

// smcClient holds the one connection and the temperature keys found on this machine. mu
// guards only the key walk, so a fan read never waits for it.
type smcClient struct {
	once sync.Once
	open bool

	mu       sync.Mutex
	walked   bool
	tempKeys []string
}

func Fans() ([]Fan, error) {
	if err := smc.connect(); err != nil {
		return nil, fmt.Errorf("read fans: %w", err)
	}
	raw, err := smcRead("FNum", smcTypeUint8)
	if err != nil {
		return nil, fmt.Errorf("read fans: %w", err)
	}
	fans := make([]Fan, raw[0])
	for i := range fans {
		for suffix, field := range map[string]*float64{"Ac": &fans[i].RPM, "Mn": &fans[i].Min, "Mx": &fans[i].Max} {
			if *field, err = smcFloat(fmt.Sprintf("F%d%s", i, suffix)); err != nil {
				return nil, fmt.Errorf("read fans: %w", err)
			}
		}
	}
	return fans, nil
}

// SystemPower is the whole machine's draw and the adapter's input, in watts.
func SystemPower() (systemW, adapterW float64, err error) {
	if err := smc.connect(); err != nil {
		return 0, 0, fmt.Errorf("read system power: %w", err)
	}
	if systemW, err = smcFloat("PSTR"); err != nil {
		return 0, 0, fmt.Errorf("read system power: %w", err)
	}
	if adapterW, err = smcFloat("PDTR"); err != nil {
		return 0, 0, fmt.Errorf("read system power: %w", err)
	}
	return systemW, adapterW, nil
}

// Temps returns every float temperature key between 0 and 150 °C. Which keys exist depends
// on the chip, so the first call walks the whole key table.
//
// That first call takes ~0.4 s on the caller's goroutine; walk in the background
// if the missed sampler tick is ever noticed.
func Temps() (map[string]float64, error) {
	if err := smc.connect(); err != nil {
		return nil, fmt.Errorf("read temperatures: %w", err)
	}
	keys, err := smc.temperatureKeys()
	if err != nil {
		return nil, fmt.Errorf("read temperatures: %w", err)
	}
	temps := make(map[string]float64, len(keys))
	for _, key := range keys {
		if t, err := smcFloat(key); err == nil && t > 0 && t < 150 {
			temps[key] = t
		}
	}
	return temps, nil
}

func (c *smcClient) connect() error {
	c.once.Do(func() { c.open = C.mpSMCOpen() == 0 })
	if !c.open {
		return fmt.Errorf("open AppleSMC: %w", ErrUnavailable)
	}
	return nil
}

func (c *smcClient) temperatureKeys() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.walked {
		return c.tempKeys, nil
	}
	raw, err := smcRead("#KEY", "ui32")
	if err != nil {
		return nil, err
	}
	for i := range binary.BigEndian.Uint32(raw) {
		var code, typ, size C.uint32_t
		if C.mpSMCKeyAt(C.uint32_t(i), &code) != 0 || byte(code>>24) != 'T' {
			continue
		}
		if C.mpSMCKeyInfo(code, &typ, &size) == 0 && fourCCString(uint32(typ)) == smcTypeFloat {
			c.tempKeys = append(c.tempKeys, fourCCString(uint32(code)))
		}
	}
	c.walked = true
	return c.tempKeys, nil
}

func smcFloat(key string) (float64, error) {
	raw, err := smcRead(key, smcTypeFloat)
	if err != nil {
		return 0, err
	}
	return decodeFloat(raw), nil
}

// smcRead trusts the caller's type: its size is the read size, which saves the key-info
// round trip that would double the cost of every read.
func smcRead(key, typ string) ([]byte, error) {
	size := map[string]int{smcTypeFloat: 4, smcTypeUint8: 1, "ui32": 4}[typ]
	var buf [32]C.uint8_t
	if rc := C.mpSMCRead(C.uint32_t(fourCC(key)), C.uint32_t(size), &buf[0]); rc != 0 {
		return nil, fmt.Errorf("smc key %s: result %#x: %w", key, uint32(rc), ErrUnavailable)
	}
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = byte(buf[i])
	}
	return raw, nil
}

func fourCC(key string) uint32 {
	if len(key) != 4 {
		return 0
	}
	return binary.BigEndian.Uint32([]byte(key))
}

func fourCCString(code uint32) string {
	return string(binary.BigEndian.AppendUint32(nil, code))
}

// decodeFloat reads the SMC "flt " type, which is little-endian on Apple Silicon.
func decodeFloat(raw []byte) float64 {
	if len(raw) != 4 {
		return 0
	}
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(raw)))
}
