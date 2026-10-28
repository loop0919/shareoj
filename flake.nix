{
  description = "Development environment for the judge API, web, and infrastructure";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            config.allowUnfreePredicate = pkg: nixpkgs.lib.getName pkg == "terraform";
          };
          # The frontend runs on Bun in development, CI, and Lambda. Keep this version equal to
          # web/package.json's packageManager and web/scripts/bun_release.py, which checks it.
          bun = pkgs.bun.overrideAttrs (
            finalAttrs: previousAttrs: {
              version = "1.4.4";
              src = finalAttrs.passthru.sources.${system};
              passthru = previousAttrs.passthru // {
                sources =
                  builtins.mapAttrs
                    (
                      _: source:
                      pkgs.fetchurl {
                        url = "https://github.com/oven-sh/bun/releases/download/bun-v${finalAttrs.version}/${source.asset}";
                        inherit (source) hash;
                      }
                    )
                    {
                      aarch64-darwin = {
                        asset = "bun-darwin-aarch64.zip";
                        hash = "sha256-wkDX8My5DCY6hJ/NsJt+5iYw2SE0jtaq/ahrnVd7xRA=";
                      };
                      aarch64-linux = {
                        asset = "bun-linux-aarch64.zip";
                        hash = "sha256-uqa7NvUdblhWHgK76oJQOKnfRU9nPW13bou/hvjHhQ4=";
                      };
                      x86_64-linux = {
                        asset = "bun-linux-x64-baseline.zip";
                        hash = "sha256-4ERutVmVdy65iUvrrppr42jZhai5Jx7hJEhvmUbu3XY=";
                      };
                    };
              };
            }
          );
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go
              python3
              gofumpt
              golangci-lint
              gopls
              bun
              postgresql_17
              gnumake
              awscli2
              terraform
              zip
            ];

            # Follow go.mod's toolchain line so local builds match CI, even before
            # nixpkgs packages that patch release. Go verifies it with the checksum database.
            GOTOOLCHAIN = "auto";
          };
        }
      );

      formatter = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        pkgs.nixfmt
      );
    };
}
