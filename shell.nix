{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    # nativeBuildInputs is usually what you want -- tools you need to run
    nativeBuildInputs = with pkgs; [
      gnumake
      git
      docker-client
      docker-compose

      kubeconform
      kubernetes-helm
      kubesec

      # Golang utilities
      go
      gotestsum
      gofumpt
      golangci-lint
    ];
    hardeningDisable = [ "all" ];
}
