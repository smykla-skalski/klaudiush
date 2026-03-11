{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.29.1";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-KJJu1LP6C6iNebUDZKtDwxrzp56PHzVfbYWl2MK+Zfc=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-G4yGgSfRLS9I3vNTVKH8pcJ/FKLoNam1EKUeWnzStpQ=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-NKzBlqKt9pTBRfXu5KksJ1irHtSGFNvtRdBzDNcW8YA=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-72IEkrlpe09pFb+zFKvyYxHvwLW2PMngLQrm8LJToT8=";
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
