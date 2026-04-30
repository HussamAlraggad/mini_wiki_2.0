// Package srs implements the 5-stage SRS generation pipeline using local LLMs.
// Stages: FR/NFR Extraction -> MoSCoW Prioritization -> DFD Generation ->
//         CSPEC Logic -> SRS Formatting (IEEE 830)
package srs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// Stage identifies a pipeline stage.
type Stage int

const (
	StageExtraction     Stage = iota + 1 // FR/NFR extraction
	StageMoSCoW                          // MoSCoW prioritization
	StageDFD                             // DFD generation
	StageCSPEC                           // CSPEC logic
	StageSRS                             // SRS formatting
)

func (s Stage) String() string {
	switch s {
	case StageExtraction:
		return "1/5 FR/NFR Extraction"
	case StageMoSCoW:
		return "2/5 MoSCoW Prioritization"
	case StageDFD:
		return "3/5 DFD Generation"
	case StageCSPEC:
		return "4/5 CSPEC Logic"
	case StageSRS:
		return "5/5 SRS Formatting"
	default:
		return "Unknown"
	}
}

// ProgressUpdate is sent via channel during pipeline execution.
type ProgressUpdate struct {
	Stage   Stage  `json:"stage"`
	Message string `json:"message"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

// RunID is a unique identifier for a pipeline run.
type RunID string

// PipelineConfig controls the SRS pipeline.
type PipelineConfig struct {
	ProjectName string // e.g., "Research System v1.0"
	Version     string // e.g., "1.0"
	ProjectCtx  string // Project context description for MoSCoW
	Model       string // LLM model to use
}

// StageResult holds the output of a single pipeline stage.
type StageResult struct {
	Stage   Stage
	Input   string // data sent to LLM
	Output  string // raw LLM response (JSON)
	Parsed  interface{} // parsed structured data
	Error   error
}

// LLMClient is the interface the pipeline needs to call the LLM.
type LLMClient interface {
	// Generate sends a prompt and returns the full text response.
	Generate(ctx context.Context, model, prompt string) (string, error)
}

// Pipeline runs the 5-stage SRS generation pipeline.
type Pipeline struct {
	client LLMClient
	cfg    PipelineConfig
}

// New creates a new SRS pipeline.
func New(client LLMClient, cfg PipelineConfig) *Pipeline {
	return &Pipeline{client: client, cfg: cfg}
}

// Results holds all outputs from a pipeline run.
type Results struct {
	RunID        RunID
	Requirements string // JSON: FR/NFR extraction output
	MoSCoW       string // JSON: MoSCoW output
	DFD          string // JSON: DFD output
	CSPEC        string // JSON: CSPEC output
	SRS          string // Markdown: final SRS document
	StartedAt    time.Time
	CompletedAt  time.Time
	StageResults []StageResult
}

// Run executes all 5 stages sequentially, sending progress updates.
func (p *Pipeline) Run(ctx context.Context, data string, progress chan<- ProgressUpdate) (*Results, error) {
	runID := RunID(fmt.Sprintf("SRS-%d", time.Now().Unix()))
	results := &Results{
		RunID:       runID,
		StartedAt:   time.Now(),
		StageResults: make([]StageResult, 0, 5),
	}

	// Stage 1: FR/NFR Extraction
	sendProgress(progress, StageExtraction, "Extracting functional and non-functional requirements...", false)
	req1, resp1, err1 := p.runStage(ctx, StageExtraction, tmplFRNFRExtraction, map[string]string{
		"Data": data,
	})
	results.StageResults = append(results.StageResults, StageResult{
		Stage: StageExtraction, Input: req1, Output: resp1, Error: err1,
	})
	if err1 != nil {
		sendProgress(progress, StageExtraction, "", true)
		return nil, fmt.Errorf("stage 1 failed: %w", err1)
	}
	results.Requirements = extractJSON(resp1)
	sendProgress(progress, StageExtraction, "Requirements extracted", true)

	// Stage 2: MoSCoW Prioritization
	sendProgress(progress, StageMoSCoW, "Prioritizing requirements using MoSCoW...", false)
	projectCtx := p.cfg.ProjectCtx
	if projectCtx == "" {
		projectCtx = "A software system being developed based on user feedback and requirements analysis."
	}
	req2, resp2, err2 := p.runStage(ctx, StageMoSCoW, tmplMoSCoW, map[string]string{
		"RequirementsJSON": results.Requirements,
		"ProjectContext":   projectCtx,
	})
	results.StageResults = append(results.StageResults, StageResult{
		Stage: StageMoSCoW, Input: req2, Output: resp2, Error: err2,
	})
	if err2 != nil {
		sendProgress(progress, StageMoSCoW, "", true)
		return nil, fmt.Errorf("stage 2 failed: %w", err2)
	}
	results.MoSCoW = extractJSON(resp2)
	sendProgress(progress, StageMoSCoW, "MoSCoW prioritization complete", true)

	// Stage 3: DFD Generation
	sendProgress(progress, StageDFD, "Generating Data Flow Diagram components...", false)
	req3, resp3, err3 := p.runStage(ctx, StageDFD, tmplDFD, map[string]string{
		"RequirementsJSON": results.Requirements,
	})
	results.StageResults = append(results.StageResults, StageResult{
		Stage: StageDFD, Input: req3, Output: resp3, Error: err3,
	})
	if err3 != nil {
		sendProgress(progress, StageDFD, "", true)
		return nil, fmt.Errorf("stage 3 failed: %w", err3)
	}
	results.DFD = extractJSON(resp3)
	sendProgress(progress, StageDFD, "DFD components generated", true)

	// Stage 4: CSPEC Logic
	sendProgress(progress, StageCSPEC, "Creating Control Specification tables...", false)
	req4, resp4, err4 := p.runStage(ctx, StageCSPEC, tmplCSPEC, map[string]string{
		"DFDJSON": results.DFD,
	})
	results.StageResults = append(results.StageResults, StageResult{
		Stage: StageCSPEC, Input: req4, Output: resp4, Error: err4,
	})
	if err4 != nil {
		sendProgress(progress, StageCSPEC, "", true)
		return nil, fmt.Errorf("stage 4 failed: %w", err4)
	}
	results.CSPEC = extractJSON(resp4)
	sendProgress(progress, StageCSPEC, "CSPEC tables created", true)

	// Stage 5: SRS Formatting
	sendProgress(progress, StageSRS, "Generating IEEE 830 SRS document...", false)
	req5, resp5, err5 := p.runStage(ctx, StageSRS, tmplSRS, map[string]string{
		"ProjectName":      p.cfg.ProjectName,
		"Version":          p.cfg.Version,
		"Date":             time.Now().Format("January 2, 2006"),
		"RequirementsJSON": results.Requirements,
		"MoscowJSON":       results.MoSCoW,
		"DFDJSON":          results.DFD,
		"CSPECJSON":        results.CSPEC,
	})
	results.StageResults = append(results.StageResults, StageResult{
		Stage: StageSRS, Input: req5, Output: resp5, Error: err5,
	})
	if err5 != nil {
		sendProgress(progress, StageSRS, "", true)
		return nil, fmt.Errorf("stage 5 failed: %w", err5)
	}
	results.SRS = resp5
	sendProgress(progress, StageSRS, "SRS document generated", true)

	results.CompletedAt = time.Now()
	close(progress)
	return results, nil
}

// runStage renders a template, sends it to the LLM, and returns the response.
func (p *Pipeline) runStage(ctx context.Context, stage Stage, tmpl string, vars map[string]string) (string, string, error) {
	// Render template
	parsed, err := template.New(stage.String()).Parse(tmpl)
	if err != nil {
		return "", "", fmt.Errorf("template parse: %w", err)
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, vars); err != nil {
		return "", "", fmt.Errorf("template execute: %w", err)
	}

	prompt := buf.String()

	// Call LLM
	model := p.cfg.Model
	if model == "" {
		model = "qwen2.5-coder"
	}

	resp, err := p.client.Generate(ctx, model, prompt)
	if err != nil {
		return prompt, "", fmt.Errorf("llm call: %w", err)
	}

	return prompt, resp, nil
}

// extractJSON extracts a JSON block from LLM output (handles markdown fences).
func extractJSON(output string) string {
	output = strings.TrimSpace(output)

	// Try to extract content between ```json and ``` fences
	if idx := strings.Index(output, "```json"); idx >= 0 {
		start := idx + 7 // len("```json")
		end := strings.Index(output[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(output[start : start+end])
		}
	}

	// Try ``` without json
	if idx := strings.Index(output, "```"); idx >= 0 {
		start := idx + 3
		end := strings.Index(output[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(output[start : start+end])
		}
	}

	return output
}

// ValidateJSON checks if a string is valid JSON.
func ValidateJSON(s string) error {
	var js interface{}
	return json.Unmarshal([]byte(s), &js)
}

func sendProgress(progress chan<- ProgressUpdate, stage Stage, msg string, done bool) {
	if progress == nil {
		return
	}
	progress <- ProgressUpdate{Stage: stage, Message: msg, Done: done}
}
