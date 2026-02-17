package trafficstats

import "fmt"

type UnitSystem string

const (
	UnitSystemBinary UnitSystem = "binary"
	UnitSystemBytes  UnitSystem = "bytes"
)

func FormatRate(bytesPerSecond uint64) string {
	return FormatRateWithSystem(bytesPerSecond, UnitSystemBinary)
}

func FormatTotal(bytes uint64) string {
	return FormatTotalWithSystem(bytes, UnitSystemBinary)
}

func FormatRateWithSystem(bytesPerSecond uint64, system UnitSystem) string {
	return formatBySystem(float64(bytesPerSecond), "/s", system)
}

func FormatTotalWithSystem(bytes uint64, system UnitSystem) string {
	return formatBySystem(float64(bytes), "", system)
}

func formatBySystem(value float64, suffix string, system UnitSystem) string {
	base := 1024.0
	units := []string{"B", "KiB", "MiB", "GiB"}
	if system == UnitSystemBytes {
		base = 1000
		units = []string{"B", "KB", "MB", "GB"}
	}

	unitIdx := 0
	for value >= base && unitIdx < len(units)-1 {
		value /= base
		unitIdx++
	}

	if unitIdx == 0 {
		return fmt.Sprintf("%.0f %s%s", value, units[unitIdx], suffix)
	}
	return fmt.Sprintf("%.1f %s%s", value, units[unitIdx], suffix)
}
