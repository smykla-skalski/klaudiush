{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.39.3";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-uu0RpDHvhTyY8ULVo9eIvP+pH+AnRT7Nux3neX3V4F4=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-cd13QIvGJjXLe5b/amHXTyYlwkXAr2/2RimGwsCKOdU=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-h8X0REhUOVg2uw0pT2j81NZVjuzTYGQxcPgYiQ59ZEQ=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-o3YX498vBVAJD/nzveKd/VRl7nLBJyXiBBNThizu7mU=";
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
