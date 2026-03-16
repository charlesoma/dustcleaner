package cmd

import (
	"fmt"
	"os"

	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/psbtbuilder"
	"dustcleaner/rpc"
	"dustcleaner/utxo"

	"github.com/spf13/cobra"
)

var (
	buildDestAddress string
	buildFeeRate     int64
	buildMinConfs    int64
	buildMode        string
	buildDryRun      bool
	buildMaxFee      int64
	buildConfirm     bool
)

// buildPSBTCmd builds an unsigned cleanup transaction that spends detected dust UTXOs.
var buildPSBTCmd = &cobra.Command{
	Use:   "build-psbt",
	Short: "Build an unsigned tx that spends detected dust UTXOs",
	Long: `Connect to Bitcoin Core via RPC, fetch wallet UTXOs, detect dust
UTXOs using the dust heuristics, and build an unsigned transaction that spends them.
The resulting transaction is printed as raw hex along with basic fee statistics.`,
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
			fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n", zeroValueCount)
		}

		dustUtxos := dust.DetectDustUTXOs(spendableUTXOs)
		if len(dustUtxos) == 0 {
			fmt.Fprintln(os.Stdout, "No dust UTXOs detected; nothing to build.")
			return nil
		}

		var warnings []string

		if buildMinConfs > 0 {
			var filtered []dust.UTXO
			for _, u := range dustUtxos {
				if u.Confirmations >= buildMinConfs {
					filtered = append(filtered, u)
				} else {
					warnings = append(warnings,
						fmt.Sprintf("Skipping %s:%d with %d confirmations (< %d)",
							u.TxID, u.Vout, u.Confirmations, buildMinConfs))
				}
			}
			dustUtxos = filtered
			if len(dustUtxos) == 0 {
				fmt.Fprintln(os.Stdout, "No dust UTXOs meet the minimum confirmations requirement; nothing to build.")
				for _, w := range warnings {
					fmt.Fprintln(os.Stdout, "Warning:", w)
				}
				return nil
			}
		}

		for _, u := range dustUtxos {
			if u.Address == "" {
				warnings = append(warnings,
					fmt.Sprintf("UTXO %s:%d has no address field; ensure it belongs to your wallet before spending.", u.TxID, u.Vout))
			}
		}

		addrSet := make(map[string]struct{})
		for _, u := range dustUtxos {
			if u.Address == "" {
				continue
			}
			addrSet[u.Address] = struct{}{}
		}
		if len(addrSet) > 1 {
			warnings = append(warnings,
				fmt.Sprintf("Consolidating UTXOs from %d distinct addresses; this may reveal wallet linkage.", len(addrSet)))
		}

		var totalIn int64
		for _, u := range dustUtxos {
			totalIn += u.ValueSats()
		}

		estimatedFee := psbtbuilder.EstimateCleanupFee(len(dustUtxos), buildFeeRate)

		safetyConfig := SafetyConfig{
			DryRun:         buildDryRun,
			MaxFee:         buildMaxFee,
			RequireConfirm: buildConfirm,
		}

		if err := ValidateSafetyChecks(estimatedFee, totalIn, safetyConfig); err != nil {
			return err
		}

		mode := psbtbuilder.SpendingMode(buildMode)
		if mode == "" {
			mode = psbtbuilder.ModeFast // Default
		}
		if mode != psbtbuilder.ModeFast && mode != psbtbuilder.ModePrivacy && mode != psbtbuilder.ModeIsolated {
			return fmt.Errorf("invalid spending mode: %s (must be fast, privacy, or isolated)", buildMode)
		}

		if buildDryRun {
			PrintDryRunSummary(len(dustUtxos), totalIn, estimatedFee, buildDestAddress, mode)
			return nil
		}

		if buildConfirm {
			if !ConfirmCleanup(len(dustUtxos), totalIn, estimatedFee, buildDestAddress) {
				return fmt.Errorf("cleanup cancelled by user")
			}
		}

		txHexes, impacts, err := psbtbuilder.BuildDustCleanupPSBTWithMode(dustUtxos, buildDestAddress, buildFeeRate, mode)
		if err != nil {
			return fmt.Errorf("build cleanup transaction: %w", err)
		}

		for _, impact := range impacts {
			if impact.WarningLevel == "MEDIUM" || impact.WarningLevel == "HIGH" {
				warnings = append(warnings, fmt.Sprintf("Privacy: %s", impact.Message))
			}
		}

		for _, w := range warnings {
			fmt.Fprintln(os.Stdout, "Warning:", w)
		}

		fmt.Fprintf(os.Stdout, "Spending mode: %s\n", mode)
		fmt.Fprintf(os.Stdout, "Dust UTXOs selected: %d\n", len(dustUtxos))
		fmt.Fprintf(os.Stdout, "Total input: %d sats\n", totalIn)
		
		if len(txHexes) == 1 {
			fmt.Fprintf(os.Stdout, "Estimated fee: %d sats\n\n", estimatedFee)
			fmt.Fprintln(os.Stdout, "Generated unsigned transaction (hex):")
			fmt.Fprintln(os.Stdout, txHexes[0])
		} else {
			fmt.Fprintf(os.Stdout, "Generated %d transactions (one per address cluster)\n\n", len(txHexes))
			for i, txHex := range txHexes {
				clusterUTXOs := 0
				if i < len(impacts) {
					clusterUTXOs = impacts[i].TotalUTXOs
				}
				estimatedFee := psbtbuilder.EstimateCleanupFee(clusterUTXOs, buildFeeRate)
				fmt.Fprintf(os.Stdout, "Transaction %d (%d UTXOs, ~%d sats fee):\n", i+1, clusterUTXOs, estimatedFee)
				fmt.Fprintln(os.Stdout, txHex)
				if i < len(txHexes)-1 {
					fmt.Fprintln(os.Stdout, "")
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildPSBTCmd)

	buildPSBTCmd.Flags().StringVar(
		&buildDestAddress,
		"dest",
		"",
		"destination address for consolidated funds (empty to burn as fees)",
	)
	buildPSBTCmd.Flags().Int64Var(
		&buildFeeRate,
		"fee-rate",
		1,
		"fee rate in sat/vbyte for the cleanup transaction",
	)

	buildPSBTCmd.Flags().Int64Var(
		&buildMinConfs,
		"min-confs",
		1,
		"minimum confirmations required for UTXOs to be included in the cleanup transaction",
	)
	buildPSBTCmd.Flags().StringVar(
		&buildMode,
		"mode",
		"fast",
		"spending mode: fast (single tx, fastest), privacy (group by address), isolated (one tx per address, most private)",
	)
	buildPSBTCmd.Flags().BoolVar(
		&buildDryRun,
		"dry-run",
		false,
		"show what would be done without actually building the transaction",
	)
	buildPSBTCmd.Flags().Int64Var(
		&buildMaxFee,
		"max-fee",
		0,
		"maximum fee in satoshis (0 = no limit). Transaction will be aborted if estimated fee exceeds this",
	)
	buildPSBTCmd.Flags().BoolVar(
		&buildConfirm,
		"confirm",
		false,
		"prompt for confirmation before building the transaction",
	)
}

