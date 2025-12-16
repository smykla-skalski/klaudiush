{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.12.9";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-p8KtL7/wonGA8B6JKkfZHz+vAnvrENWYfuvVcgSvDMk=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-9f4G84vGa/7aBBKGF+2yWBO+6PgwOjNRPcwDkwWN7w8=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-W5k2kBqISalCnSrAGRygsLDU2gwtjTEihrKieVMI7J4=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-FLQrBwreaD7Slp1MPJJ18KH1XgvZbvlpLwHn+ZKeV9E=";
    };
  };

  source = sources.${stdenv.hostPlatform.system} or (throw "Unsupported platform: ${stdenv.hostPlatform.system}");
in
stdenv.mkDerivation {
  pname = "klaudiush";
  inherit version;

  src = fetchurl {
    inherit (source) url hash;
  };

  nativeBuildInputs = [ installShellFiles ];

  sourceRoot = ".";

  installPhase = ''
    runHook preInstall

    install -Dm755 klaudiush $out/bin/klaudiush

    # Generate and install shell completions
    installShellCompletion --cmd klaudiush \
      --bash <($out/bin/klaudiush completion bash) \
      --fish <($out/bin/klaudiush completion fish) \
      --zsh <($out/bin/klaudiush completion zsh)

    runHook postInstall
  '';

  meta = {
    description = "Validation dispatcher for Claude Code hooks";
    homepage = "https://github.com/smykla-labs/klaudiush";
    license = lib.licenses.mit;
    mainProgram = "klaudiush";
    platforms = lib.platforms.unix;
  };
}
