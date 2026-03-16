package dust

import (
	"encoding/hex"
	"fmt"

	"dustcleaner/utxo"

	"github.com/btcsuite/btcd/txscript"
)

// ScriptType represents the type of Bitcoin script.
type ScriptType string

const (
	ScriptTypeP2PKH  ScriptType = "P2PKH"  // Pay-to-PubKey-Hash
	ScriptTypeP2WPKH ScriptType = "P2WPKH" // Pay-to-Witness-PubKey-Hash
	ScriptTypeP2SH   ScriptType = "P2SH"   // Pay-to-Script-Hash
	ScriptTypeP2TR   ScriptType = "P2TR"   // Pay-to-Taproot
	ScriptTypeUnknown ScriptType = "Unknown"
)

// GetDustThreshold returns the dust threshold in satoshis for a given script type.
func GetDustThreshold(scriptType ScriptType) int64 {
	switch scriptType {
	case ScriptTypeP2PKH:
		return 546
	case ScriptTypeP2WPKH:
		return 294
	case ScriptTypeP2SH:
		return 546
	case ScriptTypeP2TR:
		return 330
	default:
		return 546
	}
}

// DetectScriptType determines the script type from a scriptPubKey hex string.
func DetectScriptType(scriptPubKeyHex string) ScriptType {
	if scriptPubKeyHex == "" {
		return ScriptTypeUnknown
	}

	scriptBytes, err := hex.DecodeString(scriptPubKeyHex)
	if err != nil {
		return ScriptTypeUnknown
	}

	scriptClass := txscript.GetScriptClass(scriptBytes)

	switch scriptClass {
	case txscript.PubKeyHashTy:
		return ScriptTypeP2PKH
	case txscript.WitnessV0PubKeyHashTy:
		return ScriptTypeP2WPKH
	case txscript.ScriptHashTy:
		return ScriptTypeP2SH
	case txscript.WitnessV0ScriptHashTy:
		return ScriptTypeP2SH
	case txscript.WitnessV1TaprootTy:
		return ScriptTypeP2TR
	default:
		return ScriptTypeUnknown
	}
}

// GetUTXODustThreshold returns the appropriate dust threshold for a UTXO based on its script type.
func GetUTXODustThreshold(u utxo.UTXO) int64 {
	scriptType := DetectScriptType(u.ScriptPubKey)
	return GetDustThreshold(scriptType)
}

// IsDustWithScriptType checks if a UTXO is dust using script-type-aware threshold.
func IsDustWithScriptType(u utxo.UTXO) bool {
	threshold, _, err := GetDustThresholdForUTXO(u)
	if err != nil {
		threshold = 546
	}
	return u.ValueSats() < threshold
}

// GetScriptTypeFromAddress infers script type from address format.
func GetScriptTypeFromAddress(address string) ScriptType {
	if address == "" {
		return ScriptTypeUnknown
	}

	if len(address) >= 1 {
		switch address[0] {
		case '1':
			return ScriptTypeP2PKH
		case '3':
			return ScriptTypeP2SH
		}
	}

	if len(address) >= 6 {
		if address[:6] == "bcrt1q" {
			return ScriptTypeP2WPKH
		}
		if address[:6] == "bcrt1p" {
			return ScriptTypeP2TR
		}
	}
	if len(address) >= 4 {
		if address[:4] == "bc1q" {
			return ScriptTypeP2WPKH
		}
		if address[:4] == "bc1p" {
			return ScriptTypeP2TR
		}
	}

	return ScriptTypeUnknown
}

// GetDustThresholdForUTXO returns the dust threshold for a UTXO.
func GetDustThresholdForUTXO(u utxo.UTXO) (int64, ScriptType, error) {
	var scriptType ScriptType

	if u.ScriptPubKey != "" {
		scriptType = DetectScriptType(u.ScriptPubKey)
	} else if u.Address != "" {
		scriptType = GetScriptTypeFromAddress(u.Address)
	} else {
		return 546, ScriptTypeUnknown, fmt.Errorf("cannot determine script type: no scriptPubKey or address")
	}

	threshold := GetDustThreshold(scriptType)
	return threshold, scriptType, nil
}
