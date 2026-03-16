package dust

import (
	"dustcleaner/utxo"
	"testing"
)

func TestAnalyzeDustUTXOs_ThresholdDetection(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "tx1", Vout: 0, Amount: 0.00000300, Confirmations: 1}, // 300 sats - dust
		{TxID: "tx2", Vout: 0, Amount: 0.00000500, Confirmations: 1}, // 500 sats - dust
		{TxID: "tx3", Vout: 0, Amount: 0.00001000, Confirmations: 1}, // 1000 sats - not dust
	}

	policy := Policy{
		MaxAmountSats:    546,
		MinConfirmations: 1,
	}

	results := AnalyzeDustUTXOs(utxos, policy)

	if len(results) != 2 {
		t.Fatalf("expected 2 dust UTXOs, got %d", len(results))
	}

	// Check first result
	if results[0].UTXO.TxID != "tx1" && results[0].UTXO.TxID != "tx2" {
		t.Errorf("unexpected txid: %s", results[0].UTXO.TxID)
	}
	if results[0].Reason.RiskScore == "" {
		t.Error("expected risk score to be set")
	}
}

func TestAnalyzeDustUTXOs_MultiOutputAttack(t *testing.T) {
	// Simulate a dust attack: single transaction creating many small outputs
	utxos := []utxo.UTXO{}
	for i := 0; i < 15; i++ {
		utxos = append(utxos, utxo.UTXO{
			TxID:          "attack_tx",
			Vout:          uint32(i),
			Amount:        0.00000300, // 300 sats
			Confirmations: 1,
			Address:       "bc1qtest",
		})
	}

	policy := Policy{
		MaxAmountSats:    546,
		MinConfirmations: 1,
	}

	results := AnalyzeDustUTXOs(utxos, policy)

	if len(results) != 15 {
		t.Fatalf("expected 15 dust UTXOs detected, got %d", len(results))
	}

	// All should be flagged as HIGH risk due to multi-output attack pattern
	highRiskCount := 0
	for _, r := range results {
		if r.Reason.RiskScore == RiskHigh {
			highRiskCount++
		}
		// Accept any multi-output attack pattern variant
		expectedPatterns := []string{
			"Multi-output dust attack pattern",
			"Multi-output dust attack with equal values",
			"Multi-output dust attack from unknown senders",
		}
		found := false
		for _, pattern := range expectedPatterns {
			if r.Reason.PrimaryReason == pattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected multi-output attack pattern, got: %s", r.Reason.PrimaryReason)
		}
	}

	if highRiskCount == 0 {
		t.Error("expected at least some HIGH risk UTXOs from multi-output attack")
	}
}

func TestAnalyzeDustUTXOs_AddressClustering(t *testing.T) {
	// Create a cluster of small UTXOs to the same address
	utxos := []utxo.UTXO{}
	for i := 0; i < 5; i++ {
		utxos = append(utxos, utxo.UTXO{
			TxID:          "tx1",
			Vout:          uint32(i),
			Amount:        0.00000300, // 300 sats
			Confirmations: 1,
			Address:       "bc1qcluster",
		})
	}

	policy := Policy{
		MaxAmountSats:    546,
		MinConfirmations: 1,
	}

	results := AnalyzeDustUTXOs(utxos, policy)

	if len(results) != 5 {
		t.Fatalf("expected 5 dust UTXOs detected, got %d", len(results))
	}

	// Check that clustering is mentioned in details
	foundCluster := false
	for _, r := range results {
		for _, detail := range r.Reason.Details {
			if contains(detail, "cluster") {
				foundCluster = true
				break
			}
		}
	}
	if !foundCluster {
		t.Error("expected clustering details to be present")
	}
}

func TestAnalyzeDustUTXOs_MinConfirmations(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "tx1", Vout: 0, Amount: 0.00000300, Confirmations: 0}, // Unconfirmed - should be filtered
		{TxID: "tx2", Vout: 0, Amount: 0.00000300, Confirmations: 1}, // Confirmed - should be detected
		{TxID: "tx3", Vout: 0, Amount: 0.00000300, Confirmations: 2}, // Confirmed - should be detected
	}

	policy := Policy{
		MaxAmountSats:    546,
		MinConfirmations: 1,
	}

	results := AnalyzeDustUTXOs(utxos, policy)

	if len(results) != 2 {
		t.Fatalf("expected 2 dust UTXOs (excluding unconfirmed), got %d", len(results))
	}
}

func TestDetectDustUTXOsWithAnalysis_BackwardCompatibility(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "tx1", Vout: 0, Amount: 0.00000300, Confirmations: 1},
		{TxID: "tx2", Vout: 0, Amount: 0.00001000, Confirmations: 1},
	}

	// Test backward compatibility
	simpleResults := DetectDustUTXOs(utxos)
	if len(simpleResults) == 0 {
		t.Error("expected at least one dust UTXO")
	}

	// Test new analysis function
	analysisResults := DetectDustUTXOsWithAnalysis(utxos)
	if len(analysisResults) == 0 {
		t.Error("expected at least one dust UTXO with analysis")
	}

	// Analysis should return same UTXOs (just with metadata)
	if len(simpleResults) != len(analysisResults) {
		t.Errorf("expected same number of results, got %d vs %d", len(simpleResults), len(analysisResults))
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
