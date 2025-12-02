{ config, lib, pkgs, ... }:

let
  cfg = config.programs.klaudiush;
  klaudiushDir = "${config.home.homeDirectory}/.klaudiush";
  claudeHooksDir = "${config.home.homeDirectory}/.claude/hooks";
in
{
  options.programs.klaudiush = {
    enable = lib.mkEnableOption "klaudiush - validation dispatcher for Claude Code hooks";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.klaudiush;
      defaultText = lib.literalExpression "pkgs.klaudiush";
      description = "The klaudiush package to use.";
    };

    configFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = ''
        Path to the klaudiush configuration file.
        If set, a symlink will be created at ~/.klaudiush/config.toml.
      '';
    };

    createDynamicDirs = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Whether to create dynamic directories that persist across rebuilds.
        These include logs, backup, cache, and plugins directories.
      '';
    };

    extraDynamicDirs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = ''
        Additional directories to create under ~/.klaudiush/.
        These will persist across rebuilds.
      '';
    };

    installHook = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Whether to install klaudiush as a Claude Code hook dispatcher.
        Creates a symlink at hookSymlinkPath pointing to the klaudiush binary.
      '';
    };

    hookSymlinkPath = lib.mkOption {
      type = lib.types.str;
      default = "${claudeHooksDir}/dispatcher";
      defaultText = lib.literalExpression ''"''${config.home.homeDirectory}/.claude/hooks/dispatcher"'';
      description = "Path where the hook symlink should be created.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    home.activation.klaudiushSetup = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
      # Create base klaudiush directory
      run mkdir -p "${klaudiushDir}"

      ${lib.optionalString cfg.createDynamicDirs ''
        # Create standard dynamic directories
        run mkdir -p "${klaudiushDir}/logs"
        run mkdir -p "${klaudiushDir}/backup"
        run mkdir -p "${klaudiushDir}/cache"
        run mkdir -p "${klaudiushDir}/plugins"
      ''}

      ${lib.optionalString (cfg.extraDynamicDirs != [ ]) ''
        # Create extra dynamic directories
        ${lib.concatMapStringsSep "\n" (dir: ''run mkdir -p "${klaudiushDir}/${dir}"'') cfg.extraDynamicDirs}
      ''}

      ${lib.optionalString (cfg.configFile != null) ''
        # Symlink config file
        run ln -sf "${cfg.configFile}" "${klaudiushDir}/config.toml"
      ''}

      ${lib.optionalString cfg.installHook ''
        # Create Claude hooks directory and symlink dispatcher
        run mkdir -p "${claudeHooksDir}"
        run ln -sf "${cfg.package}/bin/klaudiush" "${cfg.hookSymlinkPath}"
      ''}
    '';
  };
}
