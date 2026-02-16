package licensing

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLicense(t *testing.T) {
	t.Parallel()

	license, err := LoadLicense("any-key")
	require.NoError(t, err)
	assert.Equal(t, fleet.TierPremium, license.Tier)
	assert.Equal(t, "Kencove Farm Fence", license.Organization)
	assert.Equal(t, 999999, license.DeviceCount)
	assert.Equal(t, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), license.Expiration)
	assert.Equal(t, "Self-hosted premium", license.Note)
	assert.True(t, license.AllowDisableTelemetry)
	assert.True(t, license.IsPremium())
}

func TestLoadLicenseEmpty(t *testing.T) {
	t.Parallel()

	// Even with no key, should still return premium
	license, err := LoadLicense("")
	require.NoError(t, err)
	assert.Equal(t, fleet.TierPremium, license.Tier)
	assert.True(t, license.IsPremium())
}
