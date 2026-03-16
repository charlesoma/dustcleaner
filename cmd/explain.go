package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/rpc"
	"dustcleaner/utxo"

	"github.com/spf13/cobra"
)

var (
	explainTxID      string
	explainVout      uint32
	explainOutputMode string
)

// explainCmd explains why specific UTXOs are flagged as dust.
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain why UTXOs are flagged as dust",
	Long: `Fetch wallet UTXOs, run advanced dust detection with risk scoring,
and explain why each UTXO was flagged. Can optionally filter by specific txid:vout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// CLI flags override config file values.
		if rpcURL != "" {
			cfg.RPCURL = rpcURL
		}
		if rpcUser != "" {
			cfg.RPCUser = rpcUser
		}
		if rpcPass != "" {
			cfg.RPCPass = rpcPass
		}
		if wallet != "" {
			cfg.Wallet = wallet
		}
		if network != "" {
			cfg.Network = network
		}

		client := rpc.NewFromConfig(cfg)

		utxos, err := client.ListUnspent()
		if err != nil {
			return fmt.Errorf("listunspent: %w", err)
		}

		var spendableUTXOs []utxo.UTXO
		var zeroValueCount int
		for _, u := range utxos {
			if u.ValueSats() > 0 {
				spendableUTXOs = append(spendableUTXOs, u)
			} else {
				zeroValueCount++
			}
		}

		if zeroValueCount > 0 {
			fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n\n", zeroValueCount)
		}

		results := dust.DetectDustUTXOsWithAnalysis(spendableUTXOs)

		if len(results) == 0 {
			fmt.Fprintln(os.Stdout, "No dust UTXOs detected.")
			return nil
		}

		if explainTxID != "" {
			var filtered []dust.DustDetectionResult
			for _, r := range results {
				if r.UTXO.TxID == explainTxID && (explainVout == 0 || r.UTXO.Vout == explainVout) {
					filtered = append(filtered, r)
				}
			}
			results = filtered
			if len(results) == 0 {
				fmt.Fprintf(os.Stdout, "No dust UTXOs found matching txid=%s vout=%d\n", explainTxID, explainVout)
				return nil
			}
		}

		// Count by risk level
		riskCounts := make(map[dust.RiskScore]int)
		for _, r := range results {
			riskCounts[r.Reason.RiskScore]++
		}

		fmt.Fprintf(os.Stdout, "Dust Detection Analysis\n")
		fmt.Fprintf(os.Stdout, "=======================\n\n")
		fmt.Fprintf(os.Stdout, "Total dust UTXOs detected: %d\n", len(results))
		fmt.Fprintf(os.Stdout, "Risk distribution:\n")
		fmt.Fprintf(os.Stdout, "  HIGH:   %d\n", riskCounts[dust.RiskHigh])
		fmt.Fprintf(os.Stdout, "  MEDIUM: %d\n", riskCounts[dust.RiskMedium])
		fmt.Fprintf(os.Stdout, "  LOW:    %d\n", riskCounts[dust.RiskLow])
		fmt.Fprintf(os.Stdout, "\n")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TXID\tVOUT\tVALUE(sats)\tADDRESS\tRISK\tREASON")
		fmt.Fprintln(w, "----\t----\t-----------\t-------\t----\t------")

		for _, r := range results {
			addr := r.UTXO.Address
			if addr == "" {
				addr = "<none>"
			}
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n",
				r.UTXO.TxID[:16]+"...",
				r.UTXO.Vout,
				r.UTXO.ValueSats(),
				addr[:16]+"...",
				r.Reason.RiskScore,
				r.Reason.PrimaryReason,
			)
		}
		w.Flush()

		fmt.Fprintf(os.Stdout, "\nDetailed Explanations\n")
		fmt.Fprintf(os.Stdout, "=====================\n\n")

		for i, r := range results {
			threshold, scriptType, _ := dust.GetDustThresholdForUTXO(r.UTXO)
			
			fmt.Fprintf(os.Stdout, "%d. UTXO: %s:%d\n", i+1, r.UTXO.TxID, r.UTXO.Vout)
			fmt.Fprintf(os.Stdout, "   Value: %d sats\n", r.UTXO.ValueSats())
			fmt.Fprintf(os.Stdout, "   Script Type: %s (threshold: %d sats)\n", scriptType, threshold)
			if r.UTXO.Address != "" {
				fmt.Fprintf(os.Stdout, "   Address: %s\n", r.UTXO.Address)
			}
			fmt.Fprintf(os.Stdout, "   Risk Score: %s\n", r.Reason.RiskScore)
			fmt.Fprintf(os.Stdout, "   Reason: %s\n", r.Reason.PrimaryReason)
			if len(r.Reason.Details) > 0 {
				fmt.Fprintf(os.Stdout, "   Details:\n")
				for _, detail := range r.Reason.Details {
					fmt.Fprintf(os.Stdout, "     - %s\n", detail)
				}
			}
			fmt.Fprintf(os.Stdout, "\n")
		}

		return nil
	},
}

func printExplainJSON(results []dust.DustDetectionResult, riskCounts map[dust.RiskScore]int) error {
	type ExplainOutput struct {
		TotalDustUTXOs int                    `json:"total_dust_utxos"`
		RiskDistribution map[string]int       `json:"risk_distribution"`
		DustUTXOs       []dust.DustDetectionResult `json:"dust_utxos"`
	}

	riskDist := make(map[string]int)
	for level, count := range riskCounts {
		riskDist[string(level)] = count
	}

	output := ExplainOutput{
		TotalDustUTXOs: len(results),
		RiskDistribution: riskDist,
		DustUTXOs:       results,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func printExplainSummary(results []dust.DustDetectionResult, riskCounts map[dust.RiskScore]int) {
	fmt.Fprintf(os.Stdout, "Dust Detection Summary\n")
	fmt.Fprintf(os.Stdout, "======================\n\n")
	fmt.Fprintf(os.Stdout, "Total dust UTXOs: %d\n", len(results))
	fmt.Fprintf(os.Stdout, "Risk distribution:\n")
	fmt.Fprintf(os.Stdout, "  HIGH:   %d\n", riskCounts[dust.RiskHigh])
	fmt.Fprintf(os.Stdout, "  MEDIUM: %d\n", riskCounts[dust.RiskMedium])
	fmt.Fprintf(os.Stdout, "  LOW:    %d\n", riskCounts[dust.RiskLow])
}

func printExplainTable(results []dust.DustDetectionResult) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TXID\tVOUT\tVALUE(sats)\tADDRESS\tRISK\tREASON")
	fmt.Fprintln(w, "----\t----\t-----------\t-------\t----\t------")

	for _, r := range results {
		addr := r.UTXO.Address
		if addr == "" {
			addr = "<none>"
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n",
			r.UTXO.TxID[:16]+"...",
			r.UTXO.Vout,
			r.UTXO.ValueSats(),
			addr[:16]+"...",
			r.Reason.RiskScore,
			r.Reason.PrimaryReason,
		)
	}
	w.Flush()
}

func printExplainVerbose(results []dust.DustDetectionResult, riskCounts map[dust.RiskScore]int) {
	fmt.Fprintf(os.Stdout, "Dust Detection Analysis\n")
	fmt.Fprintf(os.Stdout, "=======================\n\n")
	fmt.Fprintf(os.Stdout, "Total dust UTXOs detected: %d\n", len(results))
	fmt.Fprintf(os.Stdout, "Risk distribution:\n")
	fmt.Fprintf(os.Stdout, "  HIGH:   %d\n", riskCounts[dust.RiskHigh])
	fmt.Fprintf(os.Stdout, "  MEDIUM: %d\n", riskCounts[dust.RiskMedium])
	fmt.Fprintf(os.Stdout, "  LOW:    %d\n", riskCounts[dust.RiskLow])
	fmt.Fprintf(os.Stdout, "\n")

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TXID\tVOUT\tVALUE(sats)\tADDRESS\tRISK\tREASON")
	fmt.Fprintln(w, "----\t----\t-----------\t-------\t----\t------")

	for _, r := range results {
		addr := r.UTXO.Address
		if addr == "" {
			addr = "<none>"
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n",
			r.UTXO.TxID[:16]+"...",
			r.UTXO.Vout,
			r.UTXO.ValueSats(),
			addr[:16]+"...",
			r.Reason.RiskScore,
			r.Reason.PrimaryReason,
		)
	}
	w.Flush()

	fmt.Fprintf(os.Stdout, "\nDetailed Explanations\n")
	fmt.Fprintf(os.Stdout, "=====================\n\n")

	for i, r := range results {
		// Get script type and threshold for display
		threshold, scriptType, _ := dust.GetDustThresholdForUTXO(r.UTXO)
		
		fmt.Fprintf(os.Stdout, "%d. UTXO: %s:%d\n", i+1, r.UTXO.TxID, r.UTXO.Vout)
		fmt.Fprintf(os.Stdout, "   Value: %d sats\n", r.UTXO.ValueSats())
		fmt.Fprintf(os.Stdout, "   Script Type: %s (threshold: %d sats)\n", scriptType, threshold)
		if r.UTXO.Address != "" {
			fmt.Fprintf(os.Stdout, "   Address: %s\n", r.UTXO.Address)
		}
		fmt.Fprintf(os.Stdout, "   Risk Score: %s\n", r.Reason.RiskScore)
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

func init() {
	rootCmd.AddCommand(explainCmd)

	explainCmd.Flags().StringVar(
		&explainTxID,
		"txid",
		"",
		"filter results to a specific transaction ID",
	)
	explainCmd.Flags().Uint32Var(
		&explainVout,
		"vout",
		0,
		"filter results to a specific output index (requires --txid)",
	)
	explainCmd.Flags().StringVar(
		&explainOutputMode,
		"output",
		"verbose",
		"output format: table, summary, verbose (default), or json",
	)
}
