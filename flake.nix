{
  description = "bonk-api";

  inputs.nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1";
  inputs.flake-compat.url = "https://flakehub.com/f/edolstra/flake-compat/1";

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
            go
            flyctl
            skopeo
            codespell
            nixpkgs-fmt
          ];
        };
      });

      packages = forEachSupportedSystem ({ pkgs }: rec {
        default = bonk;

        bonk = pkgs.buildGoModule {
          pname = "bonk";
          version = "unreleased";

          src = ./.;

          vendorHash = "sha256-hks/ItrAxVImBPb83+Q/nHW+KeayAtco2GCUPmBS8Vo=";
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
