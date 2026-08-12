package bubble_tea

import (
	"fmt"
	"strings"

	"tungo/internal/trafficstats"
)

const statsValueWidth = 12

func formatStatsLines(prefs UIPreferences, snapshot trafficstats.Snapshot) []string {
	units := unitSystemForPrefs(prefs)
	rxRate := trafficstats.FormatRateWithSystem(snapshot.RXRate, units)
	txRate := trafficstats.FormatRateWithSystem(snapshot.TXRate, units)
	rxTotal := trafficstats.FormatTotalWithSystem(snapshot.RXBytesTotal, units)
	txTotal := trafficstats.FormatTotalWithSystem(snapshot.TXBytesTotal, units)

	return []string{
		formatStatsLine("RX", rxRate, "TX", txRate),
		formatStatsLine("Total RX", rxTotal, "Total TX", txTotal),
	}
}

func formatStatsLine(labelA, valueA, labelB, valueB string) string {
	var b strings.Builder
	b.Grow(8 + 1 + statsValueWidth + 3 + 8 + 1 + statsValueWidth)
	writeRightPadded(&b, labelA, 8)
	b.WriteByte(' ')
	writeLeftPadded(&b, valueA, statsValueWidth)
	b.WriteString(" | ")
	writeRightPadded(&b, labelB, 8)
	b.WriteByte(' ')
	writeLeftPadded(&b, valueB, statsValueWidth)
	return b.String()
}

func writeRightPadded(b *strings.Builder, s string, width int) {
	b.WriteString(s)
	for i := len(s); i < width; i++ {
		b.WriteByte(' ')
	}
}

func writeLeftPadded(b *strings.Builder, s string, width int) {
	for i := len(s); i < width; i++ {
		b.WriteByte(' ')
	}
	b.WriteString(s)
}

func unitSystemForPrefs(prefs UIPreferences) trafficstats.UnitSystem {
	if prefs.StatsUnits == StatsUnitsBytes {
		return trafficstats.UnitSystemBytes
	}
	return trafficstats.UnitSystemBinary
}

func formatCount(current, max int) string {
	if max > 0 {
		return fmt.Sprintf("%d/%d", current, max)
	}
	return fmt.Sprintf("%d", current)
}
