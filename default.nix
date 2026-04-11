{
    pkgs ? import <nixpkgs> {},
    version ? "unstable",
    tag ? "latest"
}:
let
  meta = {
    description = "MachineCFG is a set of specialized tools designed to bridge the gap between your Inventory Management System (NetBox) and your provisioning stack (Tinkerbell, Flatcar Linux, Talos Linux or Kamaji).";
    homepage = "https://github.com/mgrzybek/machinecfg";
    license = pkgs.lib.licenses.gpl3;
    maintainers = with pkgs.lib.maintainers; [ mgrzybek ];
  };

  # Generates a CycloneDX SBOM from the Nix closure of a package.
  # sbomnix traverses the Nix store graph directly — more accurate than
  # filesystem scanners (syft) for Nix-built binaries.
  mkSbom = name: pkg: pkgs.runCommand "${name}-sbom" {
    nativeBuildInputs = [ pkgs.sbomnix ];
  } ''
    mkdir -p $out
    sbomnix --buildtime --cdx=$out/${name}.cdx.json --spdx=$out/${name}.spdx.json --csv=$out/${name}.csv ${pkg}
  '';

  # Generates a Helm package
  mkHelm = name: version: pkgs.stdenv.mkDerivation {
    pname = "${name}-helm";
    version = version;

    src = ./charts;

    nativeBuildInputs = [ pkgs.kubernetes-helm ];

    installPhase = ''
      helm package ${name} --destination $out
    '';
  };


  machinecfg-package = pkgs.buildGo125Module {
    pname = "machinecfg";
    version = "${version}";
    meta = meta;

    src = pkgs.lib.cleanSource ./.;

    subPackages = [ "cli/machinecfg" ];

    # Static binary, stripped for minimal image size.
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    vendorHash = "sha256-E7zWyGqHxahMyXTinIRM0RPE1YeXHtj7TaDfkJMSYdQ=";
  };

  machinecfg-controller-kubernetes-updater-package = pkgs.buildGo125Module {
    pname = "machinecfg-controller-kubernetes-updater";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;
    meta = meta;

    subPackages = [ "controllers/controller-kubernetes-updater" ];

    # Static binary, stripped for minimal image size.
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    vendorHash = "sha256-E7zWyGqHxahMyXTinIRM0RPE1YeXHtj7TaDfkJMSYdQ=";
  };

  machinecfg-controller-netbox-updater-package = pkgs.buildGo125Module {
    pname = "machinecfg-controller-netbox-updater";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;
    meta = meta;

    subPackages = [ "controllers/controller-netbox-updater" ];

    # Static binary, stripped for minimal image size.
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    vendorHash = "sha256-E7zWyGqHxahMyXTinIRM0RPE1YeXHtj7TaDfkJMSYdQ=";
  };
in rec {
  machinecfg-oci = pkgs.dockerTools.buildImage {
    name = "machinecfg";
    tag = "${tag}";

    copyToRoot = pkgs.buildEnv {
      name = "image-root-copy";
      paths = [
        machinecfg-package
      ];
    };

    config = {
      Cmd = [ "/bin/machinecfg/bin/machinecfg" ];
      User = "1000:1000";
    };
  };

  machinecfg-controller-netbox-updater-oci = pkgs.dockerTools.buildImage {
    name = "machinecfg-controller-netbox-updater";
    tag = "${tag}";

    copyToRoot = pkgs.buildEnv {
      name = "image-root-copy";
      paths = [
        machinecfg-controller-netbox-updater-package
      ];
    };

    config = {
      Cmd = [ "/bin/machinecfg-controller-netbox-updater" ];
      User = "1000:1000";
    };
  };

  machinecfg-controller-kubernetes-updater-oci = pkgs.dockerTools.buildImage {
    name = "machinecfg-controller-kubernetes-updater";
    tag = "${tag}";

    copyToRoot = pkgs.buildEnv {
      name = "image-root-copy";
      paths = [ machinecfg-controller-kubernetes-updater-package ];
    };

    config = {
      Cmd = [ "/bin/machinecfg-controller-kubernetes-updater" ];
      User = "1000:1000";
    };
  };

  # SBOMs: binaries
  machinecfg-sbom = mkSbom "machinecfg" machinecfg-package;
  machinecfg-controller-netbox-updater-sbom = mkSbom "machinecfg-controller-netbox-updater" machinecfg-controller-netbox-updater-package;
  machinecfg-controller-kubernetes-updater-sbom = mkSbom "machinecfg-controller-kubernetes-updater" machinecfg-controller-kubernetes-updater-package;

  # SBOMs: oci images
  machinecfg-oci-sbom = mkSbom "machinecfg-oci" machinecfg-oci;
  machinecfg-controller-netbox-updater-oci-sbom = mkSbom "machinecfg-controller-netbox-updater-oci" machinecfg-controller-netbox-updater-oci;
  machinecfg-controller-kubernetes-updater-oci-sbom = mkSbom "machinecfg-controller-kubernetes-updater-oci" machinecfg-controller-kubernetes-updater-oci;

  # Helm charts
  machinecfg-controller-netbox-updater-helm = mkHelm "machinecfg-controller-netbox-updater" "0.1.0";
  machinecfg-controller-kubernetes-updater-helm = mkHelm "machinecfg-controller-kubernetes-updater" "0.1.0";
}
