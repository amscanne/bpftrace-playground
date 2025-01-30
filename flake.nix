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
            src = ./.;
            vendorHash = null;
          };
          base = pkgs.dockerTools.buildImage {
            name = "base";
            tag = "latest";
            copyToRoot = pkgs.buildEnv {
              name = "image-root";
              paths = [ pkgs.bashInteractive ];
              pathsToLink = [ "/bin" ];
            };
          };
          service = pkgs.dockerTools.buildImage {
            name = "bpftrace-playground";
            tag = "latest";
            fromImage = base;
            runAsRoot = "mkdir -p /work";
            copyToRoot = pkgs.buildEnv {
              name = "image-root";
              paths = [ binary ];
              pathsToLink = [ "/bin" ];
            };
            config = {
              Cmd = [ "/bin/bpftrace-playground" ];
              WorkingDir = "/work";
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
