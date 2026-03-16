package psbtbuilder

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"dustcleaner/utxo"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

type UTXO = utxo.UTXO

const DustThresholdSats int64 = 546

// EstimateCleanupFee estimates the fee for a consolidation transaction.
func EstimateCleanupFee(inputCount int, feeRate int64) int64 {
	if inputCount <= 0 || feeRate <= 0 {
		return 0
	}

	const overheadVBytes = 10
	const inputVBytes = 68
	const outputVBytes = 31

	vsize := int64(overheadVBytes + inputCount*inputVBytes + 1*outputVBytes)
	return vsize * feeRate
}

// BuildDustCleanupPSBT builds an unsigned transaction that spends the provided UTXOs.
func BuildDustCleanupPSBT(
	utxosIn []UTXO,
	destinationAddress string,
	feeRate int64,
) (string, error) {
	if len(utxosIn) == 0 {
		return "", fmt.Errorf("no UTXOs provided")
	}
	if feeRate <= 0 {
		return "", fmt.Errorf("feeRate must be positive")
	}

	var totalIn int64
	for _, u := range utxosIn {
		totalIn += u.ValueSats()
	}

	tx := wire.NewMsgTx(wire.TxVersion)

	for _, u := range utxosIn {
		hash, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			return "", fmt.Errorf("parse txid %s: %w", u.TxID, err)
		}
		outPoint := wire.OutPoint{
			Hash:  *hash,
			Index: u.Vout,
		}
		txIn := wire.NewTxIn(&outPoint, nil, nil)
		tx.AddTxIn(txIn)
	}

	fee := int64(0)
	if destinationAddress != "" {
		fee = EstimateCleanupFee(len(utxosIn), feeRate)
	}
	if fee >= totalIn {
		return "", fmt.Errorf("fee %d sats exceeds or equals total input %d sats", fee, totalIn)
	}

	if destinationAddress != "" {
		destAddr, err := btcutil.DecodeAddress(destinationAddress, &chaincfg.MainNetParams)
		if err != nil {
			return "", fmt.Errorf("decode destination address: %w", err)
		}
		pkScript, err := txscript.PayToAddrScript(destAddr)
		if err != nil {
			return "", fmt.Errorf("build pkScript: %w", err)
		}

		value := totalIn - fee
		if value <= DustThresholdSats {
			return "", fmt.Errorf("output value %d sats is at or below dust threshold %d", value, DustThresholdSats)
		}
		txOut := wire.NewTxOut(value, pkScript)
		tx.AddTxOut(txOut)
	}

	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("serialize tx: %w", err)
	}

	return hex.EncodeToString(buf.Bytes()), nil
}

