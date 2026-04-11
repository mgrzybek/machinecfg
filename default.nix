{
    pkgs ? import <nixpkgs> {},
    version ? "unstable",
    tag ? "latest"
}:

let
  machinecfg-package = pkgs.buildGo125Module {
    pname = "machinecfg";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;

    # Let's create a static binary
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    vendorHash = "sha256-zu2WpbrRT3pCcOwJBowku/W92CX7VD2h5GC9OmovBGA=";
  };

  machinecfg-controller-kubernetes-updater-package = pkgs.buildGo125Module {
    pname = "machinecfg-controller-kubernetes-updater";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;

    subPackages = [ "cmd/controller-kubernetes-updater" ];

    buildFlagsArray = [ "-ldflags" "-s -w" ];

    # Update after running: go mod tidy && nix-build -A machinecfg-controller-kubernetes-updater-oci
    vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  };

  machinecfg-controller-netbox-updater-package = pkgs.buildGo125Module {
    pname = "machinecfg-controller-netbox-updater";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;

    subPackages = [ "cmd/controller-netbox-updater" ];

    # Static binary, stripped for minimal image size.
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    # Update after running: go mod tidy && nix-build -A machinecfg-controller-netbox-updater-oci
    vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
  };
in {
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

    # Generate a CycloneDX SBOM alongside the image.
    # Requires pkgs.syft in nativeBuildInputs of the surrounding derivation,
    # or invoke separately: syft packages dir:<image-root> -o cyclonedx-json
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
}
