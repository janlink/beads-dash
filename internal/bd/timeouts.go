package bd

import "time"

// Timeouts bound one bd call by kind.
type Timeouts struct {
	// Probe covers startup probes (version, where).
	Probe time.Duration
	// Read covers list, ready and the other reads.
	Read time.Duration
	// Write covers mutating commands.
	Write time.Duration
}

// DefaultTimeouts are 5 s for probes and 30 s for reads and writes.
func DefaultTimeouts() Timeouts {
	return Timeouts{Probe: 5 * time.Second, Read: 30 * time.Second, Write: 30 * time.Second}
}

// Scaled multiplies every timeout by scale (BDASH_TIMEOUT_SCALE); a scale
// of zero or less changes nothing.
func (t Timeouts) Scaled(scale float64) Timeouts {
	if scale <= 0 {
		return t
	}
	f := func(d time.Duration) time.Duration { return time.Duration(float64(d) * scale) }
	return Timeouts{Probe: f(t.Probe), Read: f(t.Read), Write: f(t.Write)}
}

type callKind int

const (
	kindProbe callKind = iota
	kindRead
	kindWrite
)

func (t Timeouts) of(k callKind) time.Duration {
	switch k {
	case kindProbe:
		return t.Probe
	case kindRead:
		return t.Read
	case kindWrite:
		return t.Write
	}
	return t.Read
}
