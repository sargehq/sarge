{
  description = "Sarge - Orchestrate code agents to process issues and create PRs";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = self.shortRev or self.dirtyShortRev or "unknown";
      in
      {
        packages = {
          sarge = pkgs.buildGoModule {
            pname = "sarge";
            inherit version;
            src = self;

            vendorHash = "sha256-R8KJZRt0lAb+fd4dHpsKbrsDhO5M682boVMY+g4WIrQ=";

            env.CGO_ENABLED = "0";

            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            meta = with pkgs.lib; {
              description = "Orchestrate code agents to process issues and create PRs";
              homepage = "https://github.com/sargehq/sarge";
              license = licenses.asl20;
              mainProgram = "sarge";
              platforms = platforms.unix;
            };
          };

          default = self.packages.${system}.sarge;
        };

        apps = {
          sarge = flake-utils.lib.mkApp {
            drv = self.packages.${system}.sarge;
          };
          default = self.apps.${system}.sarge;
        };
      }
    );
}
