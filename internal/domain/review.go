package domain

import (
	"fmt"
	"strings"
)

// Voice scores the critic gives a draft against the voice rules.
const (
	VoiceScorePoor = 1 // Breaks the rules: questions, emoji, hype, talk about the blog
	VoiceScoreFair = 2 // Acceptable
	VoiceScoreGood = 3 // Plain, specific, dated
)

// ClaimVerdict is the critic's finding on one factual statement of a draft.
type ClaimVerdict struct {
	Claim     string // The statement, as the draft makes it
	Supported bool   // Whether the source states or directly implies it
	Evidence  string // The source passage that supports it, or why nothing does
}

// Review is the critic's verdict on a draft: every claim checked against
// the source, and a score against the voice rules.
type Review struct {
	Verdicts      []ClaimVerdict
	VoiceScore    int    // VoiceScorePoor to VoiceScoreGood; 0 when not scored
	VoiceProblems string // What breaks the voice rules, if anything
}

// Unsupported returns the claims the source does not back.
func (r Review) Unsupported() []ClaimVerdict {
	var unsupported []ClaimVerdict
	for _, verdict := range r.Verdicts {
		if !verdict.Supported {
			unsupported = append(unsupported, verdict)
		}
	}
	return unsupported
}

// Approved reports whether the draft may be posted: no unsupported claim,
// and a voice score above poor.
func (r Review) Approved() bool {
	return len(r.Unsupported()) == 0 && r.VoiceScore != VoiceScorePoor
}

// Feedback tells the writer what to fix, one line per problem.
func (r Review) Feedback() string {
	var lines []string
	for _, verdict := range r.Unsupported() {
		line := fmt.Sprintf("- Not supported by THE STORY: %q", verdict.Claim)
		if evidence := strings.TrimSpace(verdict.Evidence); evidence != "" {
			line += " (" + evidence + ")"
		}
		lines = append(lines, line)
	}
	if r.VoiceScore == VoiceScorePoor {
		problems := strings.TrimSpace(r.VoiceProblems)
		if problems == "" {
			problems = "the text breaks the voice rules"
		}
		lines = append(lines, "- Voice: "+problems)
	}
	return strings.Join(lines, "\n")
}
