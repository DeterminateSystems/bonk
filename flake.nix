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
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
    in
    {
      devShells.x86_64-linux.default = pkgs.mkShell {
        name = "bonk";

        buildInputs = with pkgs; [
          go_1_19
          flyctl
          skopeo
          codespell
          nixpkgs-fmt
        ];
      };

      packages.x86_64-linux = rec {
        default = bonk;

        bonk = pkgs.buildGo119Module rec {
          pname = "bonk";
          version = "unreleased";

          src = ./.;

          vendorSha256 = "sha256-xPUh2jh7WLkbPiUKtLp9JxFIo+38Bw0XZYQ897rsTLM=";
        };

        dockerImage = pkgs.dockerTools.buildLayeredImage {
          name = "bonk";
          contents = [ pkgs.cacert bonk ];
          maxLayers = 300;
          config = {
            ExposedPorts."80/tcp" = { };
            Cmd = [ "${bonk}/bin/bonk" ];
          };
        };
      };
    };
}
