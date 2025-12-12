{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.12.5";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-fWPKDq/PagXTESaMgZ45Z3EcebruCmPjSQRrbriB14U=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-cjfKbuD7WOw0ItEYSmGwSjdBITKkyrPSg+uMx0NkpBM=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-WK2vwBzkDbUFB5HGfz5qOwAPVSaEYQU0wUv6HpgoMEw=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-labs/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-3aXa7crj5tkF2yxMd2hrN0eC0OiPJfQAwfJu0I/k8Es=";
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
