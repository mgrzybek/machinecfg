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
