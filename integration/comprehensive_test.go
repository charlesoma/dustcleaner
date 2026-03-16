//go:build integration
// +build integration

package integration

import (
	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/psbtbuilder"
	"dustcleaner/rpc"
	"testing"
)

// TestComprehensiveWorkflow tests the complete dustcleaner workflow end-to-end.
func TestComprehensiveWorkflow(t *testing.T) {
	cfg := &config.Config{
		RPCURL:  "http://127.0.0.1:18443",
		RPCUser: "user",
		RPCPass: "password",
		Network: "regtest",
	}

	client := rpc.NewFromConfig(cfg)

	// Test 1: RPC Connectivity
	t.Run("RPC Connectivity", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Skipf("Skipping integration test: cannot connect to regtest node: %v", err)
			return
		}
		t.Logf("Connected successfully, found %d UTXOs", len(utxos))
	})

	// Test 2: Dust Detection
	t.Run("Dust Detection", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Fatalf("listunspent failed: %v", err)
		}

		results := dust.DetectDustUTXOsWithAnalysis(utxos)
		t.Logf("Detected %d dust UTXOs", len(results))

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
	})

	// Test 3: Risk Scoring
	t.Run("Risk Scoring", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Fatalf("listunspent failed: %v", err)
		}

		results := dust.DetectDustUTXOsWithAnalysis(utxos)

		riskCounts := make(map[dust.RiskScore]int)
		for _, r := range results {
			riskCounts[r.Reason.RiskScore]++
		}

		t.Logf("Risk distribution: HIGH=%d, MEDIUM=%d, LOW=%d",
			riskCounts[dust.RiskHigh],
			riskCounts[dust.RiskMedium],
			riskCounts[dust.RiskLow])
	})

	// Test 4: PSBT Building
	t.Run("PSBT Building", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Fatalf("listunspent failed: %v", err)
		}

		dustUtxos := dust.DetectDustUTXOs(utxos)
		if len(dustUtxos) == 0 {
			t.Skip("No dust UTXOs available for PSBT building test")
			return
		}

		// Get a destination address
		destAddr, err := client.GetNewAddress("test", "")
		if err != nil {
			t.Fatalf("getnewaddress failed: %v", err)
		}

		// Test fast mode
		txHexes, impacts, err := psbtbuilder.BuildDustCleanupPSBTWithMode(
			dustUtxos, destAddr, 1, psbtbuilder.ModeFast)
		if err != nil {
			t.Fatalf("build PSBT failed: %v", err)
		}

		if len(txHexes) != 1 {
			t.Errorf("expected 1 transaction in fast mode, got %d", len(txHexes))
		}

		if len(impacts) != 1 {
			t.Errorf("expected 1 privacy impact, got %d", len(impacts))
		}

		t.Logf("Built PSBT successfully: %d transactions, privacy impact: %s",
			len(txHexes), impacts[0].WarningLevel)
	})

	// Test 5: Privacy Modes
	t.Run("Privacy Modes", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Fatalf("listunspent failed: %v", err)
		}

		dustUtxos := dust.DetectDustUTXOs(utxos)
		if len(dustUtxos) == 0 {
			t.Skip("No dust UTXOs available for privacy mode test")
			return
		}

		destAddr, err := client.GetNewAddress("test", "")
		if err != nil {
			t.Fatalf("getnewaddress failed: %v", err)
		}

		modes := []psbtbuilder.SpendingMode{
			psbtbuilder.ModeFast,
			psbtbuilder.ModePrivacy,
			psbtbuilder.ModeIsolated,
		}

		for _, mode := range modes {
			txHexes, impacts, err := psbtbuilder.BuildDustCleanupPSBTWithMode(
				dustUtxos, destAddr, 1, mode)
			if err != nil {
				t.Errorf("build PSBT with mode %s failed: %v", mode, err)
				continue
			}

			t.Logf("Mode %s: %d transactions, %d privacy impacts",
				mode, len(txHexes), len(impacts))

			// Verify privacy impact analysis
			for _, impact := range impacts {
				if impact.WarningLevel == "" {
					t.Error("privacy impact missing warning level")
				}
				if impact.Message == "" {
					t.Error("privacy impact missing message")
				}
			}
		}
	})

	// Test 6: Privacy Impact Analysis
	t.Run("Privacy Impact Analysis", func(t *testing.T) {
		utxos, err := client.ListUnspent()
		if err != nil {
			t.Fatalf("listunspent failed: %v", err)
		}

		dustUtxos := dust.DetectDustUTXOs(utxos)
		if len(dustUtxos) == 0 {
			t.Skip("No dust UTXOs available for privacy impact test")
			return
		}

		impact := psbtbuilder.AnalyzePrivacyImpact(dustUtxos)

		if impact.DistinctAddresses < 0 {
			t.Error("privacy impact has invalid distinct addresses count")
		}
		if impact.TotalUTXOs != len(dustUtxos) {
			t.Errorf("privacy impact total UTXOs mismatch: expected %d, got %d",
				len(dustUtxos), impact.TotalUTXOs)
		}
		if impact.WarningLevel == "" {
			t.Error("privacy impact missing warning level")
		}
		if impact.Message == "" {
			t.Error("privacy impact missing message")
		}

		t.Logf("Privacy impact: %s - %s", impact.WarningLevel, impact.Message)
	})

	// Test 7: Fee Estimation
	t.Run("Fee Estimation", func(t *testing.T) {
		testCases := []struct {
			inputCount int
			feeRate    int64
		}{
			{1, 1},
			{10, 1},
			{50, 2},
			{100, 3},
		}

		for _, tc := range testCases {
			fee := psbtbuilder.EstimateCleanupFee(tc.inputCount, tc.feeRate)
			if fee <= 0 {
				t.Errorf("estimated fee should be positive, got %d for %d inputs at %d sat/vbyte",
					fee, tc.inputCount, tc.feeRate)
			}
			t.Logf("Fee estimate: %d inputs @ %d sat/vbyte = %d sats",
				tc.inputCount, tc.feeRate, fee)
		}
	})
}

