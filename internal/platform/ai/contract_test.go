package ai

import "testing"

func TestPurposeValid(t *testing.T) {
	for _, p := range []Purpose{PurposeAgentStep, PurposeContextSummary, PurposeMemoryDerivation} {
		if !p.Valid() {
			t.Fatalf("purpose %q should be valid", p)
		}
	}
	if (Purpose("chat")).Valid() {
		t.Fatal("unknown purpose should be invalid")
	}
	if (Purpose("")).Valid() {
		t.Fatal("empty purpose should be invalid")
	}
}

func TestBlindRetryForbidden(t *testing.T) {
	for _, code := range []string{ErrProviderQuotaExhausted, ErrProviderAuthFailed, ErrProviderRequestInvalid, ErrProviderCapabilityUnsupported} {
		if !BlindRetryForbidden(code) {
			t.Fatalf("code %q should forbid blind retry", code)
		}
	}
	for _, code := range []string{ErrProviderTimeout, ErrProviderRateLimited, ErrProviderUnavailable, ErrProviderContextTooLong, ErrProviderOutputInvalid} {
		if BlindRetryForbidden(code) {
			t.Fatalf("code %q should allow retry", code)
		}
	}
}

func TestCapabilitiesSupports(t *testing.T) {
	capabilities := Capabilities{
		TextInput:        CapabilityAvailable,
		ImageInput:       CapabilityVerified,
		ToolCalling:      CapabilityUnsupported,
		StructuredOutput: CapabilityMisconfigured,
	}
	if !capabilities.Supports(CapabilityTextInput) || !capabilities.Supports(CapabilityImageInput) {
		t.Fatal("available/verified dimensions should be supported")
	}
	if capabilities.Supports(CapabilityToolCalling) || capabilities.Supports(CapabilityStructuredOutput) {
		t.Fatal("unsupported/misconfigured dimensions should not be supported")
	}
	if capabilities.Supports(CapabilityCancellation) {
		t.Fatal("zero-value (unset) dimension should not be supported")
	}
}

func TestProfileOmitOptional(t *testing.T) {
	profile := Profile{}
	required := Parameter{Name: "max_output_tokens", Value: 1200, Required: true}
	optional := Parameter{Name: "temperature", Value: 0.7, Required: false}
	if _, ok := profile.OmitOptional(required, "unsupported", "v1", nil); ok {
		t.Fatal("required parameter must not be omittable")
	}
	omission, ok := profile.OmitOptional(optional, "unsupported", "v1", []string{"max_output_tokens"})
	if !ok {
		t.Fatal("optional parameter should be omittable")
	}
	if omission.Name != "temperature" || omission.Reason != "unsupported" || omission.Evidence != "v1" {
		t.Fatalf("omission = %+v", omission)
	}
	if len(omission.Sent) != 1 || omission.Sent[0] != "max_output_tokens" {
		t.Fatalf("omission sent = %v", omission.Sent)
	}
}

func TestUsageNormalizeUnknownNotZero(t *testing.T) {
	unknown := Usage{}.Normalize()
	if unknown.SettlementStatus != SettlementUnknown {
		t.Fatalf("missing usage should be unknown, got %s", unknown.SettlementStatus)
	}
	if unknown.Known() {
		t.Fatal("missing usage must not be Known")
	}
	reported := Usage{InputTokens: 10, OutputTokens: 5, Complete: true}.Normalize()
	if reported.TotalTokens != 15 {
		t.Fatalf("total should derive 10+5=15, got %d", reported.TotalTokens)
	}
	if reported.SettlementStatus != SettlementUnsettled {
		t.Fatalf("reported usage should be unsettled, got %s", reported.SettlementStatus)
	}
	if !reported.Known() {
		t.Fatal("reported usage should be Known")
	}
}

func TestSelectMode(t *testing.T) {
	if SelectMode(true, true) != StreamModeNonStreaming {
		t.Fatal("nonStreamOnly should select non-streaming")
	}
	if SelectMode(false, false) != StreamModeNonStreaming {
		t.Fatal("unverified streaming should select non-streaming")
	}
	if SelectMode(false, true) != StreamModeStreaming {
		t.Fatal("verified streaming should select streaming")
	}
}

func TestCanDegradeToNonStreaming(t *testing.T) {
	if d := CanDegradeToNonStreaming(true, true, true); d.Allow {
		t.Fatal("body published must forbid degrade")
	}
	if d := CanDegradeToNonStreaming(false, false, true); d.Allow {
		t.Fatal("not recoverable must forbid degrade")
	}
	if d := CanDegradeToNonStreaming(false, true, false); d.Allow {
		t.Fatal("no budget must forbid degrade")
	}
	if d := CanDegradeToNonStreaming(false, true, true); !d.Allow {
		t.Fatalf("should allow degrade, got %+v", d)
	}
}
