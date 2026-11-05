package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/board-agents/internal/ai"
)

// riskPrompt is step 2's three classes, as git-board's page words them.
const riskPrompt = `You classify a change to software by how much it can break. The classes:
low: a copy, a style, a config value.
normal: behaviour changes in one area.
structural: data, a schema, a public interface.
Answer with one word on the first line: low, normal or structural. Then one sentence why.`

// Suggestion is the AI's class for an issue, and why.
type Suggestion struct {
	Issue   string `json:"issue"`
	Class   string `json:"class"` // low, normal, structural
	Why     string `json:"why"`
	Model   string `json:"model"`
	Comment bool   `json:"comment"` // written on the issue
	// Note says what the comment lacks: the board records no cycle step
	// for it (board.NoStep)
	Note string `json:"note,omitempty"`
}

// SuggestRisk asks the AI for the issue's class, with its title and
// description only (what leaves the machine, dev-cycle.md), and with
// comment writes the answer as a comment in the cycle's risk step. The
// person sets the risk: what the AI writes is a comment, never a
// decision.
func (s *Service) SuggestRisk(ctx context.Context, c *ai.Client, ref string, comment bool) (Suggestion, error) {
	if c == nil {
		return Suggestion{}, errors.New("AI is off: set LLM_BASE_URL and LLM_MODEL (or configure opencode), and LLM_API_KEY")
	}
	th, err := s.Board.Thread(ctx, ref)
	if err != nil {
		return Suggestion{}, err
	}
	// the comment goes in step 2's discussion: refused past it, when the
	// board says where the issue is
	version := ""
	if cyc, err := s.Board.Cycle(ctx, th.ID); comment && err == nil {
		if cyc.Step != 2 || cyc.Done {
			return Suggestion{}, refuse("not-at-risk", "not at Risk: the issue is at step "+fmt.Sprint(cyc.Step))
		}
		version = cyc.Version // written only if the issue is still where the check saw it
	}
	answer, err := c.Chat(ctx, riskPrompt, "# "+th.Title+"\n\n"+th.Description())
	if err != nil {
		return Suggestion{}, err
	}
	class, why := riskOf(answer)
	if class == "" {
		return Suggestion{}, fmt.Errorf("%s answered no class: %q", c.Model, answer[:min(len(answer), 200)])
	}
	out := Suggestion{Issue: th.Name(), Class: class, Why: why, Model: c.Model}
	if comment {
		// ponytail: v1 wrote ai:model and ai:risk as the comment's
		// metadata, which no API route takes from a client: the model goes
		// in the text, and the web page doesn't show it as a suggestion
		body := strings.TrimSpace(fmt.Sprintf("Suggested: **%s** (%s)\n\n%s", class, c.Model, why))
		w, err := s.Board.Comment(ctx, th.ID, body, "risk", version)
		if err != nil {
			return out, err
		}
		out.Comment, out.Note = true, w.Note
	}
	return out, nil
}

// riskOf is the class an answer opens with, and the rest: its reason.
// "" when the first word is no class.
func riskOf(answer string) (class, why string) {
	answer = strings.TrimSpace(answer)
	end := strings.IndexFunc(answer, func(r rune) bool { return r == ' ' || r == '\n' || r == '.' || r == ':' || r == ',' })
	if end < 0 {
		end = len(answer)
	}
	word := strings.ToLower(strings.Trim(answer[:end], "*_`#\"' "))
	if !slices.Contains([]string{"low", "normal", "structural"}, word) {
		return "", ""
	}
	return word, strings.TrimSpace(strings.TrimLeft(answer[end:], ".:,"))
}
