{
  description = "Validation dispatcher for Claude Code hooks";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      overlay = final: prev: {
        klaudiush = final.callPackage ./package.nix { };
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
