package config_test

import (
	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Tests are run as part of Config Rules Suite from rules_test.go.

var _ = Describe("Severity", func() {
	Describe("ShouldBlock", func() {
		It("should return true for SeverityError", func() {
			Expect(config.SeverityError.ShouldBlock()).To(BeTrue())
		})

		It("should return false for SeverityWarning", func() {
			Expect(config.SeverityWarning.ShouldBlock()).To(BeFalse())
		})

		It("should return false for SeverityUnknown", func() {
			Expect(config.SeverityUnknown.ShouldBlock()).To(BeFalse())
		})
	})

	Describe("ParseSeverity", func() {
		It("should parse 'error' correctly", func() {
			severity, err := config.ParseSeverity("error")
			Expect(err).NotTo(HaveOccurred())
			Expect(severity).To(Equal(config.SeverityError))
		})

		It("should parse 'warning' correctly", func() {
			severity, err := config.ParseSeverity("warning")
			Expect(err).NotTo(HaveOccurred())
			Expect(severity).To(Equal(config.SeverityWarning))
		})

		It("should return error for invalid severity", func() {
			severity, err := config.ParseSeverity("invalid")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidSeverity)).To(BeTrue())
			Expect(severity).To(Equal(config.SeverityUnknown))
		})

		It("should return error for empty string", func() {
			severity, err := config.ParseSeverity("")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrInvalidSeverity)).To(BeTrue())
			Expect(severity).To(Equal(config.SeverityUnknown))
		})
	})
})

var _ = Describe("Duration", func() {
	Describe("UnmarshalText", func() {
		It("should parse valid duration strings", func() {
			var d config.Duration
			err := d.UnmarshalText([]byte("10s"))
			Expect(err).NotTo(HaveOccurred())
			Expect(d.String()).To(Equal("10s"))
		})

		It("should parse duration with multiple units", func() {
			var d config.Duration
			err := d.UnmarshalText([]byte("1h30m"))
			Expect(err).NotTo(HaveOccurred())
			Expect(d.String()).To(Equal("1h30m0s"))
		})

		It("should return error for invalid duration format", func() {
			var d config.Duration
			err := d.UnmarshalText([]byte("invalid"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid duration"))
		})

		It("should return error for negative duration", func() {
			var d config.Duration
			err := d.UnmarshalText([]byte("-5s"))
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, config.ErrNegativeDuration)).To(BeTrue())
		})

		It("should accept zero duration", func() {
			var d config.Duration
			err := d.UnmarshalText([]byte("0s"))
			Expect(err).NotTo(HaveOccurred())
			Expect(d.String()).To(Equal("0s"))
		})
	})

	Describe("MarshalText", func() {
		It("should marshal duration to text", func() {
			var d config.Duration
			_ = d.UnmarshalText([]byte("5m"))
			text, err := d.MarshalText()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(text)).To(Equal("5m0s"))
		})

		It("should marshal zero duration", func() {
			var d config.Duration
			text, err := d.MarshalText()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(text)).To(Equal("0s"))
		})
	})

	Describe("String", func() {
		It("should return string representation", func() {
			var d config.Duration
			_ = d.UnmarshalText([]byte("2h"))
			Expect(d.String()).To(Equal("2h0m0s"))
		})
	})

	Describe("ToDuration", func() {
		It("should convert to time.Duration", func() {
			var d config.Duration
			_ = d.UnmarshalText([]byte("30s"))
			Expect(d.ToDuration().Seconds()).To(Equal(float64(30)))
		})
	})
})
