{ lib, stdenv, fetchurl, installShellFiles }:

let
  version = "1.19.0";

  # Platform-specific release URLs and hashes
  sources = {
    aarch64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_arm64.tar.gz";
      hash = "sha256-MYusOzS53zrzruoKYuuPQ2cxPMeGNe2Qj1i6lVfX070=";
    };
    x86_64-darwin = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_darwin_amd64.tar.gz";
      hash = "sha256-AVPOA8ZoT+D6xNdbh1kREqYNktvaA8pYjTfeDa7kKu4=";
    };
    x86_64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_amd64.tar.gz";
      hash = "sha256-a7UpvU/fESHcwj8ko7f7r91Ol1i54nvpNhr1cRO2tQs=";
    };
    aarch64-linux = {
      url = "https://github.com/smykla-skalski/klaudiush/releases/download/v${version}/klaudiush_${version}_linux_arm64.tar.gz";
      hash = "sha256-xGF341Ndl1pSQWWHvKKdmLH+3jiRTctxyplGQSGZEBk=";
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
