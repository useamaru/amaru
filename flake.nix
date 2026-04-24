{
  description = "Amaru's flake";

  inputs = { nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-25.11"; };

  outputs = { self, nixpkgs }:
    let
      # define system once
      system = "x86_64-linux";
      # use it here, and bind platform-specific packages to `pkgs`
      pkgs = nixpkgs.legacyPackages.${system};
      nativeBuildInputs = with pkgs; [ go ];
      developmentPakcages = with pkgs; [ gopls delve ];
    in {

      packages.${system}.default = pkgs.buildGoModule {
        pname = "amaru";
        version = "0.2.6";
        src = ./.;
        vendorHash = "sha256-OzZzPyNKfbR5K47k/pRa86MfrMYD0/s+PgucZ0HM56U=";
        # subPackages = [ "path1", "path2" ]
        env = { CGO_ENABLED = 0; };
      };
      devShells.${system}.default = pkgs.mkShell {
        packages = nativeBuildInputs ++ developmentPakcages
          ++ [ pkgs.bashInteractive ];
      };
    };
}
