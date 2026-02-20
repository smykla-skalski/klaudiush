{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.20.2";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-r07wQDuQG9/Ze9zYKwvThYoEJ2XNIkKfMn8GKfRp+A4=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-i8wp9vDqwuf7WcEi/HWIiB2Po65DmBe3gT1J5BX08oA=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-6UUk4And5q34xptxGT4FFsyceA4kq/hkioylnC/ukng=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-QrXo31dai3f/C7tcETtPL5RKAR7fCXKT6NvtRrcGv2o=";
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
    homepage = "https://github.com/smykla-skalski/klaudiush";
    license = lib.licenses.mit;
    mainProgram = "klaudiush";
    platforms = lib.platforms.unix;
  };
}
