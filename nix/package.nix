{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.12.8";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-vJA8RSZ3xrd959ETlWAaKz/93oe32pArLMYghHm2h98=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-Yy6xcxJHc9LcwcdJoBrBqJ44bPLhznY/AS5YasgCeeQ=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-I0UTrTmqxfYHPedOPJmIF/o361/ZfBZ+2o/3GEnp26g=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-uUydMEZH/aeM7wyfNK9QMWa0y5hxxHJLc7k27Dg+tM4=";
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
