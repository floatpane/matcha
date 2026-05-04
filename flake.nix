{
  description = "matcha — a beautiful and functional email client for the terminal";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, gomod2nix }:
    flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          gomod2nixPkgs = gomod2nix.legacyPackages.${system};
        in
        {
          packages = rec {
            matcha = gomod2nixPkgs.buildGoApplication {
              go = pkgs.go_1_26;
              pname = "matcha";
              version = self.shortRev or "dev";

              src = ./.;
              modules = ./gomod2nix.toml;

              CGO_ENABLED = 1;

              nativeBuildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [
                pkgs.pkg-config
              ];

              buildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isDarwin [
                pkgs.apple-sdk
              ] ++ pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux [
                pkgs.pcsclite
              ];

              ldflags = [
                "-s"
                "-w"
                "-X main.version=${self.shortRev or "dev"}"
                "-X main.commit=${self.rev or "dirty"}"
                "-X main.date=1970-01-01T00:00:00Z"
              ];

              meta = {
                description = "A beautiful and functional email client for the terminal";
                homepage = "https://github.com/floatpane/matcha";
                license = pkgs.lib.licenses.mit;
                mainProgram = "matcha";
              };
            };
            default = matcha;
          };

          devShells.default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go_1_26
              gopls
              gotools
              gomod2nix.packages.${system}.default
            ];
          };
        }
      ) // {
      overlays.default = final: _prev: {
        matcha = self.packages.${final.system}.matcha;
      };
    };
}
