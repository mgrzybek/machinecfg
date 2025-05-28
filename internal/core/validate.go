package core

import (
	"log/slog"
	"net/url"

	"machinecfg/internal/core/domain"
)

func ValidateMachineInfo(mc *domain.MachineInfo) (result bool) {
	result = true

	if !hasHostname(mc.Hostname) {
		result = false
	}
	if !bondingsHaveInterfaces(mc.Bondings) {
		result = false
	}
	if !interfacesInBondingsDontHaveStaticIPs(mc) {
		result = false
	}
	if !interfacesInBondingsHaveSameMTU(mc) {
		result = false
	}
	if !vlanIDsAreValid(mc) {
		result = false
	}
	if !bootstrapURLisURL(mc.BootstrapURL) {
		result = false
	}
	if !loggingEndpointsAreURL(mc.LoggingEndpoints) {
		result = false
	}

	if !result {
		slog.Warn("ValidateMachineInfo", "message", "Validation failed", "device", mc.Hostname)
	}

	return result
}

func hasHostname(value string) (result bool) {
	result = true

	if len(value) == 0 {
		result = false
		slog.Warn("hasHostname", "message", "Given hostname is too short", "value", value)
	}

	return result
}

func bondingsHaveInterfaces(bondings []domain.BondingConfiguration) (result bool) {
	result = true

	for _, bonding := range bondings {
		if len(bonding.Interfaces) < 2 {
			result = false
			slog.Warn("bondingsHaveInterfaces", "message", "Given bonding must have as least two interfaces", "name", bonding.Name)
		}
	}

	return result
}

func interfacesInBondingsDontHaveStaticIPs(mc *domain.MachineInfo) (result bool) {
	result = true

	for _, bonding := range mc.Bondings {
		if !interfacesInBondingDontHaveStaticIPs(&bonding) {
			result = false
		}
	}

	return result
}

func interfacesInBondingDontHaveStaticIPs(bc *domain.BondingConfiguration) (result bool) {
	result = true

	for _, iface := range bc.Interfaces {
		if len(iface.IPsWithCIDR) > 0 {
			result = false
			slog.Warn("interfacesInBondingDontHaveStaticIPs", "message", "interface of bonding has an IP address", "bonding", bc.Name)
		}
	}

	return result
}

func interfacesInBondingsHaveSameMTU(mc *domain.MachineInfo) (result bool) {
	result = true
	for _, bonding := range mc.Bondings {
		if !interfacesInBondingHaveSameMTU(&bonding) {
			result = false
			slog.Warn("interfacesInBondingsHaveSameMTU", "message", "bonding has broken MTU", "bonding", bonding.Name)
		}
	}
	return result
}

func interfacesInBondingHaveSameMTU(bc *domain.BondingConfiguration) (result bool) {
	result = true

	if len(bc.Interfaces) > 0 {
		if bc.Interfaces[0].MTU != bc.Interfaces[1].MTU {
			result = false
			slog.Warn("interfacesInBondingHaveSameMTU", "message", "MTU differs", bc.Interfaces[0].Name, bc.Interfaces[0].MTU, bc.Interfaces[1].Name, bc.Interfaces[1].MTU)
		}
	}

	return result
}

func vlanIDsAreValid(mc *domain.MachineInfo) (result bool) {
	result = true

	for _, bonding := range mc.Bondings {
		for _, vlan := range bonding.VLANs {
			if !vlanIDIsValid(&vlan) {
				result = false
				slog.Warn("vlanIDsAreValid", "message", "VLAN attached to bonding is invalid", "bonding", bonding.Name, "vlan", vlan.ID)
			}
		}
	}

	for _, iface := range mc.Interfaces {
		for _, vlan := range iface.VLANs {
			if !vlanIDIsValid(&vlan) {
				result = false
				slog.Warn("vlanIDsAreValid", "message", "VLAN attached to physical interfaces is invalid", "interface", iface.Name, "vlan", vlan.ID)
			}
		}
	}

	return result
}

func vlanIDIsValid(vif *domain.VLANInterface) (result bool) {
	result = true

	if vif.ID < 1 || vif.ID > 4095 {
		result = false
		slog.Warn("vlanIDsAreValid", "message", "the given VLAN ID is out of range", "value", vif.ID)
	}

	return result
}

func bootstrapURLisURL(value string) (result bool) {
	result = true

	if value != "" {
		u, err := url.Parse(value)
		if err != nil {
			result = false
			slog.Warn("bootstrapURLisURL", "message", "This is not a valid URL", "value", value)
		}

		if u.Scheme != "https" && u.Scheme != "http" {
			result = false
			slog.Warn("bootstrapURLisURL", "message", "This is not a valid scheme", "value", u.Scheme)
		}
	}

	return result
}

func loggingEndpointsAreURL(endpoints []string) (result bool) {
	result = true

	for _, value := range endpoints {
		if value != "" {
			u, err := url.Parse(value)
			if err != nil {
				result = false
				slog.Warn("bootstrapURLisURL", "message", "This is not a valid URL", "value", value)
			}

			if u.Scheme != "tcp" && u.Scheme != "udp" {
				result = false
				slog.Warn("bootstrapURLisURL", "message", "This is not a valid scheme", "value", u.Scheme)
			}
		}
	}

	return result
}
