package parser

import "strings"

const (
	// gitProgram is the program other invocations of git resolve to.
	gitProgram = "git"
	// hubCLI is GitHub's git wrapper, which runs git for any git command.
	hubCLI = "hub"
)

// nameSet builds a set from a space-separated list of names.
func nameSet(list string) map[string]bool {
	set := make(map[string]bool)

	for name := range strings.FieldsSeq(list) {
		set[name] = true
	}

	return set
}

// gitBuiltins are the commands built into git (git --list-cmds=builtins).
// Git ignores an alias that shares a builtin's name. Script commands such as
// send-email are left out: git honours an alias for them when they are not
// installed, and the resolver checks whether they are.
var gitBuiltins = nameSet(`add am annotate apply archive backfill bisect blame branch
	bugreport bundle cat-file check-attr check-ignore check-mailmap check-ref-format
	checkout checkout--worker checkout-index cherry cherry-pick clean clone column
	commit commit-graph commit-tree config count-objects credential credential-cache
	credential-cache--daemon credential-store describe diagnose diff diff-files
	diff-index diff-pairs diff-tree difftool fast-export fast-import fetch fetch-pack
	fmt-merge-msg for-each-ref for-each-repo format-patch format-rev fsck fsck-objects
	fsmonitor--daemon gc get-tar-commit-id grep hash-object help history hook
	index-pack init init-db interpret-trailers last-modified log ls-files ls-remote
	ls-tree mailinfo mailsplit maintenance merge merge-base merge-file merge-index
	merge-ours merge-recursive merge-recursive-ours merge-recursive-theirs
	merge-subtree merge-tree mktag mktree multi-pack-index mv name-rev notes
	pack-objects pack-redundant pack-refs patch-id pickaxe prune prune-packed pull
	push range-diff read-tree rebase receive-pack reflog refs remote remote-ext
	remote-fd repack replace replay repo rerere reset restore rev-list rev-parse
	revert rm send-pack shortlog show show-branch show-index show-ref sparse-checkout
	stage stash status stripspace submodule--helper switch symbolic-ref tag
	unpack-file unpack-objects update-index update-ref update-server-info
	upload-archive upload-archive--writer upload-pack url-parse var verify-commit
	verify-pack verify-tag version whatchanged worktree write-tree`)

// ghBuiltins are gh's own commands. Any other first word is an alias or an
// extension.
var ghBuiltins = nameSet(`agent-task alias api attestation auth browse cache codespace
	completion config copilot extension gist gpg-key help issue label org preview pr
	project release repo ruleset run search secret ssh-key status variable workflow`)

// ghGlobalValueFlags are gh options that may come before the command and
// take the next argument as their value.
var ghGlobalValueFlags = nameSet("-R --repo --hostname")

// validatedGitSubcommands are the git subcommands klaudiush validates. An
// unknown program invoked with one of them is checked as git.
var validatedGitSubcommands = nameSet(
	"add branch checkout cherry-pick commit fetch merge push revert switch tag",
)

// validatedGHCommands are the gh commands klaudiush validates.
var validatedGHCommands = nameSet("api issue pr")

// shellBuiltins run inside the shell, so no file on disk explains them and
// they must never be mistaken for a missing program.
var shellBuiltins = nameSet(`. : [ [[ alias bg bind break builtin caller cd command
	compgen complete compopt continue declare dirs disown echo enable eval exec
	exit export false fc fg getopts hash help history jobs kill let local logout
	mapfile popd printf pushd pwd read readarray readonly return select set shift
	shopt source suspend test time times trap true type typeset ulimit umask
	unalias unset wait`)

// dataCommands only read, print or file their arguments, so a git word among
// them is data rather than a command being launched.
var dataCommands = nameSet(`ack ag apropos bat cat chmod chown cp cut
	declare echo egrep export fgrep file grep head help history info jq less ln
	local ls man mkdir more mv printenv printf read readonly rg rm sed sort stat
	tail test tldr touch tr type unalias uniq unset wc whatis whereis which yq`)
