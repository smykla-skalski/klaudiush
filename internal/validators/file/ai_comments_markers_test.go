package file_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const rustTestModule = `#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_empty_input() {
        assert!(parse("").is_empty());
    }

    #[cfg(unix)]
    #[test]
    fn reads_socket() {}
}
`

var _ = Describe("AICommentValidator in languages where # is code", func() {
	var (
		strict *file.AICommentValidator
		filler *file.AICommentValidator
	)

	BeforeEach(func() {
		strict = file.NewAICommentValidator(
			logger.NewNoOpLogger(),
			&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
			nil,
		)
		filler = file.NewAICommentValidator(logger.NewNoOpLogger(), nil, nil)
	})

	writeCtx := func(path, content string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{FilePath: path, Content: content},
		}
	}

	DescribeTable(
		"strict mode does not report # syntax as a comment",
		func(path, content string) {
			result := strict.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeTrue(), result.Message)
		},
		Entry("rust outer attributes", "/repo/src/lib.rs", rustTestModule),
		Entry("rust inner attributes", "/repo/src/lib.rs",
			"#![forbid(unsafe_code)]\n#![cfg_attr(docsrs, feature(doc_cfg))]\nfn main() {}"),
		Entry("rust multi-line attribute", "/repo/src/lib.rs",
			"#[cfg(all(\n    unix,\n    not(target_os = \"macos\"),\n))]\nfn probe() {}"),
		Entry("rust attributes followed by code on the same line", "/repo/src/lib.rs",
			"#[inline] fn id(x: u8) -> u8 { x }\n    #[derive(Debug)] struct Unit;"),
		Entry("rust attribute holding a // string", "/repo/src/lib.rs",
			"#[doc = \"see https://example.com // docs\"]\nfn f() {}"),
		Entry("c preprocessor directives", "/repo/src/main.c",
			"#include <stdio.h>\n#define MAX_ITEMS 10\n#ifdef DEBUG\n  #undef MAX_ITEMS\n#endif"),
		Entry("c header guard", "/repo/include/api.h",
			"#pragma once\n#ifndef API_H\n#define API_H\n#endif"),
		Entry(
			"c++ preprocessor directives",
			"/repo/src/main.cpp",
			"#include <vector>\n#if defined(_WIN32)\n#  define EXPORT __declspec(dllexport)\n#endif",
		),
		Entry("objective-c directives", "/repo/App/View.m",
			"#import <UIKit/UIKit.h>\n#pragma mark - Lifecycle"),
		Entry("c# directives", "/repo/src/Program.cs",
			"#nullable enable\n#region Helpers\nstatic int Two() => 2;\n#endregion"),
		Entry("swift compiler directives", "/repo/Sources/App.swift",
			"#if DEBUG\nlet level = 1\n#else\nlet level = 0\n#endif"),
		Entry("typescript private members", "/repo/src/counter.ts",
			"class Counter {\n  #count = 0;\n  #set(n: number) { this.#count = n; }\n}"),
		Entry("javascript shebang and private field", "/repo/bin/cli.mjs",
			"#!/usr/bin/env node\nclass A {\n  #ready = false;\n}"),
		Entry("css id selector and hex colour", "/repo/src/app.css",
			"#main {\n  color: #fff;\n}"),
		Entry("vue slot shorthand", "/repo/src/App.vue",
			"<template>\n  <Card #header>Title</Card>\n</template>"),
		Entry("glsl version and define", "/repo/shaders/blur.frag",
			"#version 330 core\n#define TAPS 9"),
		Entry("arduino include", "/repo/sketch/sketch.ino", "#include <Arduino.h>"),
		Entry("f# compiler directive", "/repo/src/App.fs",
			"#if DEBUG\nlet level = 1\n#endif"),
		Entry("c# script directives", "/repo/build.csx",
			"#r \"nuget: Newtonsoft.Json, 13.0.3\"\n#load \"common.csx\""),
	)

	DescribeTable(
		"strict mode keeps allowing doc comments",
		func(path, content string) {
			result := strict.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeTrue(), result.Message)
		},
		Entry("rust outer doc comment above an attribute", "/repo/src/lib.rs",
			"/// Widget models a thing.\n#[derive(Debug)]\npub struct Widget;"),
		Entry("rust inner doc comment", "/repo/src/lib.rs",
			"//! Parsing helpers.\n#![deny(missing_docs)]"),
	)

	DescribeTable(
		"strict mode still reports real comments and only them",
		func(path, content, reported string) {
			result := strict.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(HaveSuffix("\n\n" + reported))
		},
		Entry("rust line comment", "/repo/src/lib.rs",
			"fn f() {}\n// holds the running total", "Line 2: // holds the running total"),
		Entry("rust comment after an attribute", "/repo/src/lib.rs",
			"#[test] // checks the empty case\nfn empty() {}", "Line 1: // checks the empty case"),
		Entry(
			"rust comment after an attribute above a const",
			"/repo/src/lib.rs",
			"#[must_use] // Limit caps retries\nconst LIMIT: u8 = 3;",
			"Line 1: // Limit caps retries",
		),
		Entry("c comment after a directive", "/repo/src/main.c",
			"#define MAX 10 // holds the cap", "Line 1: // holds the cap"),
		Entry("typescript comment after a private field", "/repo/src/counter.ts",
			"class C {\n  #count = 0; // holds the total\n}", "Line 2: // holds the total"),
		Entry("python hash comment", "/repo/app.py",
			"x = 1\n# holds the total", "Line 2: # holds the total"),
		Entry("ruby hash comment", "/repo/app.rb",
			"x = 1\n# holds the total", "Line 2: # holds the total"),
		Entry("hash comment in a file of unknown language", "/repo/app.unknown",
			"x = 1\n# holds the total", "Line 2: # holds the total"),
	)

	DescribeTable(
		"filler mode does not read # syntax as a filler comment",
		func(path, content string) {
			result := filler.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeTrue(), result.Message)
		},
		Entry("c define", "/repo/src/main.c", "#define SET_LIMIT 10"),
		Entry("c define with a why comment", "/repo/src/main.c",
			"#define CAP 10 // matches the kernel limit"),
		Entry("typescript private members named after verbs", "/repo/src/counter.ts",
			"class C {\n  #render() {}\n  #count = 0;\n}"),
	)

	DescribeTable(
		"filler mode still reports filler comments and only them",
		func(path, content, reported string) {
			result := filler.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(HaveSuffix("\n\n" + reported))
		},
		Entry("rust filler line comment", "/repo/src/lib.rs",
			"#[test]\nfn empty() {\n    // Set the value\n    let v = 1;\n}", "Line 3: // Set"),
		Entry("c filler comment after a directive", "/repo/src/main.c",
			"#define CAP 10 // Set the cap", "Line 1: // Set"),
		Entry("shell filler hash comment", "/repo/run.sh",
			"# Set the value\nX=1", "Line 1: # Set"),
	)

	DescribeTable(
		"filler mode custom patterns match the line but only report the comment",
		func(pattern, path, content string, passes bool) {
			custom := file.NewAICommentValidator(
				logger.NewNoOpLogger(),
				&config.AICommentValidatorConfig{Patterns: []string{pattern}},
				nil,
			)
			result := custom.Validate(context.Background(), writeCtx(path, content))
			Expect(result.Passed).To(Equal(passes), result.Message)
		},
		Entry("pattern needing whitespace before the marker",
			`(?i)\s//\s*set\b`, "/repo/main.go", "x := 1 // set x", false),
		Entry("pattern anchored at line start",
			`(?i)^\s+//\s*set\b`, "/repo/main.go", "\t// set x", false),
		Entry("pattern matching only code before the comment",
			`(?i)#define`, "/repo/src/main.c", "#define CAP 10 // matches the kernel limit", true),
	)

	It("allows an Edit that adds a Rust test module", func() {
		path := filepath.Join(GinkgoT().TempDir(), "lib.rs")
		Expect(
			os.WriteFile(
				path,
				[]byte("pub fn parse(s: &str) -> Vec<u8> {\n    s.into()\n}\n"),
				0o600,
			),
		).
			To(Succeed())

		ctx := &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeEdit,
			ToolInput: hook.ToolInput{
				FilePath:  path,
				OldString: "    s.into()\n}\n",
				NewString: "    s.into()\n}\n\n" + rustTestModule,
			},
		}

		result := strict.Validate(context.Background(), ctx)
		Expect(result.Passed).To(BeTrue(), result.Message)
	})
})
