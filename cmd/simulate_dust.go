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
	simDustFeeRate  int64
	simDustCount    int
	simDustValueSat int64
)

// simulateDustCmd runs a full regtest dust simulation: mine coins, create dust
// UTXOs, run detection, and build a cleanup transaction.
var simulateDustCmd = &cobra.Command{
	Use:   "simulate-dust",
	Short: "Simulate a dust attack on regtest and run detection/cleanup",
	Long: `Connect to a Bitcoin Core regtest node, mine coins to a fresh wallet
address, generate multiple tiny UTXOs to simulate a dust attack, run dust detection,
and build a cleanup transaction. Intended for testing only; requires regtest network.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		if cfg.Network != "regtest" && network != "regtest" {
			return fmt.Errorf("simulate-dust must be run against a regtest node (current network: %s)", cfg.Network)
		}

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

		fmt.Fprintln(os.Stdout, "Requesting new wallet address for simulation...")
		addr, err := client.GetNewAddress("dust-sim", "")
		if err != nil {
			return fmt.Errorf("getnewaddress: %w", err)
		}
		fmt.Fprintf(os.Stdout, "Simulation address: %s\n", addr)

		fmt.Fprintln(os.Stdout, "Mining 101 blocks to mature coinbase rewards...")
		if _, err := client.GenerateToAddress(101, addr); err != nil {
			return fmt.Errorf("generatetoaddress: %w", err)
		}

		utxos, err := client.ListUnspent()
		if err != nil {
			return fmt.Errorf("listunspent: %w", err)
		}

		var spendableBalance int64
		var zeroValueCount int
		var addrUTXOCount int
		for _, u := range utxos {
			if u.Address == addr {
				addrUTXOCount++
				if u.ValueSats() > 0 {
					spendableBalance += u.ValueSats()
				} else {
					zeroValueCount++
				}
			}
		}

		if zeroValueCount > 0 {
			fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n", zeroValueCount)
		}

		estimatedRequired := int64(simDustCount) * (simDustValueSat + 500)
		maxRetries := 3
		for retry := 0; retry < maxRetries && spendableBalance < estimatedRequired; retry++ {
			if retry > 0 {
				blocksToMine := 50 // Mine 50 blocks per retry (should provide ~2.5 BTC)
				fmt.Fprintf(os.Stdout, "Insufficient spendable balance (%d sats). Mining %d more blocks (retry %d/%d)...\n", 
					spendableBalance, blocksToMine, retry+1, maxRetries)
				if _, err := client.GenerateToAddress(blocksToMine, addr); err != nil {
					return fmt.Errorf("generatetoaddress (additional blocks): %w", err)
				}
			}
			
			// Re-check balance after mining
			utxos, err = client.ListUnspent()
			if err != nil {
				return fmt.Errorf("listunspent (recheck balance): %w", err)
			}
			
			spendableBalance = 0
			zeroValueCount = 0
			addrUTXOCount = 0
			for _, u := range utxos {
				if u.Address == addr {
					addrUTXOCount++
					if u.ValueSats() > 0 {
						spendableBalance += u.ValueSats()
					} else {
						zeroValueCount++
					}
				}
			}
			
			if zeroValueCount > 0 {
				fmt.Fprintf(os.Stdout, "Ignored %d zero-value UTXOs\n", zeroValueCount)
			}
		}
		
		if spendableBalance < estimatedRequired {
			return fmt.Errorf("insufficient balance: have %d sats, need %d sats", spendableBalance, estimatedRequired)
		}

		fmt.Fprintf(os.Stdout, "Creating %d dust-like UTXOs of %d sats each...\n", simDustCount, simDustValueSat)
		feeRateSatPerVByte := 1.0
		for i := 0; i < simDustCount; i++ {
			if _, err := client.SendToAddress(addr, simDustValueSat, feeRateSatPerVByte); err != nil {
				return fmt.Errorf("sendtoaddress (dust %d): %w", i, err)
			}
		}

		fmt.Fprintln(os.Stdout, "Mining 1 block to confirm dust transactions...")
		if _, err := client.GenerateToAddress(1, addr); err != nil {
			return fmt.Errorf("generatetoaddress: %w", err)
		}

		utxos, err = client.ListUnspent()
		if err != nil {
			return fmt.Errorf("listunspent: %w", err)
		}

		var spendableUTXOs []utxo.UTXO
		zeroValueCount = 0
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

		allDust := dust.DetectDustUTXOs(spendableUTXOs)
		if len(allDust) == 0 {
			fmt.Fprintln(os.Stdout, "No dust UTXOs detected after simulation.")
			return nil
		}

		fmt.Fprintf(os.Stdout, "Detected %d dust UTXOs after simulation.\n", len(allDust))

		var totalIn int64
		for _, u := range allDust {
			totalIn += u.ValueSats()
		}

		estimatedFee := psbtbuilder.EstimateCleanupFee(len(allDust), simDustFeeRate)
		valueOut := totalIn - estimatedFee
		if valueOut <= psbtbuilder.DustThresholdSats {
			return fmt.Errorf("cleanup output %d sats would be at or below dust threshold %d; aborting",
				valueOut, psbtbuilder.DustThresholdSats)
		}

		txHex, err := psbtbuilder.BuildDustCleanupPSBT(allDust, addr, simDustFeeRate)
		if err != nil {
			return fmt.Errorf("build cleanup transaction: %w", err)
		}

		fmt.Fprintf(os.Stdout, "Total dust input: %d sats\n", totalIn)
		fmt.Fprintf(os.Stdout, "Estimated cleanup fee: %d sats\n", estimatedFee)
		fmt.Fprintln(os.Stdout, "Simulated cleanup transaction (unsigned, hex):")
		fmt.Fprintln(os.Stdout, txHex)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(simulateDustCmd)

	simulateDustCmd.Flags().Int64Var(
		&simDustFeeRate,
		"fee-rate",
		1,
		"fee rate in sat/vbyte to assume for the simulated cleanup transaction",
	)
	simulateDustCmd.Flags().IntVar(
		&simDustCount,
		"count",
		10,
		"number of dust-like UTXOs to create in the simulation",
	)
	simulateDustCmd.Flags().Int64Var(
		&simDustValueSat,
		"value",
		320,
		"value (in satoshis) of each simulated dust UTXO",
	)
}

