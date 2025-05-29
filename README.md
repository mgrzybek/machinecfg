# machinecfg

Create machine configuration using Netbox and Matchbox.

![Architecture](docs/architecture.svg)

## Usage

### Help message

```console
$ ./machinecfg --help
Creates machines’ configurations to use with matchbox

Usage:
  machinecfg [command]

Available Commands:
  butane      Creates a butane-based YAML document
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  kea         Creates a DHCPv4 configuration
  matchbox    Interact with Matchbox service
  netbox      Interact with Netbox CMDB

Flags:
  -h, --help                     help for machinecfg
  -l, --log-level string         Log level ’development’ (default) or ’production’
  -e, --netbox-endpoint string   URL of the API
  -t, --netbox-token string      Token used to call Netbox API
  -o, --output string            Where to write the result (default 'console')

Use "machinecfg [command] --help" for more information about a command.
```

### How-Tos

To configure machines using Ignition and deploying them using Matchbox: [Using Netbox DCIM to deploy Butane/Ignition-based systems and configure Matchbox](./docs/butane.md)

To configure Kea DHCP server: [Creating the Kea DHCP configuration](./docs/kea.md)
