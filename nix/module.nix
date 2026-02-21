{ config, lib, pkgs, ... }:

let
  cfg = config.programs.klaudiush;
  xdgConfigDir = "${config.xdg.configHome}/klaudiush";
  xdgDataDir = "${config.xdg.dataHome}/klaudiush";
  xdgStateDir = "${config.xdg.stateHome}/klaudiush";
  legacyDir = "${config.home.homeDirectory}/.klaudiush";
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
        If set, a symlink will be created at the XDG config location.
      '';
    };

    useLegacyPaths = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = ''
        Use legacy ~/.klaudiush/ paths instead of XDG base directories.
        Only enable this if you need backward compatibility with older setups.
      '';
    };

    createDynamicDirs = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Whether to create dynamic directories that persist across rebuilds.
        Uses XDG directories by default, or legacy paths if useLegacyPaths is set.
      '';
    };

    extraDynamicDirs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = ''
        Additional directories to create under the data directory.
        These will persist across rebuilds.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    home.activation.klaudiushSetup = lib.hm.dag.entryAfter [ "writeBoundary" ] (
      if cfg.useLegacyPaths then ''
        # Legacy mode: use ~/.klaudiush/
        run mkdir -p "${legacyDir}"

        ${lib.optionalString cfg.createDynamicDirs ''
          run mkdir -p "${legacyDir}/logs"
          run mkdir -p "${legacyDir}/backup"
          run mkdir -p "${legacyDir}/cache"
        ''}

        ${lib.optionalString (cfg.extraDynamicDirs != [ ]) ''
          ${lib.concatMapStringsSep "\n" (dir: ''run mkdir -p "${legacyDir}/${dir}"'') cfg.extraDynamicDirs}
        ''}

        ${lib.optionalString (cfg.configFile != null) ''
          run ln -sf "${cfg.configFile}" "${legacyDir}/config.toml"
        ''}
      '' else ''
        # XDG mode: create config, data, and state directories
        run mkdir -p -m 0700 "${xdgConfigDir}"
        run mkdir -p -m 0700 "${xdgDataDir}"
        run mkdir -p -m 0700 "${xdgStateDir}"

        ${lib.optionalString cfg.createDynamicDirs ''
          run mkdir -p -m 0700 "${xdgDataDir}/crash_dumps"
          run mkdir -p -m 0700 "${xdgDataDir}/patterns"
          run mkdir -p -m 0700 "${xdgDataDir}/backups"
          run mkdir -p -m 0700 "${xdgDataDir}/plugins"
          run mkdir -p -m 0700 "${xdgDataDir}/exceptions"
        ''}

        ${lib.optionalString (cfg.extraDynamicDirs != [ ]) ''
          ${lib.concatMapStringsSep "\n" (dir: ''run mkdir -p -m 0700 "${xdgDataDir}/${dir}"'') cfg.extraDynamicDirs}
        ''}

        ${lib.optionalString (cfg.configFile != null) ''
          run ln -sf "${cfg.configFile}" "${xdgConfigDir}/config.toml"
        ''}
      ''
    );
  };
}
