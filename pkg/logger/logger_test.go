package logger_test

import (
	"bytes"
	"regexp"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func TestLogger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logger Suite")
}

var _ = Describe("SlogAdapter", func() {
	var (
		buf    *bytes.Buffer
		log    *logger.SlogAdapter
		output string
	)

	BeforeEach(func() {
		buf = &bytes.Buffer{}
	})

	AfterEach(func() {
		output = buf.String()
	})

	Describe("Timestamp format", func() {
		BeforeEach(func() {
			log = logger.NewFileLoggerWithWriter(buf, true, false)
		})

		It("should use local timezone in timestamps", func() {
			log.Info("test message")
			output = buf.String()

			// Timestamp should include timezone offset like +01:00 or -05:00
			// Format: 2006-01-02T15:04:05-07:00
			timestampRegex := regexp.MustCompile(
				`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}`,
			)
			Expect(timestampRegex.MatchString(output)).To(BeTrue(),
				"expected local timezone format, got: %s", output)
		})

		It("should not use UTC (Z suffix) in timestamps", func() {
			log.Info("test message")
			output = buf.String()

			// Should NOT end with Z (UTC marker)
			Expect(output).NotTo(MatchRegexp(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`),
				"timestamp should not use UTC (Z suffix), got: %s", output)
		})

		It("should use current local time", func() {
			log.Info("test message")
			output = buf.String()

			// Extract the timestamp part
			timestampRegex := regexp.MustCompile(
				`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2})`,
			)
			matches := timestampRegex.FindStringSubmatch(output)
			Expect(matches).To(HaveLen(2), "should match timestamp format")

			// Parse the timestamp
			logTime, err := time.Parse("2006-01-02T15:04:05-07:00", matches[1])
			Expect(err).ToNot(HaveOccurred())

			// Should be within 5 seconds of now (local time)
			now := time.Now()
			diff := now.Sub(logTime)
			Expect(diff).To(BeNumerically("<", 5*time.Second),
				"log timestamp should be within 5 seconds of now")
		})
	})

	Describe("Message logging", func() {
		Context("with debug mode enabled", func() {
			BeforeEach(func() {
				log = logger.NewFileLoggerWithWriter(buf, true, false)
			})

			It("should log Info messages", func() {
				log.Info("test info message")
				output = buf.String()

				Expect(output).To(ContainSubstring("INFO"))
				Expect(output).To(ContainSubstring("test info message"))
			})

			It("should log Error messages", func() {
				log.Error("test error message")
				output = buf.String()

				Expect(output).To(ContainSubstring("ERROR"))
				Expect(output).To(ContainSubstring("test error message"))
			})

			It("should not log Debug messages without trace mode", func() {
				log.Debug("test debug message")
				output = buf.String()

				Expect(output).To(BeEmpty())
			})
		})

		Context("with trace mode enabled", func() {
			BeforeEach(func() {
				log = logger.NewFileLoggerWithWriter(buf, true, true)
			})

			It("should log Debug messages", func() {
				log.Debug("test debug message")
				output = buf.String()

				Expect(output).To(ContainSubstring("DEBUG"))
				Expect(output).To(ContainSubstring("test debug message"))
			})
		})

		Context("without debug mode", func() {
			BeforeEach(func() {
				log = logger.NewFileLoggerWithWriter(buf, false, false)
			})

			It("should not log Info messages", func() {
				log.Info("test info message")
				output = buf.String()

				Expect(output).To(BeEmpty())
			})

			It("should still log Error messages", func() {
				log.Error("test error message")
				output = buf.String()

				Expect(output).To(ContainSubstring("ERROR"))
			})
		})
	})

	Describe("Key-value pairs", func() {
		BeforeEach(func() {
			log = logger.NewFileLoggerWithWriter(buf, true, false)
		})

		It("should log key-value pairs", func() {
			log.Info("test message", "key1", "value1", "key2", 42)
			output = buf.String()

			Expect(output).To(ContainSubstring("key1=value1"))
			Expect(output).To(ContainSubstring("key2=42"))
		})

		It("should quote values with spaces", func() {
			log.Info("test message", "command", "echo hello world")
			output = buf.String()

			Expect(output).To(ContainSubstring(`command="echo hello world"`))
		})

		It("should escape quotes in values", func() {
			log.Info("test message", "msg", `say "hello"`)
			output = buf.String()

			Expect(output).To(ContainSubstring(`msg="say \"hello\""`))
		})

		It("should escape newlines in values", func() {
			log.Info("test message", "text", "line1\nline2")
			output = buf.String()

			Expect(output).To(ContainSubstring(`text="line1\nline2"`))
		})

		It("should not truncate long values", func() {
			longCommand := "git -C /Users/bart.smykla@konghq.com/Projects/github.com/smykla-skalski/klaudiush add pkg/mdtable/parser.go pkg/mdtable/parser_test.go && git -C /Users/bart.smykla@konghq.com/Projects/github.com/smykla-skalski/klaudiush commit -sS -m \"fix(mdtable): prevent false positives in spacing detection\""

			log.Info("context parsed", "command", longCommand)
			output = buf.String()

			// The full command should be present, not truncated
			Expect(output).To(ContainSubstring("fix(mdtable): prevent false positives"))
			Expect(output).To(ContainSubstring("pkg/mdtable/parser.go"))
			Expect(output).NotTo(ContainSubstring("..."))
		})
	})

	Describe("With method", func() {
		BeforeEach(func() {
			log = logger.NewFileLoggerWithWriter(buf, true, false)
		})

		It("should create logger with base key-value pairs", func() {
			childLog := log.With("baseKey", "baseValue")
			childLog.Info("test message", "msgKey", "msgValue")
			output = buf.String()

			Expect(output).To(ContainSubstring("baseKey=baseValue"))
			Expect(output).To(ContainSubstring("msgKey=msgValue"))
		})

		It("should not affect parent logger", func() {
			childLog := log.With("childKey", "childValue")
			log.Info("parent message")
			childLog.Info("child message")
			output = buf.String()

			// Parent log should not have childKey
			lines := bytes.Split(buf.Bytes(), []byte("\n"))
			Expect(string(lines[0])).NotTo(ContainSubstring("childKey"))
			Expect(string(lines[1])).To(ContainSubstring("childKey"))
		})
	})
})

var _ = Describe("NoOpLogger", func() {
	var log *logger.NoOpLogger

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
	})

	It("should not panic on Debug", func() {
		Expect(func() { log.Debug("test") }).NotTo(Panic())
	})

	It("should not panic on Info", func() {
		Expect(func() { log.Info("test") }).NotTo(Panic())
	})

	It("should not panic on Error", func() {
		Expect(func() { log.Error("test") }).NotTo(Panic())
	})

	It("should return itself from With", func() {
		child := log.With("key", "value")
		Expect(child).To(Equal(log))
	})
})
