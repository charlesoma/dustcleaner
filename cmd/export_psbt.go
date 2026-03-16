package cmd

import (
	"fmt"
	"os"

	"dustcleaner/config"
	"dustcleaner/dust"
	"dustcleaner/psbtbuilder"
	"dustcleaner/rpc"

	"github.com/spf13/cobra"
)

var (
	exportDestAddress string
	exportFeeRate     int64
	exportMinConfs    int64
	exportFilePath    string
	exportMode        string
	exportDryRun      bool
	exportMaxFee      int64
	exportConfirm     bool
)

// exportPSBTCmd builds a cleanup transaction from dust UTXOs, converts it to a PSBT
// using Bitcoin Core, and writes the PSBT to disk.
var exportPSBTCmd = &cobra.Command{
	Use:   "export-psbt",
	Short: "Build and export a PSBT spending detected dust UTXOs",
	Long: `Connect to Bitcoin Core via RPC, fetch wallet UTXOs, detect dust
UTXOs using the dust heuristics, build a cleanup transaction, convert it to a PSBT
via Bitcoin Core's converttopsbt RPC, and write the PSBT to disk. The resulting PSBT
is compatible with walletprocesspsbt and modern hardware wallets.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if exportFilePath == "" {
			return fmt.Errorf("--file is required to specify the PSBT output path")
		}

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

		dustUtxos := dust.DetectDustUTXOs(utxos)
		if len(dustUtxos) == 0 {
			fmt.Fprintln(os.Stdout, "No dust UTXOs detected; nothing to export.")
			return nil
		}

		var warnings []string

		if exportMinConfs > 0 {
			var filtered []dust.UTXO
			for _, u := range dustUtxos {
				if u.Confirmations >= exportMinConfs {
					filtered = append(filtered, u)
				} else {
					warnings = append(warnings,
						fmt.Sprintf("Skipping %s:%d with %d confirmations (< %d)",
							u.TxID, u.Vout, u.Confirmations, exportMinConfs))
				}
			}
			dustUtxos = filtered
			if len(dustUtxos) == 0 {
				fmt.Fprintln(os.Stdout, "No dust UTXOs meet the minimum confirmations requirement; nothing to export.")
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

		estimatedFee := psbtbuilder.EstimateCleanupFee(len(dustUtxos), exportFeeRate)

		mode := psbtbuilder.SpendingMode(exportMode)
		if mode == "" {
			mode = psbtbuilder.ModeFast // Default
		}
		if mode != psbtbuilder.ModeFast && mode != psbtbuilder.ModePrivacy && mode != psbtbuilder.ModeIsolated {
			return fmt.Errorf("invalid spending mode: %s (must be fast, privacy, or isolated)", exportMode)
		}

		safetyConfig := SafetyConfig{
			DryRun:         exportDryRun,
			MaxFee:         exportMaxFee,
			RequireConfirm: exportConfirm,
		}

		if err := ValidateSafetyChecks(estimatedFee, totalIn, safetyConfig); err != nil {
			return err
		}

		if exportDryRun {
			PrintDryRunSummary(len(dustUtxos), totalIn, estimatedFee, exportDestAddress, mode)
			return nil
		}

		if exportConfirm {
			if !ConfirmCleanup(len(dustUtxos), totalIn, estimatedFee, exportDestAddress) {
				return fmt.Errorf("cleanup cancelled by user")
			}
		}

		txHexes, impacts, err := psbtbuilder.BuildDustCleanupPSBTWithMode(dustUtxos, exportDestAddress, exportFeeRate, mode)
		if err != nil {
			return fmt.Errorf("build cleanup transaction: %w", err)
		}

		for _, impact := range impacts {
			if impact.WarningLevel == "MEDIUM" || impact.WarningLevel == "HIGH" {
				warnings = append(warnings, fmt.Sprintf("Privacy: %s", impact.Message))
			}
		}

		var psbtStrings []string
		for i, txHex := range txHexes {
			psbtStr, err := client.ConvertToPSBT(txHex)
			if err != nil {
				return fmt.Errorf("converttopsbt (tx %d): %w", i+1, err)
			}
			psbtStrings = append(psbtStrings, psbtStr)
		}

		if len(psbtStrings) == 1 {
			if err := os.WriteFile(exportFilePath, []byte(psbtStrings[0]+"\n"), 0o600); err != nil {
				return fmt.Errorf("write PSBT to %s: %w", exportFilePath, err)
			}
		} else {
			for i, psbtStr := range psbtStrings {
				filePath := fmt.Sprintf("%s.%d", exportFilePath, i+1)
				if err := os.WriteFile(filePath, []byte(psbtStr+"\n"), 0o600); err != nil {
					return fmt.Errorf("write PSBT to %s: %w", filePath, err)
				}
			}
		}

		for _, w := range warnings {
			fmt.Fprintln(os.Stdout, "Warning:", w)
		}

		fmt.Fprintf(os.Stdout, "Spending mode: %s\n", mode)
		fmt.Fprintf(os.Stdout, "Dust UTXOs selected: %d\n", len(dustUtxos))
		fmt.Fprintf(os.Stdout, "Total input: %d sats\n", totalIn)
		if len(psbtStrings) == 1 {
			fmt.Fprintf(os.Stdout, "Estimated fee: %d sats\n", estimatedFee)
			fmt.Fprintf(os.Stdout, "PSBT written to: %s\n", exportFilePath)
		} else {
			fmt.Fprintf(os.Stdout, "Generated %d PSBTs (one per address cluster)\n", len(psbtStrings))
			for i := range psbtStrings {
				filePath := fmt.Sprintf("%s.%d", exportFilePath, i+1)
				fmt.Fprintf(os.Stdout, "  PSBT %d written to: %s\n", i+1, filePath)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportPSBTCmd)

	exportPSBTCmd.Flags().StringVar(
		&exportDestAddress,
		"dest",
		"",
		"destination address for consolidated funds (empty to burn as fees)",
	)
	exportPSBTCmd.Flags().Int64Var(
		&exportFeeRate,
		"fee-rate",
		1,
		"fee rate in sat/vbyte for the cleanup transaction",
	)
	exportPSBTCmd.Flags().Int64Var(
		&exportMinConfs,
		"min-confs",
		1,
		"minimum confirmations required for UTXOs to be included in the cleanup transaction",
	)
	exportPSBTCmd.Flags().StringVar(
		&exportFilePath,
		"file",
		"",
		"path where the generated PSBT will be written",
	)
	exportPSBTCmd.Flags().StringVar(
		&exportMode,
		"mode",
		"fast",
		"spending mode: fast (single tx, fastest), privacy (group by address), isolated (one tx per address, most private)",
	)
	exportPSBTCmd.Flags().BoolVar(
		&exportDryRun,
		"dry-run",
		false,
		"show what would be done without actually building the transaction",
	)
	exportPSBTCmd.Flags().Int64Var(
		&exportMaxFee,
		"max-fee",
		0,
		"maximum fee in satoshis (0 = no limit). Transaction will be aborted if estimated fee exceeds this",
	)
	exportPSBTCmd.Flags().BoolVar(
		&exportConfirm,
		"confirm",
		false,
		"prompt for confirmation before building the transaction",
	)
}

