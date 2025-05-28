package core

import (
	"machinecfg/internal/core/domain"
	"testing"

	"github.com/stretchr/testify/assert"
)

var result bool
var expected bool

func init() {

}

func createCorrectMachineInfo() (result domain.MachineInfo) {
	result.Hostname = "myhost"
	result.DNS = []string{"1.1.1.1", "2.2.2.2"}
	result.BootstrapURL = "https://my-bootstrap.local:8443"
	result.LoggingEndpoints = []string{
		"udp://1.2.3.4:443",
		"tcp://4.3.2.1:443",
	}

	correctInterface1 := domain.PhysicalInterface{
		MACAddress: "00:11:22:33:44",
		Name:       "ens0",
		MTU:        1500,
	}
	correctInterface2 := domain.PhysicalInterface{
		MACAddress: "01:12:23:34:45",
		Name:       "ens1",
		MTU:        1500,
	}

	correctVLANInterface := domain.VLANInterface{
		ID:          100,
		IPsWithCIDR: []string{"10.5.189.10/24"},
	}
	result.Interfaces = append(result.Interfaces, correctInterface1, correctInterface2)

	correctBonding := domain.BondingConfiguration{
		Name:        "bond0",
		IPsWithCIDR: []string{"172.16.0.10/24"},
		Gateways:    []string{"172.16.0.1"},
		VLANs:       []domain.VLANInterface{correctVLANInterface},
	}
	correctBonding.Interfaces = append(correctBonding.Interfaces, &result.Interfaces[0], &result.Interfaces[1])
	result.Bondings = append(result.Bondings, correctBonding)

	return result
}

/*
 * Clean MachineInfo
 */
func TestValidateMachineInfo(t *testing.T) {
	correctMachineInfo := createCorrectMachineInfo()

	result = ValidateMachineInfo(&correctMachineInfo)
	expected = true
	assert.Equal(t, expected, result, "The validation of a correct MachineInfo should have passed")
}

/*
 * MachineInfo: broken hostname
 */
func TestValidateMachineInfoHostname(t *testing.T) {
	brokenHostname := createCorrectMachineInfo()

	brokenHostname.Hostname = ""
	result = ValidateMachineInfo(&brokenHostname)
	expected = false
	assert.Equal(t, expected, result, "The hostname is broken, the validation should have failed")
}

func TestValidateMachineInfoBonding(t *testing.T) {
	/*
	 * MachineInfo: bonding without interface
	 */
	t.Log("Testing bonding without interface")
	brokenBonding := createCorrectMachineInfo()
	brokenBonding.Bondings[0].Interfaces = []*domain.PhysicalInterface{}
	result = ValidateMachineInfo(&brokenBonding)
	expected = false
	assert.Equal(t, expected, result, "The bonding is empty, the validation should have failed")

	/*
	 * MachineInfo: broken IPs in bonding
	 */
	t.Log("Testing broken IPs in bonding")
	brokenIP := createCorrectMachineInfo()
	brokenIP.Interfaces[0].IPsWithCIDR = []string{"192.168.1.24/24"}
	result = ValidateMachineInfo(&brokenIP)
	expected = false
	assert.Equal(t, expected, result, "The IP is broken, the validation should have failed")

	/*
	 * MachineInfo: broken MTUs in bonding
	 */
	t.Log("Testing broken MTUs")
	brokenMTU := createCorrectMachineInfo()
	brokenMTU.Interfaces[0].MTU = 1500
	brokenMTU.Interfaces[1].MTU = 9000
	result = ValidateMachineInfo(&brokenMTU)
	expected = false
	assert.Equal(t, expected, result, "The MTUs are broken, the validation should have failed")

}

/*
 * MachineInfo: broken VLAN
 */
func TestValidateMachineInfoVLAN(t *testing.T) {
	brokenVLAN1 := createCorrectMachineInfo()
	brokenVLAN1.Bondings[0].VLANs[0].ID = 0
	result = ValidateMachineInfo(&brokenVLAN1)
	expected = false
	assert.Equal(t, expected, result, "The VLAN ID is broken, the validation should have failed")

	brokenVLAN2 := createCorrectMachineInfo()
	brokenVLAN2.Bondings[0].VLANs[0].ID = 89078
	result = ValidateMachineInfo(&brokenVLAN2)
	expected = false
	assert.Equal(t, expected, result, "The VLAN ID is broken, the validation should have failed")
}

/*
 * MachineInfo: broken bootstrap URL
 */
func TestValidateMachineInfoBootstrapURL(t *testing.T) {
	brokenBootstrapURL := createCorrectMachineInfo()
	brokenBootstrapURL.BootstrapURL = "ht://idontknow"
	result = ValidateMachineInfo(&brokenBootstrapURL)
	expected = false
	assert.Equal(t, expected, result, "The bootstrap URL is broken, the validation should have failed")
}

func TestValidateMachineInfoLoggingEndpoints(t *testing.T) {
	brokenLoggingEndpoints := createCorrectMachineInfo()
	brokenLoggingEndpoints.LoggingEndpoints[0] = "toto://2.8.3.3"
	brokenLoggingEndpoints.LoggingEndpoints[0] = "too:/2.8.3.3"
	result = ValidateMachineInfo(&brokenLoggingEndpoints)
	expected = false
	assert.Equal(t, expected, result, "The Logging Endpoint is broken, the validation should have failed")
}
