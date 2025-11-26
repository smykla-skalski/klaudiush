// Package main implements a sample klaudiush gRPC plugin server.
//
// This plugin validates git push operations and prevents force pushes
// without the safer --force-with-lease flag.
//
// Build:
//
//	go build -o git-validator main.go
//
// Run:
//
//	./git-validator
//	# or specify custom port:
//	./git-validator -port 50052
//
// Configure in ~/.klaudiush/config.toml:
//
//	[[plugins.plugins]]
//	name = "git-validator"
//	type = "grpc"
//	address = "localhost:50051"
//	timeout = "5s"
//
//	[plugins.plugins.predicate]
//	event_types = ["PreToolUse"]
//	tool_types = ["Bash"]
//	command_patterns = ["^git push"]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"

	"google.golang.org/grpc"

	pluginv1 "github.com/smykla-labs/klaudiush/api/plugin/v1"
)

const (
	defaultGRPCPort = 50051
)

var (
	port = flag.Int("port", defaultGRPCPort, "gRPC server port")

	// Regular expressions for validation
	forcePushRe      = regexp.MustCompile(`git\s+push.*--force\b`)
	forceWithLeaseRe = regexp.MustCompile(`git\s+push.*--force-with-lease\b`)
)

// server implements the ValidatorPlugin gRPC service.
type server struct {
	pluginv1.UnimplementedValidatorPluginServer
}

// Info returns plugin metadata.
func (*server) Info(_ context.Context, _ *pluginv1.InfoRequest) (*pluginv1.InfoResponse, error) {
	return &pluginv1.InfoResponse{
		Name:        "git-validator",
		Version:     "1.0.0",
		Description: "Validates git operations (prevents unsafe force pushes)",
		Author:      "klaudiush",
		Url:         "https://github.com/smykla-labs/klaudiush/examples/plugins/grpc-go",
	}, nil
}

// Validate performs validation of git commands.
func (s *server) Validate(
	_ context.Context,
	req *pluginv1.ValidateRequest,
) (*pluginv1.ValidateResponse, error) {
	// Only validate bash commands
	if req.ToolName != "Bash" {
		return &pluginv1.ValidateResponse{
			Passed:      true,
			ShouldBlock: false,
		}, nil
	}

	command := req.Command

	// Check for force push without --force-with-lease
	if forcePushRe.MatchString(command) && !forceWithLeaseRe.MatchString(command) {
		// Check if strict mode is enabled in config
		strictMode := req.Config["strict_mode"] == "true"

		if strictMode {
			// Block in strict mode
			return &pluginv1.ValidateResponse{
				Passed:      false,
				ShouldBlock: true,
				Message:     "Force push detected without --force-with-lease",
				ErrorCode:   "UNSAFE_FORCE_PUSH",
				FixHint:     "Use 'git push --force-with-lease' instead of '--force'",
				DocLink: "https://git-scm.com/docs/git-push#" +
					"Documentation/git-push.txt---force-with-leaseltrefnamegtltexpectgt",
				Details: map[string]string{
					"command": command,
					"mode":    "strict",
				},
			}, nil
		}

		// Warn in non-strict mode
		return &pluginv1.ValidateResponse{
			Passed:      false,
			ShouldBlock: false,
			Message:     "Force push detected - consider using --force-with-lease",
			ErrorCode:   "FORCE_PUSH_WARNING",
			FixHint:     "Use 'git push --force-with-lease' to avoid overwriting others' work",
			DocLink: "https://git-scm.com/docs/git-push#" +
				"Documentation/git-push.txt---force-with-leaseltrefnamegtltexpectgt",
			Details: map[string]string{
				"command": command,
				"mode":    "warn",
			},
		}, nil
	}

	// Check for push to protected branches
	if strings.Contains(command, "git push") {
		protectedBranches := s.getProtectedBranches(req.Config)
		for _, branch := range protectedBranches {
			if strings.Contains(command, branch) {
				return &pluginv1.ValidateResponse{
					Passed:      false,
					ShouldBlock: true,
					Message: fmt.Sprintf(
						"Direct push to protected branch '%s' is not allowed",
						branch,
					),
					ErrorCode: "PROTECTED_BRANCH",
					FixHint:   "Create a pull request instead of pushing directly",
					Details: map[string]string{
						"branch": branch,
					},
				}, nil
			}
		}
	}

	return &pluginv1.ValidateResponse{
		Passed:      true,
		ShouldBlock: false,
	}, nil
}

func (*server) getProtectedBranches(cfg map[string]string) []string {
	// Default protected branches
	defaults := []string{"main", "master"}

	// Check config for custom protected branches
	if branches, ok := cfg["protected_branches"]; ok {
		// Parse comma-separated list
		return strings.Split(branches, ",")
	}

	return defaults
}

func main() {
	flag.Parse()

	listenCfg := &net.ListenConfig{}

	lis, err := listenCfg.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pluginv1.RegisterValidatorPluginServer(s, &server{})

	log.Printf("gRPC plugin server listening on :%d", *port)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
