package psbtbuilder

import (
	"dustcleaner/utxo"
	"fmt"
)

type SpendingMode string

const (
	ModeFast     SpendingMode = "fast"
	ModePrivacy  SpendingMode = "privacy"
	ModeIsolated SpendingMode = "isolated"
)

type AddressCluster struct {
	Address string
	UTXOs   []utxo.UTXO
}

// GroupUTXOsByAddress clusters UTXOs by their address.
func GroupUTXOsByAddress(utxos []utxo.UTXO) []AddressCluster {
	clusters := make(map[string][]utxo.UTXO)
	
	for _, u := range utxos {
		addr := u.Address
		if addr == "" {
			addr = "<unknown>"
		}
		clusters[addr] = append(clusters[addr], u)
	}
	
	var result []AddressCluster
	for addr, utxosForAddr := range clusters {
		result = append(result, AddressCluster{
			Address: addr,
			UTXOs:   utxosForAddr,
		})
	}
	
	return result
}

type PrivacyImpact struct {
	DistinctAddresses int
	TotalUTXOs        int
	WarningLevel      string
	Message           string
}

// AnalyzePrivacyImpact analyzes the privacy implications of spending the given UTXOs together.
func AnalyzePrivacyImpact(utxos []utxo.UTXO) PrivacyImpact {
	addrSet := make(map[string]struct{})
	for _, u := range utxos {
		if u.Address != "" {
			addrSet[u.Address] = struct{}{}
		}
	}
	
	distinctAddrs := len(addrSet)
	totalUTXOs := len(utxos)
	
	var warningLevel string
	var message string
	
	if distinctAddrs == 0 {
		warningLevel = "UNKNOWN"
		message = "Some UTXOs have no address field; cannot assess privacy impact"
	} else if distinctAddrs == 1 {
		warningLevel = "LOW"
		message = fmt.Sprintf("All %d UTXOs belong to the same address; no linkage risk", totalUTXOs)
	} else if distinctAddrs <= 3 {
		warningLevel = "MEDIUM"
		message = fmt.Sprintf("Consolidating %d UTXOs from %d addresses may reveal wallet linkage via common input ownership heuristic", totalUTXOs, distinctAddrs)
	} else {
		warningLevel = "HIGH"
		message = fmt.Sprintf("Consolidating %d UTXOs from %d distinct addresses will reveal wallet linkage (common input ownership heuristic)", totalUTXOs, distinctAddrs)
	}
	
	return PrivacyImpact{
		DistinctAddresses: distinctAddrs,
		TotalUTXOs:        totalUTXOs,
		WarningLevel:      warningLevel,
		Message:           message,
	}
}

// BuildDustCleanupPSBTWithMode builds cleanup transactions according to the specified spending mode.
func BuildDustCleanupPSBTWithMode(
	utxosIn []utxo.UTXO,
	destinationAddress string,
	feeRate int64,
	mode SpendingMode,
) ([]string, []PrivacyImpact, error) {
	if len(utxosIn) == 0 {
		return nil, nil, fmt.Errorf("no UTXOs provided")
	}
	if feeRate <= 0 {
		return nil, nil, fmt.Errorf("feeRate must be positive")
	}
	
	var txHexes []string
	var impacts []PrivacyImpact
	
	switch mode {
	case ModeFast:
		impact := AnalyzePrivacyImpact(utxosIn)
		impacts = append(impacts, impact)
		
		txHex, err := BuildDustCleanupPSBT(utxosIn, destinationAddress, feeRate)
		if err != nil {
			return nil, nil, err
		}
		txHexes = append(txHexes, txHex)
		
	case ModePrivacy:
		clusters := GroupUTXOsByAddress(utxosIn)
		
		for _, cluster := range clusters {
			if len(cluster.UTXOs) == 0 {
				continue
			}
			
			impact := AnalyzePrivacyImpact(cluster.UTXOs)
			impacts = append(impacts, impact)
			
			txHex, err := BuildDustCleanupPSBT(cluster.UTXOs, destinationAddress, feeRate)
			if err != nil {
				return nil, nil, fmt.Errorf("build transaction for cluster %s: %w", cluster.Address, err)
			}
			txHexes = append(txHexes, txHex)
		}
		
	case ModeIsolated:
		clusters := GroupUTXOsByAddress(utxosIn)
		
		for _, cluster := range clusters {
			if len(cluster.UTXOs) == 0 {
				continue
			}
			
			impact := AnalyzePrivacyImpact(cluster.UTXOs)
			impacts = append(impacts, impact)
			
			txHex, err := BuildDustCleanupPSBT(cluster.UTXOs, destinationAddress, feeRate)
			if err != nil {
				return nil, nil, fmt.Errorf("build transaction for address %s: %w", cluster.Address, err)
			}
			txHexes = append(txHexes, txHex)
		}
		
	default:
		return nil, nil, fmt.Errorf("unknown spending mode: %s", mode)
	}
	
	return txHexes, impacts, nil
}
