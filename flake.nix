{
  description = "matcha — a beautiful and functional email client for the terminal";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = rec {
          matcha = pkgs.buildGoModule.override { go = pkgs.go_1_26; } {
            pname = "matcha";
            version = self.shortRev or "dev";

            src = ./.;

            vendorHash = "sha256-fZnAZwwQH2WNewS4pEkl7Bko4smdgo5omkdtA1voXkY=";

            env.CGO_ENABLED = 0;

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
          ];
        };
      }
    ) // {
      overlays.default = final: _prev: {
        matcha = self.packages.${final.system}.matcha;
      };
    };
}
