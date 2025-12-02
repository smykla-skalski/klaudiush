{ lib, buildGoModule, rev ? "unknown", shortRev ? "unknown", lastModifiedDate ? "unknown" }:

buildGoModule rec {
  pname = "klaudiush";
  version = "1.9.0";

  src = lib.cleanSource ./..;

  vendorHash = "sha256-IAt0alrCJu9fWxfEjChn400v2OXd+ifar6DrPEIrWhk=";

  subPackages = [ "cmd/klaudiush" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
    "-X main.commit=${shortRev}"
    "-X main.date=${lastModifiedDate}"
  ];

  doCheck = false;

  meta = {
    description = "Validation dispatcher for Claude Code hooks";
    license = lib.licenses.mit;
    mainProgram = "klaudiush";
  };
}
