package licensing

import (
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

// LoadLicense returns a self-hosted premium license for Kencove.
func LoadLicense(licenseKey string) (*fleet.LicenseInfo, error) {
	return &fleet.LicenseInfo{
		Tier:                  fleet.TierPremium,
		Organization:          "Kencove Farm Fence",
		DeviceCount:           999999,
		Expiration:            time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC),
		Note:                  "Self-hosted premium",
		AllowDisableTelemetry: true,
	}, nil
}
