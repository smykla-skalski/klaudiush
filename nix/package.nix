{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.12.10";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-S8rJ2XxLDn0MAiX2T7fi6iE1gquRg+ZtUl7WWI5JG2g=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-Dx+hiIxMeJxvtH3DP8P5j4mwN8ymRaho3LNAZgZul0A=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-q89gIGXm8nlutQAMFMiDz/2Cq60MBecRAOhlZOHFcfU=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-K5MGEDi9EU3pwJMv4fWVlz+ZQ0f3JZceS5EcvIiB+Nw=";
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