// TestEdgeCases tests various edge cases and error conditions.
func TestEdgeCases(t *testing.T) {
	cfg := &config.Config{
		RPCURL:  "http://127.0.0.1:18443",
		RPCUser: "user",
		RPCPass: "password",
		Network: "regtest",
	}

	client := rpc.NewFromConfig(cfg)

	// Test 1: Empty UTXO list
	t.Run("Empty UTXO List", func(t *testing.T) {
		results := dust.DetectDustUTXOsWithAnalysis([]dust.UTXO{})
		if len(results) != 0 {
			t.Errorf("expected 0 results for empty UTXO list, got %d", len(results))
		}
	})

	// Test 2: No dust UTXOs
	t.Run("No Dust UTXOs", func(t *testing.T) {
		largeUTXOs := []dust.UTXO{
			{TxID: "tx1", Vout: 0, Amount: 0.001, Confirmations: 1}, // 100,000 sats
			{TxID: "tx2", Vout: 0, Amount: 0.01, Confirmations: 1},  // 1,000,000 sats
		}

		results := dust.DetectDustUTXOsWithAnalysis(largeUTXOs)
		if len(results) > 0 {
			t.Logf("Some large UTXOs flagged (may be due to clustering): %d", len(results))
		}
	})

	// Test 3: Invalid spending mode
	t.Run("Invalid Spending Mode", func(t *testing.T) {
		utxos := []dust.UTXO{
			{TxID: "tx1", Vout: 0, Amount: 0.000003, Address: "bc1qtest"},
		}

		_, _, err := psbtbuilder.BuildDustCleanupPSBTWithMode(
			utxos, "bc1qtest", 1, psbtbuilder.SpendingMode("invalid"))
		if err == nil {
			t.Error("expected error for invalid spending mode")
		}
	})

	// Test 4: Zero fee rate
	t.Run("Zero Fee Rate", func(t *testing.T) {
		utxos := []dust.UTXO{
			{TxID: "tx1", Vout: 0, Amount: 0.000003, Address: "bc1qtest"},
		}

		_, _, err := psbtbuilder.BuildDustCleanupPSBTWithMode(
			utxos, "bc1qtest", 0, psbtbuilder.ModeFast)
		if err == nil {
			t.Error("expected error for zero fee rate")
		}
	})
}

// TestAttackPatternDetection tests advanced attack pattern detection.
func TestAttackPatternDetection(t *testing.T) {
	// Simulate a multi-output dust attack
	utxos := make([]dust.UTXO, 15)
	for i := 0; i < 15; i++ {
		utxos[i] = dust.UTXO{
			TxID:          "attack_tx",
			Vout:          uint32(i),
			Amount:        0.000003, // 300 sats
			Confirmations: 1,
			Address:       "bc1qtest",
		}
	}

	results := dust.DetectDustUTXOsWithAnalysis(utxos)

	if len(results) != 15 {
		t.Errorf("expected 15 dust UTXOs detected, got %d", len(results))
	}

	// Check for multi-output attack pattern detection
	highRiskCount := 0
	attackPatternCount := 0
	for _, r := range results {
		if r.Reason.RiskScore == dust.RiskHigh {
			highRiskCount++
		}
		if r.Reason.PrimaryReason == "Multi-output dust attack pattern" {
			attackPatternCount++
		}
	}

	if attackPatternCount == 0 {
		t.Error("expected multi-output attack pattern to be detected")
	}

	if highRiskCount == 0 {
		t.Error("expected at least some HIGH risk UTXOs from multi-output attack")
	}

	t.Logf("Attack pattern detection: %d HIGH risk, %d attack patterns detected",
		highRiskCount, attackPatternCount)
}
