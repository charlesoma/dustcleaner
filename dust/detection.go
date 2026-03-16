package dust

import (
	"fmt"
	"dustcleaner/utxo"
)

// RiskScore represents the risk level of a dust UTXO.
type RiskScore string

const (
	RiskLow    RiskScore = "LOW"
	RiskMedium RiskScore = "MEDIUM"
	RiskHigh   RiskScore = "HIGH"
)

// DetectionReason explains why a UTXO was flagged as dust.
type DetectionReason struct {
	// PrimaryReason is the main reason for detection
	PrimaryReason string
	// RiskScore indicates the severity
	RiskScore RiskScore
	// Details provides additional context
	Details []string
}

// DustDetectionResult contains a UTXO and its detection metadata.
type DustDetectionResult struct {
	UTXO   utxo.UTXO
	Reason DetectionReason
}

// AnalyzeDustUTXOs performs advanced dust attack detection with risk scoring.
func AnalyzeDustUTXOs(utxosIn []utxo.UTXO, policy Policy) []DustDetectionResult {
	var results []DustDetectionResult
	var spendableUTXOs []utxo.UTXO

	for _, u := range utxosIn {
		if u.ValueSats() > 0 {
			spendableUTXOs = append(spendableUTXOs, u)
		}
	}

	txOutputs := make(map[string][]utxo.UTXO)
	for _, u := range spendableUTXOs {
		txOutputs[u.TxID] = append(txOutputs[u.TxID], u)
	}

	addrOutputs := make(map[string][]utxo.UTXO)
	for _, u := range spendableUTXOs {
		if u.Address != "" {
			addrOutputs[u.Address] = append(addrOutputs[u.Address], u)
		}
	}

	flagged := make(map[string]bool)

	const minMultiOutputThreshold = 10
	for txID, outputs := range txOutputs {
		smallOutputs := 0
		var smallUTXOs []utxo.UTXO
		valueCounts := make(map[int64]int)
		
		for _, u := range outputs {
			threshold, _, err := GetDustThresholdForUTXO(u)
			if err != nil {
				// Fallback to policy threshold
				threshold = policy.MaxAmountSats
			}
			
			if u.ValueSats() <= threshold*2 && u.Confirmations >= policy.MinConfirmations {
				smallOutputs++
				smallUTXOs = append(smallUTXOs, u)
				valueCounts[u.ValueSats()]++
			}
		}

		maxEqualValues := 0
		for _, count := range valueCounts {
			if count > maxEqualValues {
				maxEqualValues = count
			}
		}

		if smallOutputs >= minMultiOutputThreshold {
			attackPattern := "Multi-output dust attack pattern"
			riskScore := RiskHigh
			
			if maxEqualValues >= 5 {
				attackPattern = "Multi-output dust attack with equal values"
				riskScore = RiskHigh
			}
			
			unknownSenders := 0
			for _, u := range smallUTXOs {
				if u.Address == "" {
					unknownSenders++
				}
			}
			if unknownSenders > 0 && unknownSenders >= smallOutputs/2 {
				attackPattern = "Multi-output dust attack from unknown senders"
				riskScore = RiskHigh
			}
			
			for _, u := range smallUTXOs {
				key := fmt.Sprintf("%s:%d", u.TxID, u.Vout)
				if !flagged[key] {
					flagged[key] = true
					threshold, scriptType, _ := GetDustThresholdForUTXO(u)
					details := []string{
						fmt.Sprintf("Transaction %s produced %d small outputs (multi-output dust attack)", txID[:8]+"...", smallOutputs),
						fmt.Sprintf("Value: %d sats (script type: %s, threshold: %d sats)", u.ValueSats(), scriptType, threshold),
					}
					if maxEqualValues >= 5 {
						details = append(details, fmt.Sprintf("Attack pattern: %d outputs have identical value (%d sats) - classic dust spam", maxEqualValues, u.ValueSats()))
					}
					if unknownSenders > 0 && unknownSenders >= smallOutputs/2 {
						details = append(details, fmt.Sprintf("Suspicious: %d outputs from unknown senders", unknownSenders))
					}
					
					results = append(results, DustDetectionResult{
						UTXO: u,
						Reason: DetectionReason{
							PrimaryReason: attackPattern,
							RiskScore:      riskScore,
							Details:        details,
						},
					})
				}
			}
		}
	}

	for _, u := range spendableUTXOs {
		key := fmt.Sprintf("%s:%d", u.TxID, u.Vout)
		if flagged[key] {
			continue
		}

		threshold, scriptType, err := GetDustThresholdForUTXO(u)
		if err != nil {
			threshold = policy.MaxAmountSats
			scriptType = ScriptTypeUnknown
		}

		if u.ValueSats() < threshold && u.Confirmations >= policy.MinConfirmations {
			flagged[key] = true
			risk := RiskMedium
			details := []string{
				fmt.Sprintf("Value %d sats is below script-type aware dust threshold (%d sats for %s)", u.ValueSats(), threshold, scriptType),
			}

			if u.Address != "" {
				clusterSize := len(addrOutputs[u.Address])
				if clusterSize >= 3 {
					risk = RiskHigh
					details = append(details, fmt.Sprintf("Part of cluster of %d UTXOs to address %s", clusterSize, u.Address))
				}
			}

			results = append(results, DustDetectionResult{
				UTXO: u,
				Reason: DetectionReason{
					PrimaryReason: "Below dust threshold",
					RiskScore:      risk,
					Details:        details,
				},
			})
		}
	}

	const minClusterSize = 3
	for addr, outputs := range addrOutputs {
		if len(outputs) < minClusterSize {
			continue
		}

		smallInCluster := 0
		var clusterUTXOs []utxo.UTXO
		for _, u := range outputs {
			threshold, _, err := GetDustThresholdForUTXO(u)
			if err != nil {
				threshold = policy.MaxAmountSats
			}
			if u.ValueSats() <= threshold*2 && u.Confirmations >= policy.MinConfirmations {
				smallInCluster++
				clusterUTXOs = append(clusterUTXOs, u)
			}
		}

		if smallInCluster >= minClusterSize {
			for _, u := range clusterUTXOs {
				key := fmt.Sprintf("%s:%d", u.TxID, u.Vout)
				if !flagged[key] {
					flagged[key] = true
					results = append(results, DustDetectionResult{
						UTXO: u,
						Reason: DetectionReason{
							PrimaryReason: "Address clustering pattern",
							RiskScore:      RiskMedium,
							Details: []string{
								fmt.Sprintf("Address %s has %d small UTXOs (cluster size: %d)", addr, smallInCluster, len(outputs)),
								fmt.Sprintf("Value: %d sats", u.ValueSats()),
							},
						},
					})
				}
			}
		}
	}

	return results
}

// DetectDustUTXOsWithAnalysis returns detailed analysis with risk scoring.
func DetectDustUTXOsWithAnalysis(utxosIn []utxo.UTXO) []DustDetectionResult {
	return AnalyzeDustUTXOs(utxosIn, DefaultPolicy)
}
