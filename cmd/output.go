package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"dustcleaner/dust"
	"dustcleaner/utxo"
)

type OutputMode string

const (
	OutputModeTable   OutputMode = "table"
	OutputModeSummary OutputMode = "summary"
	OutputModeVerbose OutputMode = "verbose"
	OutputModeJSON    OutputMode = "json"
)

type ScanOutput struct {
	TotalUTXOs       int                        `json:"total_utxos"`
	SpendableUTXOs   int                        `json:"spendable_utxos"`
	ZeroValueUTXOs   int                        `json:"zero_value_utxos_ignored"`
	DustUTXOsCount   int                        `json:"dust_utxos_count"`
	DustUTXOs        []dust.DustDetectionResult `json:"dust_utxos,omitempty"`
	RiskDistribution map[string]int              `json:"risk_distribution"`
}

func PrintScanResults(utxos []utxo.UTXO, results []dust.DustDetectionResult, mode OutputMode, zeroValueCount int) {
	switch mode {
	case OutputModeJSON:
		printScanResultsJSON(utxos, results, zeroValueCount)
	case OutputModeSummary:
		printScanResultsSummary(utxos, results, zeroValueCount)
	case OutputModeVerbose:
		printScanResultsVerbose(utxos, results, zeroValueCount)
	default:
		printScanResultsTable(utxos, results, zeroValueCount)
	}
}

func printScanResultsTable(utxos []utxo.UTXO, results []dust.DustDetectionResult, zeroValueCount int) {
	if len(results) == 0 {
		fmt.Fprintf(os.Stdout, "TXID  VOUT  VALUE(sats)  ADDRESS\n\n")
		if zeroValueCount > 0 {
			fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n", zeroValueCount)
		}
		fmt.Fprintf(os.Stdout, "Total spendable UTXOs: %d\n", len(utxos))
		fmt.Fprintf(os.Stdout, "Dust UTXOs detected: 0\n")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TXID\tVOUT\tVALUE(sats)\tADDRESS")
	fmt.Fprintln(w, "----\t----\t-----------\t-------")

	for _, r := range results {
		addr := r.UTXO.Address
		if addr == "" {
			addr = "<none>"
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n",
			r.UTXO.TxID, r.UTXO.Vout, r.UTXO.ValueSats(), addr)
	}
	w.Flush()

	if zeroValueCount > 0 {
		fmt.Fprintf(os.Stdout, "\nIgnored %d zero-value UTXOs\n", zeroValueCount)
	}
	fmt.Fprintf(os.Stdout, "Total spendable UTXOs: %d\n", len(utxos))
	fmt.Fprintf(os.Stdout, "Dust UTXOs detected: %d\n", len(results))
}

func printScanResultsSummary(utxos []utxo.UTXO, results []dust.DustDetectionResult, zeroValueCount int) {
	riskCounts := make(map[dust.RiskScore]int)
	for _, r := range results {
		riskCounts[r.Reason.RiskScore]++
	}

	if zeroValueCount > 0 {
		fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n", zeroValueCount)
	}
	fmt.Fprintf(os.Stdout, "Total spendable UTXOs: %d\n", len(utxos))
	fmt.Fprintf(os.Stdout, "Dust UTXOs: %d\n", len(results))
	fmt.Fprintf(os.Stdout, "Risk distribution:\n")
	fmt.Fprintf(os.Stdout, "  HIGH:   %d\n", riskCounts[dust.RiskHigh])
	fmt.Fprintf(os.Stdout, "  MEDIUM: %d\n", riskCounts[dust.RiskMedium])
	fmt.Fprintf(os.Stdout, "  LOW:    %d\n", riskCounts[dust.RiskLow])
}

func printScanResultsVerbose(utxos []utxo.UTXO, results []dust.DustDetectionResult, zeroValueCount int) {
	printScanResultsTable(utxos, results, zeroValueCount)
	
	if len(results) > 0 {
		fmt.Fprintf(os.Stdout, "\nDetailed Analysis:\n")
		fmt.Fprintf(os.Stdout, "==================\n\n")
		
		for i, r := range results {
			fmt.Fprintf(os.Stdout, "%d. %s:%d\n", i+1, r.UTXO.TxID, r.UTXO.Vout)
			fmt.Fprintf(os.Stdout, "   Value: %d sats\n", r.UTXO.ValueSats())
			if r.UTXO.Address != "" {
				fmt.Fprintf(os.Stdout, "   Address: %s\n", r.UTXO.Address)
			}
			fmt.Fprintf(os.Stdout, "   Risk: %s\n", r.Reason.RiskScore)
			fmt.Fprintf(os.Stdout, "   Reason: %s\n", r.Reason.PrimaryReason)
			if len(r.Reason.Details) > 0 {
				fmt.Fprintf(os.Stdout, "   Details:\n")
				for _, detail := range r.Reason.Details {
					fmt.Fprintf(os.Stdout, "     - %s\n", detail)
				}
			}
			fmt.Fprintf(os.Stdout, "\n")
		}
	}
}

func printScanResultsJSON(utxos []utxo.UTXO, results []dust.DustDetectionResult, zeroValueCount int) {
	riskCounts := make(map[string]int)
	for _, r := range results {
		riskCounts[string(r.Reason.RiskScore)]++
	}

	output := ScanOutput{
		TotalUTXOs:       len(utxos) + zeroValueCount,
		SpendableUTXOs:   len(utxos),
		ZeroValueUTXOs:   zeroValueCount,
		DustUTXOsCount:  len(results),
		DustUTXOs:        results,
		RiskDistribution: riskCounts,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		return
	}

	fmt.Fprintln(os.Stdout, string(jsonData))
}
