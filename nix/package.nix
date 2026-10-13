{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.44.5";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-Y7yW9kuxchzxh4p0sbGIHM0Lk5/qECebXLGB3FUNXi8=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-pR/EXDWVPgMxRkzZH5MkGNTvLhXHvCyaTSaullA/6UM=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-GH97aDPt6wyRNWmcPOz6kr3QzaoltCGvznz6CuvOW84=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-qAGEofVZvU/GfjyD4xgjxeH1cJdDcjV1kgpRPP9/gUA=";
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
    # Redirect log to a writable tmp path to avoid sandbox directory creation failures
    export KLAUDIUSH_LOG_FILE="$TMPDIR/klaudiush.log"
    installShellCompletion --cmd klaudiush \
      --bash <($out/bin/klaudiush completion bash) \
      --fish <($out/bin/klaudiush completion fish) \
      --zsh <($out/bin/klaudiush completion zsh)

    runHook postInstall
  '';

  meta = {
    description = "Validation dispatcher for Claude Code hooks";
    homepage = "https://github.com/smykla-skalski/klaudiush";
    license = lib.licenses.mit;
    mainProgram = "klaudiush";
    platforms = lib.platforms.unix;
  };
}
