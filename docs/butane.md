# Using Netbox DCIM to deploy Butane/Ignition-based systems and configure Matchbox

This section is useful to deploy Flatcar Linux or SLE Micro using Matchbox.

## Preparing Matchbox environment

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

## Creating a butane file per machine

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
