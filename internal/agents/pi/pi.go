package pi

import (
	_ "embed"
	"text/template"

	"github.com/sargehq/sarge/internal/project"
)

// Binary is the CLI binary name for the pi agent.
const Binary = "pi"

// Embedded template text.
//
//go:embed templates/estimate.tmpl
var estimateText string

//go:embed templates/task.tmpl
var taskText string

//go:embed templates/pr.tmpl
var prText string

//go:embed templates/review.tmpl
var reviewText string

//go:embed templates/update-pr-description.tmpl
var updatePRDescriptionText string

//go:embed templates/plan.tmpl
var planText string

//go:embed templates/log_analysis.tmpl
var logAnalysisText string

// Compiled templates.
var (
	Estimate            = template.Must(template.New("estimate").Parse(estimateText))
	Task                = template.Must(template.New("task").Parse(taskText))
	PR                  = template.Must(template.New("pr").Parse(prText))
	Review              = template.Must(template.New("review").Parse(reviewText))
	UpdatePRDescription = template.Must(template.New("update-pr-description").Parse(updatePRDescriptionText))
	Plan                = template.Must(template.New("plan").Parse(planText))
	LogAnalysis         = template.Must(template.New("log_analysis").Parse(logAnalysisText))
)

// BuildArgs returns pi-specific CLI arguments from project configuration.
func BuildArgs(cfg *project.Config) []string {
	if cfg == nil {
		return nil
	}
	var args []string
	if cfg.Pi.Provider != "" {
		args = append(args, "--provider", cfg.Pi.Provider)
	}
	if cfg.Pi.Model != "" {
		args = append(args, "--model", cfg.Pi.Model)
	}
	if cfg.Pi.Thinking != "" {
		args = append(args, "--thinking", cfg.Pi.Thinking)
	}
	return args
}
