{ lib, buildGoModule, installShellFiles, rev ? "unknown", shortRev ? "unknown", lastModifiedDate ? "unknown" }:

buildGoModule rec {
  pname = "klaudiush";
  version = "1.10.0";

  src = lib.cleanSource ./..;

  vendorHash = "sha256-IAt0alrCJu9fWxfEjChn400v2OXd+ifar6DrPEIrWhk=";

  subPackages = [ "cmd/klaudiush" ];

  nativeBuildInputs = [ installShellFiles ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${shortRev}"
    "-X main.date=${lastModifiedDate}"
  ];

  postInstall = ''
    # Generate shell completions
    installShellCompletion --cmd klaudiush \
      --bash <($out/bin/klaudiush completion bash) \
      --fish <($out/bin/klaudiush completion fish) \
      --zsh <($out/bin/klaudiush completion zsh)
  '';

  doCheck = false;

  meta = {
    description = "Validation dispatcher for Claude Code hooks";
    license = lib.licenses.mit;
    mainProgram = "klaudiush";
  };
}
