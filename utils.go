package tls_client

import (
	"fmt"
	"math"
	"math/rand/v2"
)

func Int64ToInt(x int64) (int, error) {
	if x < math.MinInt || x > math.MaxInt {
		return 0, fmt.Errorf("int64 value %d out of int range [%d, %d]", x, math.MinInt, math.MaxInt)
	}
	return int(x), nil
}

// generateGREASESettingID generates a valid GREASE setting ID
// GREASE IDs are of the form 0x1f * N + 0x21 where N is random.
// Uses math/rand/v2 (not crypto/rand) — GREASE values don't need
// cryptographic randomness and v2 avoids /dev/urandom syscalls.
func generateGREASESettingID() uint64 {
	// Generate large N values similar to Chrome (produces 10-11 digit IDs)
	// N between 1,000,000,000 and 10,000,000,000
	n := uint64(1000000000) + rand.Uint64N(9000000000)
	return 0x1f*n + 0x21
}

// generateGREASESettingValue generates a random non-zero 32-bit value for GREASE
func generateGREASESettingValue() uint64 {
	val := rand.Uint32()
	// Chrome never sends 0
	if val == 0 {
		val = 1
	}
	return uint64(val)
}
