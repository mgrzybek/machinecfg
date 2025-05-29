# Creating the Kea DHCP configuration

```bash
export KEA_CONF=/etc/kea/dhcp4.json

# Allow MAC addresses found on the Netbox devices database to get an offer
./machinecfg $NETBOX_OPTS kea server4 > $KEA_CONF
```
