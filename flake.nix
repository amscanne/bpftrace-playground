{
  description = "Bpftrace playground image";
  inputs = {
     nixpkgs.url = "github:nixos/nixpkgs/release-24.11";
     flake-utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" ]
     (system:
        let
          pkgs = import nixpkgs { inherit system; };
          binary = pkgs.buildGoModule {
            name = "bpftrace-playground";
            version = "0.0.1";
            src = pkgs.lib.cleanSource ./.;
            vendorHash = "sha256-RQ7Opw7xK1MNZFGxCpEv7MCiKUlYTd6VfHMtL3SZLk8=";
          };
          service = pkgs.dockerTools.buildImage {
            name = "bpftrace-playground";
            tag = "latest";
            copyToRoot = [
              binary
              pkgs.bash
              pkgs.cacert
            ];
            config = {
              Cmd = [ "/bin/bpftrace-playground" ];
              ExposedPorts = {
                "8080/tcp" = {};
              };
            };
          };
          shell = pkgs.mkShell {
            buildInputs = [
              pkgs.go
              pkgs.skopeo
              pkgs.google-cloud-sdk
            ];
          };
        in
        {
          packages = {
            default = service;
          };
          devShells = {
            default = shell;
          };
        });
}
