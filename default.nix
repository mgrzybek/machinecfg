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

    vendorHash = "sha256-Pfmvw1SAO/YssLTJwTfc/2DQKATnMA9srBTYBG7FYYU=";

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
