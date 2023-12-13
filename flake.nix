{
  description = "bonk-api";

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  inputs.flake-compat = {
    url = "github:edolstra/flake-compat";
    flake = false;
  };

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
            go_1_19
            flyctl
            skopeo
            codespell
            nixpkgs-fmt
          ];
        };
      });

      packages = forEachSupportedSystem ({ pkgs }: rec {
        default = bonk;

        bonk = pkgs.buildGo119Module rec {
          pname = "bonk";
          version = "unreleased";

          src = ./.;

          vendorSha256 = "sha256-TFNoAjqyFHuFPURobqorWkChYiR2pi8TqAAn2TpDFDg=";
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
