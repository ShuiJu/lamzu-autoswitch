//go:build windows

package main

import "fmt"

func BatteryStatusTextINCA() string {
	dev, err := FindOneLamzuDevice()
	if err != nil {
		return "Battery: N/A"
	}

	pct, chg, ok := ReadBatteryINCA(dev)
	if !ok {
		return "Battery: N/A"
	}

	state := "discharging"
	if chg {
		state = "charging"
	}
	return fmt.Sprintf("Battery: %d%% (%s)", pct, state)
}
