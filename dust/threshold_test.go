package dust

import (
	"dustcleaner/utxo"
	"testing"
)

func TestGetDustThreshold(t *testing.T) {
	tests := []struct {
		scriptType ScriptType
		expected   int64
	}{
		{ScriptTypeP2PKH, 546},
		{ScriptTypeP2WPKH, 294},
		{ScriptTypeP2SH, 546},
		{ScriptTypeP2TR, 330},
		{ScriptTypeUnknown, 546}, // Default fallback
	}

	for _, tt := range tests {
		result := GetDustThreshold(tt.scriptType)
		if result != tt.expected {
			t.Errorf("GetDustThreshold(%s) = %d, expected %d", tt.scriptType, result, tt.expected)
		}
	}
}

func TestDetectScriptType(t *testing.T) {
	tests := []struct {
		name           string
		scriptPubKey   string
		expectedType   ScriptType
		shouldError    bool
	}{
		{
			name:         "empty script",
			scriptPubKey: "",
			expectedType: ScriptTypeUnknown,
		},
		{
			name:         "invalid hex",
			scriptPubKey: "nothex",
			expectedType: ScriptTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectScriptType(tt.scriptPubKey)
			if result != tt.expectedType {
				t.Errorf("DetectScriptType(%s) = %s, expected %s", tt.scriptPubKey, result, tt.expectedType)
			}
		})
	}
}

func TestGetScriptTypeFromAddress(t *testing.T) {
	tests := []struct {
		address      string
		expectedType ScriptType
	}{
		{"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", ScriptTypeP2PKH},      // Legacy P2PKH
		{"3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", ScriptTypeP2SH},      // P2SH
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", ScriptTypeP2WPKH}, // P2WPKH
		{"bcrt1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", ScriptTypeP2WPKH}, // Regtest P2WPKH
		{"bc1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr", ScriptTypeP2TR}, // P2TR
		{"bcrt1p5cyxnuxmeuwuvkwfem96lqzszd02n6xdcjrs20cac6yqjjwudpxqkedrcr", ScriptTypeP2TR}, // Regtest P2TR
		{"", ScriptTypeUnknown},
		{"invalid", ScriptTypeUnknown},
	}

	for _, tt := range tests {
		result := GetScriptTypeFromAddress(tt.address)
		if result != tt.expectedType {
			t.Errorf("GetScriptTypeFromAddress(%s) = %s, expected %s", tt.address, result, tt.expectedType)
		}
	}
}

func TestGetDustThresholdForUTXO(t *testing.T) {
	tests := []struct {
		name        string
		utxo        utxo.UTXO
		expectError bool
	}{
		{
			name: "P2WPKH address",
			utxo: utxo.UTXO{
				Address: "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
			},
			expectError: false,
		},
		{
			name: "P2PKH address",
			utxo: utxo.UTXO{
				Address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			},
			expectError: false,
		},
		{
			name: "no address or script",
			utxo: utxo.UTXO{
				Address:      "",
				ScriptPubKey: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threshold, scriptType, err := GetDustThresholdForUTXO(tt.utxo)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if threshold <= 0 {
					t.Error("threshold should be positive")
				}
				if scriptType == ScriptTypeUnknown && tt.utxo.Address != "" {
					t.Error("should have detected script type from address")
				}
			}
		})
	}
}

func TestIsDustWithScriptType(t *testing.T) {
	// P2WPKH threshold is 294 sats
	p2wpkhUTXO := utxo.UTXO{
		Address: "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
		Amount:  0.00000293, // 293 sats - should be dust
	}

	if !IsDustWithScriptType(p2wpkhUTXO) {
		t.Error("293 sats P2WPKH UTXO should be detected as dust")
	}

	p2wpkhUTXO.Amount = 0.00000294 // 294 sats - at threshold, should NOT be dust
	if IsDustWithScriptType(p2wpkhUTXO) {
		t.Error("294 sats P2WPKH UTXO (at threshold) should NOT be detected as dust")
	}

	// Test with value above threshold
	p2wpkhUTXO.Amount = 0.00000295 // 295 sats - above threshold, should NOT be dust
	if IsDustWithScriptType(p2wpkhUTXO) {
		t.Error("295 sats P2WPKH UTXO should NOT be detected as dust")
	}

	// P2PKH threshold is 546 sats
	p2pkhUTXO := utxo.UTXO{
		Address: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		Amount:  0.00000545, // 545 sats - should be dust
	}

	if !IsDustWithScriptType(p2pkhUTXO) {
		t.Error("545 sats P2PKH UTXO should be detected as dust")
	}

	p2pkhUTXO.Amount = 0.00000546 // 546 sats - at threshold, should NOT be dust
	if IsDustWithScriptType(p2pkhUTXO) {
		t.Error("546 sats P2PKH UTXO (at threshold) should NOT be detected as dust")
	}

	// Test with value above threshold
	p2pkhUTXO.Amount = 0.00000547 // 547 sats - above threshold, should NOT be dust
	if IsDustWithScriptType(p2pkhUTXO) {
		t.Error("547 sats P2PKH UTXO should NOT be detected as dust")
	}
}
