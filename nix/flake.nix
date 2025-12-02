{
  description = "Validation dispatcher for Claude Code hooks";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      rev = self.rev or self.dirtyRev or "unknown";
      shortRev = self.shortRev or self.dirtyShortRev or "unknown";
      lastModifiedDate = self.lastModifiedDate or "unknown";
      overlay = final: prev: {
        klaudiush = final.callPackage ./package.nix {
          inherit rev shortRev lastModifiedDate;
        };
      };
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ overlay ];
        };
      in
      {
        packages = {
          default = pkgs.klaudiush;
          klaudiush = pkgs.klaudiush;
        };

        apps.default = flake-utils.lib.mkApp {
          drv = pkgs.klaudiush;
        };
      }
    ) // {
      overlays.default = overlay;
      homeManagerModules.default = import ./module.nix;
    };
}
