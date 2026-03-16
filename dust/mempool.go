package dust

import (
	"dustcleaner/utxo"
	"fmt"
)

type MempoolDustAnalysis struct {
	UnconfirmedDustUTXOs   []utxo.UTXO
	SuspiciousTransactions []string
	MempoolDustCount       int
}

func AnalyzeMempoolDust(unconfirmedUTXOs []utxo.UTXO) MempoolDustAnalysis {
	analysis := MempoolDustAnalysis{
		UnconfirmedDustUTXOs: []utxo.UTXO{},
		SuspiciousTransactions: []string{},
	}

	txOutputs := make(map[string][]utxo.UTXO)
	for _, u := range unconfirmedUTXOs {
		txOutputs[u.TxID] = append(txOutputs[u.TxID], u)
	}

	for txID, outputs := range txOutputs {
		smallOutputs := 0
		valueCounts := make(map[int64]int)
		
		for _, u := range outputs {
			threshold, _, err := GetDustThresholdForUTXO(u)
			if err != nil {
				threshold = DefaultPolicy.MaxAmountSats
			}
			
			if u.ValueSats() < threshold*2 {
				smallOutputs++
				valueCounts[u.ValueSats()]++
			}
		}

		if smallOutputs >= 5 {
			analysis.SuspiciousTransactions = append(analysis.SuspiciousTransactions, txID)
			
			maxEqualValues := 0
			for _, count := range valueCounts {
				if count > maxEqualValues {
					maxEqualValues = count
				}
			}
			
			if maxEqualValues >= 3 {
				for _, u := range outputs {
					threshold, _, err := GetDustThresholdForUTXO(u)
					if err != nil {
						threshold = DefaultPolicy.MaxAmountSats
					}
					if u.ValueSats() < threshold {
						analysis.UnconfirmedDustUTXOs = append(analysis.UnconfirmedDustUTXOs, u)
					}
				}
			}
		}
	}

	analysis.MempoolDustCount = len(analysis.UnconfirmedDustUTXOs)
	return analysis
}

// FormatMempoolWarning returns a warning message about unconfirmed dust.
func (m MempoolDustAnalysis) FormatMempoolWarning() string {
	if m.MempoolDustCount == 0 {
		return ""
	}
	return fmt.Sprintf("Warning: %d unconfirmed dust outputs detected in mempool. Consider waiting for confirmations before cleanup.", m.MempoolDustCount)
}
