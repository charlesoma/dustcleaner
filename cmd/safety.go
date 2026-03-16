package cmd

import (
	"fmt"
	"os"
	"bufio"
	"dustcleaner/psbtbuilder"
)

type SafetyConfig struct {
	DryRun         bool
	MaxFee         int64
	RequireConfirm bool
}

// ConfirmCleanup prompts the user for confirmation before building a cleanup transaction.
func ConfirmCleanup(dustCount int, totalInput int64, estimatedFee int64, destAddress string) bool {
	fmt.Fprintf(os.Stderr, "\n⚠️  Cleanup Transaction Summary\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "Dust UTXOs to spend: %d\n", dustCount)
	fmt.Fprintf(os.Stderr, "Total input value: %d sats\n", totalInput)
	fmt.Fprintf(os.Stderr, "Estimated fee: %d sats\n", estimatedFee)
	
	if destAddress != "" {
		fmt.Fprintf(os.Stderr, "Output address: %s\n", destAddress)
		fmt.Fprintf(os.Stderr, "Output value: %d sats\n", totalInput-estimatedFee)
	} else {
		fmt.Fprintf(os.Stderr, "Output: Burn as fees (no output)\n")
	}
	
	fmt.Fprintf(os.Stderr, "\nThis transaction will spend %d dust UTXOs.\n", dustCount)
	fmt.Fprintf(os.Stderr, "Continue? (y/N): ")
	
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	
	return len(response) > 0 && (response[0] == 'y' || response[0] == 'Y')
}

func ValidateSafetyChecks(estimatedFee int64, totalInput int64, config SafetyConfig) error {
	if config.MaxFee > 0 && estimatedFee > config.MaxFee {
		return fmt.Errorf("estimated fee %d sats exceeds maximum allowed fee %d sats (use --max-fee to increase limit)", estimatedFee, config.MaxFee)
	}
	
	if totalInput <= estimatedFee {
		return fmt.Errorf("estimated fee %d sats exceeds or equals total input %d sats", estimatedFee, totalInput)
	}
	
	outputValue := totalInput - estimatedFee
	if outputValue <= psbtbuilder.DustThresholdSats {
		return fmt.Errorf("output value %d sats would be at or below dust threshold %d sats", outputValue, psbtbuilder.DustThresholdSats)
	}
	
	return nil
}

func PrintDryRunSummary(dustCount int, totalInput int64, estimatedFee int64, destAddress string, mode psbtbuilder.SpendingMode) {
	fmt.Fprintf(os.Stdout, "\n🔍 DRY RUN MODE - No transaction will be created\n")
	fmt.Fprintf(os.Stdout, "═══════════════════════════════════════════════\n")
	fmt.Fprintf(os.Stdout, "Would spend: %d dust UTXOs\n", dustCount)
	fmt.Fprintf(os.Stdout, "Total input: %d sats\n", totalInput)
	fmt.Fprintf(os.Stdout, "Estimated fee: %d sats\n", estimatedFee)
	fmt.Fprintf(os.Stdout, "Spending mode: %s\n", mode)
	
	if destAddress != "" {
		fmt.Fprintf(os.Stdout, "Output address: %s\n", destAddress)
		fmt.Fprintf(os.Stdout, "Output value: %d sats\n", totalInput-estimatedFee)
	} else {
		fmt.Fprintf(os.Stdout, "Output: Burn as fees\n")
	}
	
	fmt.Fprintf(os.Stdout, "\nRun without --dry-run to execute this cleanup.\n")
}
