{
  description = "bonk-api";

  inputs.nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1.533189.tar.gz";
  inputs.flake-compat.url = "https://flakehub.com/f/edolstra/flake-compat/1.0.1.tar.gz";

  outputs =
    { self
    , nixpkgs
    , ...
    } @ inputs:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSupportedSystem = f: nixpkgs.lib.genAttrs supportedSystems (system: f {
        pkgs = import nixpkgs { inherit system; };
      });
    in
    {
      devShells = forEachSupportedSystem ({ pkgs }: {
        default = pkgs.mkShell {
          name = "bonk";
          packages = with pkgs; [
            go_1_22
            flyctl
            skopeo
            codespell
            nixpkgs-fmt
          ];
        };
      });

      packages = forEachSupportedSystem ({ pkgs }: rec {
        default = bonk;

        bonk = pkgs.buildGo122Module rec {
          pname = "bonk";
          version = "unreleased";

          src = ./.;

          vendorHash = "sha256-iBZ10bWOzn9aRvSdYwGTph+f0/nQinijyqX0oB0WC9A=";
        };

        dockerImage =
          let
            linuxPkgs = nixpkgs.legacyPackages.x86_64-linux;
          in
          pkgs.dockerTools.buildLayeredImage {
            name = "bonk";
            contents = [ linuxPkgs.cacert bonk ];
            maxLayers = 300;
            config = {
              ExposedPorts."80/tcp" = { };
              Cmd = [ "${self.packages.x86_64-linux.bonk}/bin/bonk" ];
            };
          };
      });
    };
}
