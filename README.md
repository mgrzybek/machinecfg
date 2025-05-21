# machinecfg
Create machine configuration using Netbox and Matchbox

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
  dhcp4       Creates a DHCPv4 configuration
  dhcp6       Creates a DHCPv6 configuration
  help        Help about any command
  matchbox    Interact Matchbox configuration files
  netbox      Interact with Netbox CMDB

Flags:
  -h, --help                     help for machinecfg
  -l, --log-level string         Log level ’development’ (default) or ’production’
  -e, --netbox-endpoint string   URL of the API
  -t, --netbox-token string      Token used to call Netbox API
  -o, --output string            Where to write the results (default "console")

Use "machinecfg [command] --help" for more information about a command.
```

### Using Netbox IPAM to deploy Butane/Ignition-based systems

This section is useful to deploy Flatcar Linux or SLE Micro using Matchbox.

#### Preparing Matchbox environment

We create the required directories to store Matchbox data.

```bash
export BUTANE_ROOT=/tmp/machinecfg
export MATCHBOX_ROOT=/var/lib/matchbox

mkdir -p $BUTANE_ROOT $MATCHBOX_ROOT/{groups,ignition,profiles}
```

We set the endpoint parameters.

```bash
# How to cennect to Netbox
export NETBOX_CREDS="--netbox-endpoint=http://netbox --netbox-token=xxxxxxx"

# What location to process
export NETBOX_LOCATION="--netbox-region=Paris --netbox-site=Seine-et-Marne --netbox-tenant=class-0"

export NETBOX_OPTS="$NETBOX_CREDS $NETBOX_LOCATION"
```

We create the generic profile.

```bash
./machinecfg $NETBOX_OPTS matchbox profile --profile=install \
    > $MATCHBOX_ROOT/profiles/flatcar-install.json
```

We create the ignition configuration file to start the installation process.

```bash
./machinecfg $NETBOX_OPTS butane --profile=install \
    | butane --strict > $MATCHBOX_ROOT/ignition/flatcar-install.ign
```

We link the profile and the ignition file together.

```bash
./machinecfg matchbox group \
    > $MATCHBOX_ROOT/groups/flatcar-install.json
```

#### Creating a butane file per machine

We create the YAML-based butane files first.

```bash
./machinecfg $NETBOX_OPTS butane --output=$BUTANE_ROOT
```

We translate them to ignition files.

```bash
for yaml in $BUTANE_ROOT/*.yml ; do
    export ign=$(echo $yaml | awk -F/ '{gsub("yml","ign") ; print $NF}')
    cat $yaml | butane --strict > $MATCHBOX_ROOT/ignition/$ign
done
```

We create the matchbox profiles of each machine.

```bash
./machinecfg $NETBOX_OPTS matchbox profile --output=$MATCHBOX_ROOT/profiles --profile=disk
```

We link the profile and the ignition file together.

```bash
./machinecfg matchbox group --os-install=true --output=$MATCHBOX_ROOT/groups
```

#### Creating the Kea DHCP configuration

```bash
export KEA_CONF=/etc/kea/dhcp4.json

# Allow MAC addresses found on the Netbox devices database to get an offer
./machinecfg $NETBOX_OPTS dhcp4 server > $KEA_CONF
```
