package utxo

import "math/big"

// UTXO represents a Bitcoin Core listunspent entry.
type UTXO struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Amount        float64 `json:"amount"`       // BTC
	Confirmations int64   `json:"confirmations"`
	ScriptPubKey  string  `json:"scriptPubKey"`
}

// ValueSats returns the value of the UTXO in satoshis.
func (u UTXO) ValueSats() int64 {
	return int64(u.Amount * 1e8) // BTC → sats
}

// IsDust reports whether the UTXO value is below the given dust threshold (in satoshis).
func (u UTXO) IsDust(threshold int64) bool {
	return u.ValueSats() < threshold
}

// SumAmount returns the total amount in satoshis for a slice of UTXOs.
func SumAmount(utxos []UTXO) *big.Int {
	total := big.NewInt(0)
	for _, u := range utxos {
		total.Add(total, big.NewInt(u.ValueSats()))
	}
	return total
}

