{
  description = "Patchwork - a tool for building collaborative boards from markdown";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachSystem [ "aarch64-linux" "x86_64-linux" "aarch64-darwin" "x86_64-darwin" ] (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = builtins.replaceStrings ["\n"] [""] (builtins.readFile ./VERSION);
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "patchwork";
            inherit version;
            src = ./.;
            vendorHash = "sha256-symuYFyFd1wleDeOeVtyiqEiXk6eCpJEvCss2RCvsOI=";
            ldflags = [ "-s" "-w" "-X main.version=${self.packages.${system}.default.version}" ];
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
          ];
        };
      }
    );
}
