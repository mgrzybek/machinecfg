package input

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"machinecfg/internal/core/domain"
)

type Netbox struct {
	clientRaw *http.Client
	args      *domain.ConfigurationArgs
}

func NewNetbox(args *domain.ConfigurationArgs) (*Netbox, error) {
	return &Netbox{
		clientRaw: &http.Client{},
		args:      args,
	}, nil

	// TODO: check connectivity
	// TODO: check that role, site and tenant exist

}

func (nb *Netbox) GetMachines() (result []domain.MachineInfo) {
	var apiResponse APIDevicesResponse
	var apiDevices []Device
	var uri string

	var queryStrings []string

	if nb.args.Region != "" {
		queryStrings = append(queryStrings, fmt.Sprintf("region=%s", nb.args.Region))
	}
	if nb.args.Site != "" {
		queryStrings = append(queryStrings, fmt.Sprintf("site=%s", nb.args.Site))
	}
	if nb.args.Location != "" {
		queryStrings = append(queryStrings, fmt.Sprintf("site=%s", nb.args.Location))
	}
	if nb.args.Rack!= "" {
		queryStrings = append(queryStrings, fmt.Sprintf("site=%s", nb.args.Rack))
	}
	if nb.args.Tenant != "" {
		queryStrings = append(queryStrings, fmt.Sprintf("tenant=%s", nb.args.Tenant))
	}
	if nb.args.Role != "" {
		queryStrings = append(queryStrings, fmt.Sprintf("role=%s", nb.args.Role))
	}

	uri = fmt.Sprintf("api/dcim/devices/?%s", strings.Join(queryStrings[:], "&"))

	resultRaw, err := nb.getHTTPRequest(uri)

	if err == nil {
		err = json.Unmarshal(resultRaw, &apiResponse)

		if err == nil {
			apiDevices = apiResponse.Results
		}
	}

	result = nb.transformAPIDevicesToMachineInfo(apiDevices)

	return result
}

func (nb *Netbox) getHTTPRequest(uri string) (result []byte, err error) {
	url := fmt.Sprintf("%s/%s", nb.args.Endpoint, uri)

	req, err := http.NewRequest("GET", url, nil)
	if err == nil {
		req.Header.Add("Authorization", fmt.Sprintf("Token %s", nb.args.Token))

		resp, err := nb.clientRaw.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			result = body

			if resp.StatusCode > 200 {
				err = fmt.Errorf("bad HTTP status code: %v", resp.StatusCode)
			}
		}

		if resp == nil {
			slog.Error("getHTTPRequest", "message", "cannot connect against Netbox")
		}

		slog.Debug("getHTTPRequest", "url", url, "result", result, "HTTP code", resp.StatusCode)

		if err != nil {
			slog.Error("getHTTPRequest", "message", err.Error())
		}

	}
	return result, err
}

func (nb *Netbox) transformAPIDevicesToMachineInfo(devices []Device) (result []domain.MachineInfo) {
	for _, device := range devices {
		var mc domain.MachineInfo

		mc.Hostname = device.Name
		mc.Serial = device.Serial
		//mc.BootstrapURL
		//mc.DNS
		//mc.JournaldURL
		//mc.NTPServers

		mc.Region = device.Role.Slug
		mc.Site = device.Site.Slug
		mc.Location = device.Location.Slug
		mc.Rack = device.Role.Slug
		mc.Position = device.Position

		mc.Interfaces, mc.Bondings, _ = nb.getInterfacesOfDevice(device.ID)

		result = append(result, mc)
	}

	return result
}

func (nb *Netbox) getInterfacesOfDevice(id int) (physicalIFaces []domain.PhysicalInterface, bondings []domain.BondingConfiguration, err error) {
	response, err := nb.callInterfacesListWithDeviceID(id)

	if err == nil {
		// First of all we need to create the physical interfaces.
		for _, iface := range response.Results {
			if iface.Type.Value != "lag" {
				var physicalIFace domain.PhysicalInterface

				physicalIFace.Name = iface.Display
				physicalIFace.MTU = iface.MTU
				physicalIFace.LAG = iface.Lag.Device.Display
				//physicalIFace.Gateways
				//physicalIFace.IPsWithCIDR
				//physicalIFace.VLANs

				if iface.CountIpaddresses > 0 {
					IPAddresses, _ := nb.getInterfaceIP(iface.ID)
					var items []string

					for _, addr := range IPAddresses {
						items = append(items, addr.Display)
					}
					physicalIFace.IPsWithCIDR = items
				}

				physicalIFaces = append(physicalIFaces, physicalIFace)

			}
		}

		// Then, the bondings have pointers to physical interfaces. That is why we need another loop.
		for _, iface := range response.Results {
			if iface.Type.Value == "lag" {
				var bonding domain.BondingConfiguration

				bonding.Interfaces, _ = nb.getInterfacesOfBonding(iface.ID, &response.Results, &physicalIFaces)
				bonding.Name = iface.Display
				//bonding.Gateways
				//bonding.IPsWithCIDR
				//bonding.VLANs

				if iface.CountIpaddresses > 0 {
					IPAddresses, _ := nb.getInterfaceIP(iface.ID)
					var items []string

					for _, addr := range IPAddresses {
						items = append(items, addr.Display)
					}

					bonding.IPsWithCIDR = items
				}

				bondings = append(bondings, bonding)

			}
		}
	}

	return physicalIFaces, bondings, err
}

func (nb *Netbox) callInterfacesListWithDeviceID(id int) (result APIDeviceInterfaceResponse, err error) {
	uri := fmt.Sprintf("api/dcim/interfaces/?device_id=%v", id)
	resultRaw, _ := nb.getHTTPRequest(uri)

	err = json.Unmarshal(resultRaw, &result)
	return result, err
}

func (nb *Netbox) getInterfacesOfBonding(id int, ifaces *[]DeviceInterface, physicalIFaces *[]domain.PhysicalInterface) (result []*domain.PhysicalInterface, err error) {
	for _, iface := range *ifaces {
		if iface.Lag.ID == id {
			deviceNameToAdd := iface.Name

			for index, physicalIFace := range *physicalIFaces {
				if physicalIFace.Name == deviceNameToAdd {
					item := &(*physicalIFaces)[index]
					result = append(result, item)
				}
			}
		}
	}
	return result, err
}

func (nb *Netbox) getInterfaceIP(id int) (result []IPAddress, err error) {
	var apiResponse APIAddressesResponse

	uri := fmt.Sprintf("api/ipam/ip-addresses/?interface_id=%v", id)
	resultRaw, err := nb.getHTTPRequest(uri)

	if err == nil {
		err = json.Unmarshal(resultRaw, &apiResponse)

		if err == nil {
			result = apiResponse.Results
		}
	}

	return result, err
}
