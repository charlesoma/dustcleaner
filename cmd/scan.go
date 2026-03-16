package cmd

import (
	"fmt"

	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/rpc"
	"dustcleaner/utxo"

	"github.com/spf13/cobra"
)

var (
	scanOutputMode string
	scanMempool    bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan wallet UTXOs for potential dust attacks",
	Long: `Connect to Bitcoin Core via RPC, fetch wallet UTXOs, run dust
detection heuristics, and print suspicious UTXOs in a table along with
summary statistics.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration (YAML/JSON) and let CLI flags override it.
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// CLI flags (if set) take precedence over file config.
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

		results := dust.DetectDustUTXOsWithAnalysis(spendableUTXOs)

		if scanMempool {
			_, err := client.GetRawMempool()
			if err != nil {
				return fmt.Errorf("getrawmempool: %w", err)
			}
		}

		// Parse output mode
		mode := OutputMode(scanOutputMode)
		if mode == "" {
			mode = OutputModeTable // Default
		}
		if mode != OutputModeTable && mode != OutputModeSummary && mode != OutputModeVerbose && mode != OutputModeJSON {
			return fmt.Errorf("invalid output mode: %s (must be table, summary, verbose, or json)", scanOutputMode)
		}

		// Print results according to output mode
		PrintScanResults(spendableUTXOs, results, mode, zeroValueCount)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVar(
		&scanOutputMode,
		"output",
		"table",
		"output format: table (default), summary, verbose, or json",
	)
	scanCmd.Flags().BoolVar(
		&scanMempool,
		"mempool",
		false,
		"analyze unconfirmed transactions in mempool for dust patterns",
	)
}

