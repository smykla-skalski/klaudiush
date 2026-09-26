package parser

import "strings"

// gitProgram is the program other invocations of git resolve to.
const gitProgram = "git"

// nameSet builds a set from a space-separated list of names.
func nameSet(list string) map[string]bool {
	set := make(map[string]bool)

	for name := range strings.FieldsSeq(list) {
		set[name] = true
	}

	return set
}

// gitBuiltins are git's own commands. Git ignores an alias that shares a
// builtin's name, so only other names are looked up as aliases.
var gitBuiltins = nameSet(`add am annotate apply archive bisect blame branch bundle
	cat-file check-attr check-ignore check-mailmap check-ref-format checkout
	checkout-index cherry cherry-pick citool clean clone column commit
	commit-graph commit-tree config count-objects credential daemon describe
	diagnose diff diff-files diff-index diff-tree difftool fast-export
	fast-import fetch fetch-pack filter-branch fmt-merge-msg for-each-ref
	for-each-repo format-patch fsck gc get-tar-commit-id grep gui hash-object
	help hook http-backend index-pack init instaweb interpret-trailers log
	ls-files ls-remote ls-tree mailinfo mailsplit maintenance merge merge-base
	merge-file merge-index merge-tree mergetool mktag mktree multi-pack-index mv
	name-rev notes pack-objects pack-redundant pack-refs patch-id prune
	prune-packed pull push quiltimport range-diff read-tree rebase receive-pack
	reflog refs remote repack replace replay request-pull rerere reset restore
	rev-list rev-parse revert rm scalar send-email send-pack shortlog show
	show-branch show-index show-ref sparse-checkout stage stash status
	stripspace submodule switch symbolic-ref tag unpack-file unpack-objects
	update-index update-ref update-server-info upload-archive upload-pack var
	verify-commit verify-pack verify-tag version whatchanged worktree write-tree`)

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
var dataCommands = nameSet(`ack ag apropos awk bat cat chmod chown cp cut
	declare echo egrep export fgrep file grep head help history info jq less ln
	local ls man mkdir more mv printenv printf read readonly rg rm sed sort stat
	tail test tldr touch tr type unalias uniq unset wc whatis whereis which yq`)
