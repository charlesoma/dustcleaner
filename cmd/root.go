package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	rpcURL    string
	rpcUser   string
	rpcPass   string
	wallet    string
	network   string
)

var rootCmd = &cobra.Command{
	Use:   "dustcleaner",
	Short: "Analyze and clean Bitcoin wallet dust UTXOs",
	Long: `DustCleaner is a CLI tool for scanning Bitcoin Core wallet UTXOs,
detecting dust outputs, and constructing PSBTs to safely consolidate them.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("dustcleaner CLI. Use --help to see available subcommands.")
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dustcleaner.yaml)")
	rootCmd.PersistentFlags().StringVar(&rpcURL, "rpc-url", "http://127.0.0.1:8332", "Bitcoin Core RPC URL")
	rootCmd.PersistentFlags().StringVar(&rpcUser, "rpc-user", "", "Bitcoin Core RPC username")
	rootCmd.PersistentFlags().StringVar(&rpcPass, "rpc-pass", "", "Bitcoin Core RPC password")
	rootCmd.PersistentFlags().StringVar(&wallet, "wallet", "", "Bitcoin Core wallet name (optional, defaults to node's default wallet)")
	rootCmd.PersistentFlags().StringVar(&network, "network", "mainnet", "Bitcoin network (mainnet, testnet, regtest, signet)")
}

