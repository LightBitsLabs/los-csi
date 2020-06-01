// +build have_net,have_lb,manual

package lbgrpc_test

import (
	"fmt"
	"testing"
	"time"
)

const (
	maxOutage = 3 * time.Second
)

// presumably whoever is running this test is also manipulating the target
// LightOS cluster in the background, to verify that the client can hop
// from one mgmt API server node to the next, etc.
func TestPollRemoteOk(t *testing.T) {
	clnt := mkClient(t)

	delay := 1 * time.Second
	runtime := 590 * time.Second
	if testing.Short() {
		runtime = 120 * time.Second
	}
	deadline := time.Now().Add(runtime)

	var outStart time.Time
	missed := 0
	reportOutage := func() {
		if !outStart.IsZero() {
			outage := time.Now().Sub(outStart)
			msg := fmt.Sprintf("spotted outage of %s, %d queries failed",
					outage, missed)
			if outage > maxOutage {
				t.Errorf("BUG: %s", msg)
			} else {
				t.Logf("HMM: %s", msg)
			}
			missed = 0
			outStart = time.Time{}
		}
	}

	for time.Now().Before(deadline) {
		err := clnt.RemoteOk(getCtx())
		if err != nil {
			t.Logf("HMM: RemoteOk() failed with: '%s'", err)
			if outStart.IsZero() {
				outStart = time.Now()
			}
			missed++
		} else {
			reportOutage()
			if testing.Verbose() {
				t.Logf("OK: RemoteOk() succeeded")
			}
		}
		time.Sleep(delay)
	}
	reportOutage()

	clnt.Close()
	err := clnt.RemoteOk(getCtx())
	if err == nil {
		t.Errorf("BUG: RemoteOk() succeeded on closed client")
	} else {
		t.Logf("OK: RemoteOk() failed after disconnect with: '%s'", err)
	}
}
