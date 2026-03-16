package dust

import (
	"dustcleaner/utxo"
)

type UTXO = utxo.UTXO

type Policy struct {
	MaxAmountSats     int64
	MinConfirmations int64
}

var DefaultPolicy = Policy{
	MaxAmountSats:    546, // default P2PKH dust threshold
	MinConfirmations: 1,
}

func SetDefaultPolicy(p Policy) {
	DefaultPolicy = p
}

// FilterDust returns the subset of UTXOs that are considered dust.
func FilterDust(utxosIn []utxo.UTXO, policy Policy) []utxo.UTXO {
	var dustUtxos []utxo.UTXO
	for _, u := range utxosIn {
		if u.ValueSats() <= 0 {
			continue
		}
		if u.ValueSats() <= policy.MaxAmountSats && u.Confirmations >= policy.MinConfirmations {
			dustUtxos = append(dustUtxos, u)
		}
	}
	return dustUtxos
}

// DetectDustUTXOs applies script-type aware dust detection with clustering heuristics.
func DetectDustUTXOs(utxosIn []UTXO) []UTXO {
	results := AnalyzeDustUTXOs(utxosIn, DefaultPolicy)
	utxos := make([]UTXO, len(results))
	for i, r := range results {
		utxos[i] = r.UTXO
	}
	return utxos
}
