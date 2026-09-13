package watch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// NarrateOptions configures the LLM narrator. The narrator is intentionally
// constrained: it may only restate facts already present in the deterministic
// report and cannot introduce new elements or relationships.
type NarrateOptions struct {
	Report   ImpactReport
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
}

const impactNarratorSystemPrompt = `You are an architecture impact narrator for a software diagram tool.
You will receive a deterministic architecture impact report.
Summarize it in 2-4 sentences for a pull request reviewer.
Rules:
- Only describe elements, relationships, and findings that appear in the report.
- Never invent components, dependencies, or code locations.
- Clearly label inferred suggestions as suggestions.
- Do not propose editing the architecture; findings are for human review.`

// NarrateImpactReport produces a human-readable summary of the report. When no
// model is configured it falls back to a deterministic template so the command
// works fully offline.
func NarrateImpactReport(ctx context.Context, opts NarrateOptions) (string, error) {
	if strings.TrimSpace(opts.Model) == "" || strings.TrimSpace(opts.Endpoint) == "" {
		return deterministicNarration(opts.Report), nil
	}
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		apiKey = DefaultOpenAIAPIKey
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	client := openai.NewClient(
		option.WithBaseURL(openAIBaseURL(opts.Endpoint)),
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
	)
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(opts.Model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(impactNarratorSystemPrompt),
			openai.UserMessage(impactNarrationPrompt(opts.Report)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("narrate impact report: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("narrate impact report: empty response")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func impactNarrationPrompt(report ImpactReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Base: %s\nHead: %s\n\n", report.Base, report.Head)
	b.WriteString("Changed elements:\n")
	if len(report.Changed) == 0 {
		b.WriteString("- none\n")
	}
	for _, element := range report.Changed {
		fmt.Fprintf(&b, "- %s (kind: %s)\n", element.Name, element.Kind)
	}
	b.WriteString("\nCandidate elements (weak name matches):\n")
	if len(report.Candidates) == 0 {
		b.WriteString("- none\n")
	}
	for _, element := range report.Candidates {
		fmt.Fprintf(&b, "- %s\n", element.Name)
	}
	b.WriteString("\nRelated elements:\n")
	if len(report.Related) == 0 {
		b.WriteString("- none\n")
	}
	for _, element := range report.Related {
		fmt.Fprintf(&b, "- %s\n", element.Name)
	}
	b.WriteString("\nUnmapped files:\n")
	if len(report.Unmapped) == 0 {
		b.WriteString("- none\n")
	}
	for _, file := range report.Unmapped {
		fmt.Fprintf(&b, "- %s\n", file)
	}
	b.WriteString("\nFindings:\n")
	if len(report.Findings) == 0 {
		b.WriteString("- none\n")
	}
	for _, finding := range report.Findings {
		observed := "observed"
		if !finding.Observed {
			observed = "inferred"
		}
		fmt.Fprintf(&b, "- [%s] %s (%s)\n", finding.Type, finding.Message, observed)
	}
	return b.String()
}

func deterministicNarration(report ImpactReport) string {
	var parts []string
	if len(report.Changed) > 0 {
		names := make([]string, 0, len(report.Changed))
		for _, element := range report.Changed {
			names = append(names, element.Name)
		}
		parts = append(parts, "This change affects "+strings.Join(names, ", ")+".")
	} else if len(report.Candidates) > 0 {
		names := make([]string, 0, len(report.Candidates))
		for _, element := range report.Candidates {
			names = append(names, element.Name)
		}
		parts = append(parts, fmt.Sprintf("No element is strongly bound to this change; %d weak candidate(s): %s.", len(report.Candidates), strings.Join(names, ", ")))
	} else {
		parts = append(parts, "This change does not affect any architecture-bound element.")
	}
	if len(report.Related) > 0 {
		names := make([]string, 0, len(report.Related))
		for _, element := range report.Related {
			names = append(names, element.Name)
		}
		parts = append(parts, "Related architecture context: "+strings.Join(names, ", ")+".")
	}
	if len(report.Unmapped) > 0 {
		parts = append(parts, fmt.Sprintf("%d changed file(s) are not bound to any architecture element.", len(report.Unmapped)))
	}
	newRelationships := 0
	for _, finding := range report.Findings {
		if finding.Type == "possible_new_relationship" {
			newRelationships++
		}
	}
	if newRelationships > 0 {
		parts = append(parts, fmt.Sprintf("%d possible new relationship(s) were observed in code.", newRelationships))
	}
	return strings.Join(parts, " ")
}
