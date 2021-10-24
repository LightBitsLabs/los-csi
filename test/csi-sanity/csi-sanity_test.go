package main

import (
	"os"
	"testing"

	"github.com/kubernetes-csi/csi-test/v3/pkg/sanity"
	"github.com/lightbitslabs/los-csi/pkg/driver"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// TestCSISanity runs the full CSI sanity tests
func TestCSISanity(t *testing.T) {

	// LightOS cluster params
	mgmtEndpoint := getEnv("CSI_SANITY_MGMT_ENDPOINT", "")
	replicas := getEnv("CSI_SANITY_REPLICAS", "1")
	compression := getEnv("CSI_SANITY_COMPRESSION", "disabled")
	// We can't run tests without LB cluster...
	if mgmtEndpoint == "" {
		t.Skip("mandatory parameter mgmt-endpoint missing, skipping CSISanity")
	}

	nodeID, err := os.Hostname()
	if err != nil {
		t.Errorf("Failed to get hostname")
		t.FailNow()
	}
	nodeID = nodeID + ".node"

	// Setup the full driver and its environment
	cfg := driver.Config{
		NodeID:        nodeID,
		Endpoint:      "unix:///tmp/csi.sock",
		DefaultFS:     "ext4",
		LogLevel:      "info",
		LogRole:       "node",
		LogFormat:     "json",
		LogTimestamps: false,
		Transport:     "tcp",
		SquelchPanics: false,
		PrettyJson:    true,
	}

	d, err := driver.New(cfg)
	if err != nil {
		t.Logf("Creating driver failed with error %s", err.Error())
		t.Fail()
	}

	go func() {
		if err := d.Run(); err != nil {
			t.Logf("Running driver failed with error %s", err.Error())
			t.Fail()

		}
	}()
	config := sanity.NewTestConfig()
	// Set configuration options as needed
	config.Address = cfg.Endpoint
	config.IdempotentCount = 5
	config.JUnitFile = "junit_01.xml"
	config.TestVolumeParameters = make(map[string]string)
	config.TestVolumeParameters["mgmt-endpoint"] = mgmtEndpoint
	config.TestVolumeParameters["replica-count"] = replicas
	config.TestVolumeParameters["compression"] = compression

	// Now call the test suite
	sanity.Test(t, config)
}
