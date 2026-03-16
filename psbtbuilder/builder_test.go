package psbtbuilder

import (
	"dustcleaner/utxo"
	"strings"
	"testing"
)

func TestEstimateCleanupFee(t *testing.T) {
	tests := []struct {
		name       string
		inputCount int
		feeRate    int64
		expected   int64
	}{
		{
			name:       "single input",
			inputCount: 1,
			feeRate:    1,
			expected:   109, // 10 + 68*1 + 31 = 109
		},
		{
			name:       "multiple inputs",
			inputCount: 10,
			feeRate:    2,
			expected:   1442, // (10 + 68*10 + 31) * 2 = 721 * 2 = 1442
		},
		{
			name:       "zero inputs",
			inputCount: 0,
			feeRate:    1,
			expected:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateCleanupFee(tt.inputCount, tt.feeRate)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestBuildDustCleanupPSBT_Basic(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "a" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000}, // 1000 sats
		{TxID: "b" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000}, // 1000 sats
	}

	destAddr := "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	feeRate := int64(1)

	txHex, err := BuildDustCleanupPSBT(utxos, destAddr, feeRate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if txHex == "" {
		t.Error("expected non-empty transaction hex")
	}

	if len(txHex)%2 != 0 {
		t.Error("transaction hex should have even length")
	}
}

func TestBuildDustCleanupPSBT_NoUTXOs(t *testing.T) {
	utxos := []utxo.UTXO{}
	destAddr := "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"

	_, err := BuildDustCleanupPSBT(utxos, destAddr, 1)
	if err == nil {
		t.Error("expected error for empty UTXO list")
	}
}

func TestGroupUTXOsByAddress(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "tx1", Vout: 0, Address: "addr1"},
		{TxID: "tx2", Vout: 0, Address: "addr1"},
		{TxID: "tx3", Vout: 0, Address: "addr2"},
		{TxID: "tx4", Vout: 0, Address: ""}, // No address
	}

	clusters := GroupUTXOsByAddress(utxos)

	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}

	// Check addr1 cluster
	foundAddr1 := false
	for _, cluster := range clusters {
		if cluster.Address == "addr1" {
			foundAddr1 = true
			if len(cluster.UTXOs) != 2 {
				t.Errorf("expected 2 UTXOs for addr1, got %d", len(cluster.UTXOs))
			}
		}
	}
	if !foundAddr1 {
		t.Error("expected to find addr1 cluster")
	}
}

func TestAnalyzePrivacyImpact(t *testing.T) {
	tests := []struct {
		name          string
		utxos         []utxo.UTXO
		expectedLevel string
		expectedAddrs int
	}{
		{
			name: "single address - low risk",
			utxos: []utxo.UTXO{
				{Address: "addr1"},
				{Address: "addr1"},
			},
			expectedLevel: "LOW",
			expectedAddrs: 1,
		},
		{
			name: "multiple addresses - medium risk",
			utxos: []utxo.UTXO{
				{Address: "addr1"},
				{Address: "addr2"},
			},
			expectedLevel: "MEDIUM",
			expectedAddrs: 2,
		},
		{
			name: "many addresses - high risk",
			utxos: []utxo.UTXO{
				{Address: "addr1"},
				{Address: "addr2"},
				{Address: "addr3"},
				{Address: "addr4"},
				{Address: "addr5"},
			},
			expectedLevel: "HIGH",
			expectedAddrs: 5,
		},
		{
			name: "no addresses - unknown",
			utxos: []utxo.UTXO{
				{Address: ""},
				{Address: ""},
			},
			expectedLevel: "UNKNOWN",
			expectedAddrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impact := AnalyzePrivacyImpact(tt.utxos)
			if impact.WarningLevel != tt.expectedLevel {
				t.Errorf("expected warning level %s, got %s", tt.expectedLevel, impact.WarningLevel)
			}
			if impact.DistinctAddresses != tt.expectedAddrs {
				t.Errorf("expected %d distinct addresses, got %d", tt.expectedAddrs, impact.DistinctAddresses)
			}
			if impact.Message == "" {
				t.Error("expected non-empty message")
			}
		})
	}
}

func TestBuildDustCleanupPSBTWithMode_Fast(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "a" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000, Address: "addr1"},
		{TxID: "b" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000, Address: "addr2"},
	}

	destAddr := "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	feeRate := int64(1)

	txHexes, impacts, err := BuildDustCleanupPSBTWithMode(utxos, destAddr, feeRate, ModeFast)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(txHexes) != 1 {
		t.Fatalf("expected 1 transaction in fast mode, got %d", len(txHexes))
	}

	if len(impacts) != 1 {
		t.Fatalf("expected 1 privacy impact, got %d", len(impacts))
	}

	// Fast mode should show medium/high privacy risk when consolidating multiple addresses
	if impacts[0].DistinctAddresses != 2 {
		t.Errorf("expected 2 distinct addresses, got %d", impacts[0].DistinctAddresses)
	}
}

func TestBuildDustCleanupPSBTWithMode_Privacy(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "a" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000, Address: "addr1"},
		{TxID: "b" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000, Address: "addr1"},
		{TxID: "c" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000, Address: "addr2"},
	}

	destAddr := "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	feeRate := int64(1)

	txHexes, impacts, err := BuildDustCleanupPSBTWithMode(utxos, destAddr, feeRate, ModePrivacy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Privacy mode should create one transaction per address cluster
	if len(txHexes) != 2 {
		t.Fatalf("expected 2 transactions (one per address), got %d", len(txHexes))
	}

	if len(impacts) != 2 {
		t.Fatalf("expected 2 privacy impacts, got %d", len(impacts))
	}
}

func TestBuildDustCleanupPSBTWithMode_InvalidMode(t *testing.T) {
	utxos := []utxo.UTXO{
		{TxID: "a" + strings.Repeat("0", 63), Vout: 0, Amount: 0.00001000},
	}

	_, _, err := BuildDustCleanupPSBTWithMode(utxos, "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", 1, SpendingMode("invalid"))
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}
