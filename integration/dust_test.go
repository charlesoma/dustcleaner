//go:build integration
// +build integration

package integration

import (
	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/rpc"
	"testing"
)

// TestDustDetection_Regtest requires a running Bitcoin Core regtest node.
// Run with: go test -tags=integration ./integration
func TestDustDetection_Regtest(t *testing.T) {
	cfg := &config.Config{
		RPCURL:  "http://127.0.0.1:18443",
		RPCUser: "user",
		RPCPass: "password",
		Network: "regtest",
	}

	client := rpc.NewFromConfig(cfg)

	// Fetch UTXOs
	utxos, err := client.ListUnspent()
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to regtest node: %v", err)
		return
	}

	// Run detection
	results := dust.DetectDustUTXOsWithAnalysis(utxos)

	t.Logf("Found %d UTXOs, %d flagged as dust", len(utxos), len(results))

	// Verify results have proper structure
	for _, r := range results {
		if r.UTXO.TxID == "" {
			t.Error("detection result missing txid")
		}
		if r.Reason.PrimaryReason == "" {
			t.Error("detection result missing reason")
		}
		if r.Reason.RiskScore == "" {
			t.Error("detection result missing risk score")
		}
	}
}

// TestDustDetection_MultiOutputAttack simulates a dust attack pattern.
func TestDustDetection_MultiOutputAttack(t *testing.T) {
	cfg := &config.Config{
		RPCURL:  "http://127.0.0.1:18443",
		RPCUser: "user",
		RPCPass: "password",
		Network: "regtest",
	}

	client := rpc.NewFromConfig(cfg)

	// Get a test address
	addr, err := client.GetNewAddress("test", "")
	if err != nil {
		t.Skipf("Skipping integration test: cannot connect to regtest node: %v", err)
		return
	}

	utxos, err := client.ListUnspent()
	if err != nil {
		t.Fatalf("listunspent failed: %v", err)
	}

	// Run detection
	results := dust.DetectDustUTXOsWithAnalysis(utxos)

	// Check if multi-output attack pattern is detected
	foundMultiOutput := false
	for _, r := range results {
		if r.Reason.PrimaryReason == "Multi-output dust attack pattern" {
			foundMultiOutput = true
			if r.Reason.RiskScore != dust.RiskHigh {
				t.Errorf("expected HIGH risk for multi-output attack, got %s", r.Reason.RiskScore)
			}
		}
	}

	t.Logf("Multi-output attack pattern detected: %v", foundMultiOutput)
	t.Logf("Total dust UTXOs: %d", len(results))
}
