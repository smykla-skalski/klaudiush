package github_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	validatorpkg "github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/github"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("APIValidator", func() {
	var (
		apiValidator *github.APIValidator
		ctx          context.Context
	)

	bashContext := func(command string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		}
	}

	BeforeEach(func() {
		apiValidator = github.NewAPIValidator(nil, logger.NewNoOpLogger(), nil)
		ctx = context.Background()
	})

	Describe("commit-creating calls", func() {
		DescribeTable(
			"blocks the call",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Reference.Code()).To(Equal("GH002"))
				Expect(result.FixHint).To(ContainSubstring("git commit -sS"))
			},
			Entry(
				"contents PUT",
				`gh api --method PUT /repos/o/r/contents/README.md -f message=x -f content=y`,
			),
			Entry(
				"contents DELETE",
				`gh api -X DELETE repos/o/r/contents/README.md -f message=x -f sha=abc`,
			),
			Entry(
				"git commits POST",
				`gh api repos/o/r/git/commits -f message=x -f tree=abc`,
			),
			Entry(
				"merges POST",
				`gh api repos/o/r/merges -f base=main -f head=topic`,
			),
			Entry(
				"full URL form",
				`gh api -X PUT https://api.github.com/repos/o/r/contents/x.txt -f message=x`,
			),
			Entry(
				"repository spelled from variables",
				`gh api -X PUT "repos/$OWNER/$REPO/contents/README.md" -f message=x`,
			),
			Entry(
				"one stage of a pipeline",
				`gh api -X PUT repos/o/r/contents/README.md -f message=x | jq .commit.sha`,
			),
			Entry(
				"one stage of a compound command",
				`git status && gh api -X PUT repos/o/r/contents/README.md -f message=x`,
			),
			Entry(
				"graphql createCommitOnBranch",
				`gh api graphql -f query='mutation { createCommitOnBranch(input: $i) { commit { url } } }'`,
			),
			Entry(
				"graphql on GitHub Enterprise Server",
				`gh api https://ghe.example.com/api/graphql `+
					`-f query='mutation { createCommitOnBranch(input: $i) { url } }'`,
			),
			Entry(
				"git commits POST with the field flag value attached",
				`gh api repos/o/r/git/commits -fmessage=x -ftree=abc`,
			),
			Entry(
				"merges POST with the field flag value attached",
				`gh api repos/o/r/merges -Fbase=main -Fhead=topic`,
			),
			Entry(
				"curl with the data flag value attached",
				`curl -d'{"base":"main"}' https://api.github.com/repos/o/r/merges`,
			),
		)

		It("names the method, the endpoint and the intended path", func() {
			result := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X PUT repos/o/r/contents/README.md -f message=x`),
			)

			Expect(result.Message).To(ContainSubstring("PUT repos/o/r/contents/README.md"))
			Expect(result.Details["help"]).To(ContainSubstring("git commit -sS"))
		})
	})

	Describe("calls that create no commit", func() {
		DescribeTable(
			"passes the call",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))
				Expect(result.Passed).To(BeTrue())
			},
			Entry(
				"branch creation",
				`gh api -X POST repos/o/r/git/refs -f ref=refs/heads/topic -f sha=abc`,
			),
			Entry(
				"reading contents",
				`gh api repos/o/r/contents/README.md`,
			),
			Entry(
				"reading contents through a pipeline",
				`gh api repos/o/r/contents/README.md --jq .sha | cat`,
			),
			Entry(
				"listing workflow runs",
				`gh api repos/o/r/actions/runs`,
			),
			Entry(
				"creating a pull request",
				`gh api -X POST repos/o/r/pulls -f title=x -f head=topic -f base=main`,
			),
			Entry(
				"merging a pull request, checked like gh pr merge instead",
				`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`,
			),
			Entry(
				"merging a pull request with --method",
				`gh api --method PUT repos/o/r/pulls/42/merge`,
			),
			Entry(
				"merging a pull request with curl",
				`curl -X PUT https://api.github.com/repos/o/r/pulls/42/merge `+
					`-d '{"merge_method":"squash"}'`,
			),
			Entry(
				"merging a pull request with httpie",
				`http PUT https://api.github.com/repos/o/r/pulls/42/merge merge_method=squash`,
			),
			Entry(
				"merging a pull request on GitHub Enterprise Server",
				`curl -X PUT https://ghe.example.com/api/v3/repos/o/r/pulls/42/merge -d '{}'`,
			),
			Entry(
				"a harmless graphql query",
				`gh api graphql -f query='query { viewer { login } }'`,
			),
			Entry(
				"gh pr create",
				`gh pr create --title "feat(api): x" --body y`,
			),
			Entry(
				"a plain git commit",
				`git commit -sS -m "feat(api): x"`,
			),
		)
	})

	Describe("clients other than gh", func() {
		DescribeTable(
			"blocks the call",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Reference.Code()).To(Equal("GH002"))
			},
			Entry(
				"curl with an explicit method",
				`curl -X PUT -H "Authorization: bearer $T" `+
					`https://api.github.com/repos/o/r/contents/README.md -d '{"message":"x"}'`,
			),
			Entry(
				"curl with the method attached to the flag",
				`curl -XDELETE https://api.github.com/repos/o/r/contents/README.md`,
			),
			Entry(
				"curl whose method is implied by --upload-file",
				`curl -T body.json https://api.github.com/repos/o/r/contents/README.md`,
			),
			Entry(
				"curl whose method is implied by --data",
				`curl --data '{"base":"main"}' https://api.github.com/repos/o/r/merges`,
			),
			Entry(
				"curl using --url instead of a positional",
				`curl -X PUT --url https://api.github.com/repos/o/r/contents/x`,
			),
			Entry(
				"curl against GitHub Enterprise Server",
				`curl -X PUT https://ghe.example.com/api/v3/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"wget",
				`wget --method=PUT --body-data='{"message":"x"}' `+
					`https://api.github.com/repos/o/r/contents/README.md`,
			),
			Entry(
				"httpie",
				`http PUT https://api.github.com/repos/o/r/contents/README.md message=x`,
			),
			Entry(
				"xh",
				`xh DELETE https://api.github.com/repos/o/r/contents/README.md`,
			),
			Entry(
				"httpie with a graphql mutation in a query item",
				`http POST https://api.github.com/graphql `+
					`query=='mutation { createCommitOnBranch(input: $i) { url } }' x=1`,
			),
			Entry(
				"httpie with only a query item and a body on stdin",
				"http api.github.com/repos/o/r/git/commits a==1 <<'EOF'\n{\"message\":\"x\"}\nEOF",
			),
			Entry(
				"httpie with an escaped separator",
				`http api.github.com/repos/o/r/git/commits 'message\==x'`,
			),
			Entry(
				"curl posting a graphql mutation",
				`curl -X POST https://api.github.com/graphql `+
					`-d '{"query":"mutation { createCommitOnBranch(input: $i) { url } }"}'`,
			),
			Entry(
				"an inline octokit script",
				`node -e 'octokit.rest.repos.createOrUpdateFileContents({owner, repo, path})'`,
			),
			Entry(
				"an inline octokit request call",
				`node -e 'await octokit.request("PUT /repos/{owner}/{repo}/contents/{path}", opts)'`,
			),
			Entry(
				"an inline octokit pull request merge",
				`node -e 'await octokit.rest.pulls.merge({owner, repo, pull_number: 42})'`,
			),
			Entry(
				"an inline request merging a pull request",
				`python3 -c 'requests.put("https://api.github.com/repos/o/r/pulls/42/merge", json=b)'`,
			),
			Entry(
				"an inline python request",
				`python3 -c 'requests.put("https://api.github.com/repos/o/r/contents/x", json=b)'`,
			),
			Entry(
				"curl with a token in the userinfo",
				`curl -X PUT https://x-access-token:$T@api.github.com/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl with an explicit port",
				`curl -X PUT https://api.github.com:443/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl with an upper-case host",
				`curl -X PUT https://API.GITHUB.COM/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl with an upper-case scheme",
				`curl -X PUT HTTPS://api.github.com/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl with no scheme at all",
				`curl -X PUT api.github.com/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl fetching a second URL",
				`curl -X PUT https://example.com/ok https://api.github.com/repos/o/r/contents/x`,
			),
			Entry(
				"curl with the call in a --next request",
				`curl https://example.com/ok --next -X PUT `+
					`https://api.github.com/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"curl whose second URL carries a query string",
				`curl -X PUT https://example.com/ok `+
					`"https://api.github.com/repos/o/r/contents/x?branch=main"`,
			),
		)

		DescribeTable(
			"passes the call",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))
				Expect(result.Passed).To(BeTrue())
			},
			Entry(
				"curl reading contents",
				`curl https://api.github.com/repos/o/r/contents/README.md`,
			),
			Entry(
				"curl creating a branch ref",
				`curl -X POST https://api.github.com/repos/o/r/git/refs -d '{"ref":"refs/heads/x"}'`,
			),
			Entry(
				"curl against a host that is not GitHub",
				`curl -X PUT https://example.com/repos/o/r/contents/README.md -d '{}'`,
			),
			Entry(
				"curl downloading a release asset",
				`curl -L -o out.tgz https://github.com/o/r/archive/refs/tags/v1.tar.gz`,
			),
			Entry(
				"curl reading GitHub then writing elsewhere",
				`curl https://api.github.com/repos/o/r/contents/x --next -X PUT https://example.com/ok -d '{}'`,
			),
			Entry(
				"an unrelated route in a script",
				`node -e 'app.put("/repos/:id/contents/:path", handler)'`,
			),
			Entry(
				"a script mentioning a blocked call without invoking it",
				`echo "use repos.createOrUpdateFileContents instead of git"`,
			),
			Entry(
				"a commit message describing a blocked call",
				`git commit -sS -m "docs: describe octokit.repos.merge() workaround"`,
			),
			Entry(
				"a PR comment describing a blocked call",
				`gh pr comment 1 --body "we should stop using octokit git.createCommit() here"`,
			),
			Entry(
				"an issue comment posted through gh api",
				`gh api repos/o/r/issues/1/comments -f body='use repos.merge() instead of the API'`,
			),
			Entry(
				"a commit message quoting a client call with a URL",
				`git commit -sS -m 'fix axios.put("https://api.github.com/repos/o/r/contents/x")'`,
			),
			Entry(
				"curl posting a harmless graphql query",
				`curl -X POST https://api.github.com/graphql -d '{"query":"query { viewer }"}'`,
			),
		)

		It("blocks a GitHub write whose path cannot be resolved", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`curl -X PUT "https://api.github.com/${ENDPOINT}" -d '{}'`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})

		It("blocks a graphql write whose body file cannot be read", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`curl -X POST https://api.github.com/graphql -d @mutation.json`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})

		It("leaves an unresolved write to another host alone", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`curl -X PUT "https://example.com/${ENDPOINT}" -d '{}'`,
			))
			Expect(result.Passed).To(BeTrue())
		})

		DescribeTable(
			"unwraps a command launcher",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))

				Expect(result.Passed).To(BeFalse())
				Expect(result.Reference.Code()).To(Equal("GH002"))
			},
			Entry(
				"sudo",
				`sudo gh api -X PUT repos/o/r/contents/x -f message=y`,
			),
			Entry(
				"sudo with its own flags",
				`sudo -u builder gh api -X PUT repos/o/r/contents/x -f message=y`,
			),
			Entry(
				"env with an assignment",
				`env GH_TOKEN=x gh api -X PUT repos/o/r/contents/x -f message=y`,
			),
			Entry(
				"timeout with a duration",
				`timeout 30 curl -X PUT https://api.github.com/repos/o/r/contents/x -d '{}'`,
			),
			Entry(
				"npx running an interpreter",
				`npx tsx -e 'octokit.repos.createOrUpdateFileContents({})'`,
			),
			Entry(
				"uv run running an interpreter",
				`uv run python -c 'requests.put("https://api.github.com/repos/o/r/contents/x")'`,
			),
		)

		DescribeTable(
			"leaves a launcher running something else alone",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))
				Expect(result.Passed).To(BeTrue())
			},
			Entry("a launcher with no known command", `sudo systemctl restart nginx`),
			Entry("a command that only prints one", `echo gh api -X PUT repos/o/r/contents/x`),
			Entry("a launcher running a read", `sudo gh api repos/o/r/contents/x`),
		)

		DescribeTable(
			"unwraps a shell interpreter's script",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))

				Expect(result.Passed).To(BeFalse())
				Expect(result.Reference.Code()).To(Equal("GH002"))
			},
			Entry(
				"bash -c running gh api",
				`bash -c 'gh api -X PUT repos/o/r/contents/x -f message=y'`,
			),
			Entry(
				"sh -c running curl",
				`sh -c "curl -X PUT https://api.github.com/repos/o/r/contents/x -d '{}'"`,
			),
			Entry(
				"a heredoc script on stdin",
				"bash <<'EOF'\ngh api -X PUT repos/o/r/contents/x -f message=y\nEOF",
			),
		)

		DescribeTable(
			"leaves a shell script that only mentions a call alone",
			func(command string) {
				result := apiValidator.Validate(ctx, bashContext(command))
				Expect(result.Passed).To(BeTrue())
			},
			Entry(
				"a commit message quoting a call",
				`bash -c 'git commit -sS -m "docs: describe octokit.repos.merge() workaround"'`,
			),
			Entry(
				"an echo quoting a call",
				`sh -c 'echo "use repos.merge() instead"'`,
			),
			Entry(
				"a PR comment quoting a call",
				`bash -c 'gh pr comment 1 --body "stop using git.createCommit() here"'`,
			),
		)

		It("names the tool that sends the request", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`curl -X PUT https://api.github.com/repos/o/r/contents/x -d '{}'`,
			))

			Expect(result.Message).To(HavePrefix("curl PUT repos/o/r/contents/x"))
		})

		It("leaves other clients alone when the check is turned off", func() {
			disabled := false
			cfg := &config.APIValidatorConfig{CheckHTTPClients: &disabled}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			viaCurl := apiValidator.Validate(ctx, bashContext(
				`curl -X PUT https://api.github.com/repos/o/r/contents/x -d '{}'`,
			))
			Expect(viaCurl.Passed).To(BeTrue())

			viaGH := apiValidator.Validate(ctx, bashContext(
				`gh api -X PUT repos/o/r/contents/x -f message=y`,
			))
			Expect(viaGH.Passed).To(BeFalse())
		})
	})

	Describe("endpoints built from variables", func() {
		It("resolves an assignment made on the same command line", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`EP=repos/o/r/contents/README.md; gh api -X PUT "$EP" -f message=x`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("resolves an assignment used as a command prefix", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`EP=repos/o/r/merges gh api "$EP" -f base=main -f head=topic`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("blocks a write whose endpoint cannot be resolved", func() {
			result := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X PUT "$ENDPOINT" -f message=x`),
			)

			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})

		It("allows a read whose endpoint cannot be resolved", func() {
			result := apiValidator.Validate(ctx, bashContext(`gh api "$ENDPOINT" --jq .sha`))
			Expect(result.Passed).To(BeTrue())
		})

		It("allows a resolved write to an endpoint that creates no commit", func() {
			result := apiValidator.Validate(ctx, bashContext(
				`EP=repos/o/r/git/refs; gh api -X POST "$EP" -f ref=refs/heads/topic`,
			))
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("GraphQL body in a file", func() {
		It("reads a file written earlier in the same command line", func() {
			result := apiValidator.Validate(ctx, bashContext(
				"cat > query.json <<'EOF'\n"+
					`{"query": "mutation { createCommitOnBranch(input: $i) { commit { url } } }"}`+
					"\nEOF\ngh api graphql --input query.json",
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("reads a file that exists on disk", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "query.json")
			Expect(os.WriteFile(
				path,
				[]byte(`{"query": "mutation { createCommitOnBranch(input: $i) { url } }"}`),
				0o600,
			)).To(Succeed())

			result := apiValidator.Validate(ctx, bashContext(`gh api graphql --input `+path))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("passes a file on disk carrying a harmless query", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "query.json")
			Expect(os.WriteFile(
				path,
				[]byte(`{"query": "query { viewer { login } }"}`),
				0o600,
			)).To(Succeed())

			result := apiValidator.Validate(ctx, bashContext(`gh api graphql --input `+path))
			Expect(result.Passed).To(BeTrue())
		})

		It("reads a query field pointed at a file with @", func() {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "q.graphql")
			Expect(os.WriteFile(
				path,
				[]byte("mutation { createCommitOnBranch(input: $i) { url } }"),
				0o600,
			)).To(Succeed())

			result := apiValidator.Validate(ctx, bashContext(`gh api graphql -F query=@`+path))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("does not read an endless character device", func() {
			done := make(chan *validatorpkg.Result, 1)

			go func() {
				defer GinkgoRecover()

				done <- apiValidator.Validate(
					ctx,
					bashContext(`gh api graphql --input /dev/zero`),
				)
			}()

			var result *validatorpkg.Result
			Eventually(done, "5s").Should(Receive(&result))
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})

		It("blocks a graphql write whose query text is not visible", func() {
			// Command substitution renders to nothing, so the query the call
			// will send cannot be inspected.
			result := apiValidator.Validate(ctx, bashContext(
				`gh api graphql -f query="$(cat createCommit.graphql)"`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})

		It("blocks when the file cannot be read", func() {
			result := apiValidator.Validate(ctx, bashContext(`gh api graphql --input missing.json`))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH003"))
		})
	})

	Describe("configuration", func() {
		It("allows unverifiable writes when the check is turned off", func() {
			disabled := false
			cfg := &config.APIValidatorConfig{BlockUnverifiableCalls: &disabled}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			opaque := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X PUT "$ENDPOINT" -f message=x`),
			)
			Expect(opaque.Passed).To(BeTrue())

			unreadable := apiValidator.Validate(
				ctx,
				bashContext(`gh api graphql --input missing.json`),
			)
			Expect(unreadable.Passed).To(BeTrue())
		})

		It("uses the configured endpoint rules instead of the defaults", func() {
			cfg := &config.APIValidatorConfig{
				BlockedEndpoints: []string{"POST **/git/refs"},
			}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			blocked := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X POST repos/o/r/git/refs -f ref=refs/heads/topic`),
			)
			Expect(blocked.Passed).To(BeFalse())

			allowed := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X PUT repos/o/r/contents/README.md -f message=x`),
			)
			Expect(allowed.Passed).To(BeTrue())
		})

		It("recognises the GitHub Enterprise Server graphql path", func() {
			// The client-call scan is narrowed to a name nothing uses, so this
			// proves the GraphQL path itself is recognised rather than the
			// mutation name merely being spotted in the command text.
			cfg := &config.APIValidatorConfig{BlockedClientCalls: []string{"nothing.matches.this"}}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			result := apiValidator.Validate(ctx, bashContext(
				`gh api https://ghe.example.com/api/graphql `+
					`-f query='mutation { createCommitOnBranch(input: $i) { url } }'`,
			))

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference.Code()).To(Equal("GH002"))
		})

		It("supports a wildcard method", func() {
			cfg := &config.APIValidatorConfig{
				BlockedEndpoints: []string{"* **/contents/**"},
			}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			result := apiValidator.Validate(ctx, bashContext(`gh api repos/o/r/contents/README.md`))
			Expect(result.Passed).To(BeFalse())
		})

		It("skips a rule with an invalid pattern instead of failing the hook", func() {
			cfg := &config.APIValidatorConfig{
				BlockedEndpoints: []string{"PUT [", "PUT **/contents/**"},
			}
			apiValidator = github.NewAPIValidator(cfg, logger.NewNoOpLogger(), nil)

			result := apiValidator.Validate(
				ctx,
				bashContext(`gh api -X PUT repos/o/r/contents/README.md`),
			)
			Expect(result.Passed).To(BeFalse())
		})
	})
})
