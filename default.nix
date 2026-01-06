{
    pkgs ? import <nixpkgs> {},
    version ? "unstable",
    tag ? "latest"
}:

let
  machinecfg-package = pkgs.buildGoModule {
    pname = "machinecfg";
    version = "${version}";
    src = pkgs.lib.cleanSource ./.;

    # Let's create a static binary
    buildFlagsArray = [ "-ldflags" "-s -w" ];

    vendorHash = "sha256-lYlPtMX0m/ZcPIwNKMUJOHy3tklH9QVh+ysOwb8d5X4=";

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
