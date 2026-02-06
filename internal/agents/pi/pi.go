package pi

import (
	_ "embed"
	"text/template"

	"github.com/sargehq/sarge/internal/project"
)

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

// Templates returns all compiled Pi templates.
func Templates() [7]*template.Template {
	return [7]*template.Template{
		template.Must(template.New("implement").Parse(taskText)),
		template.Must(template.New("estimate").Parse(estimateText)),
		template.Must(template.New("pr").Parse(prText)),
		template.Must(template.New("review").Parse(reviewText)),
		template.Must(template.New("update-pr-description").Parse(updatePRDescriptionText)),
		template.Must(template.New("plan").Parse(planText)),
		template.Must(template.New("log_analysis").Parse(logAnalysisText)),
	}
}

// Binary is the CLI binary name for the pi agent.
const Binary = "pi"

// BaseArgs returns pi-specific CLI arguments from project configuration.
func BaseArgs(cfg *project.Config) []string {
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

// TaskArgs returns additional CLI arguments for a specific task type.
func TaskArgs(taskType string, cfg *project.Config) []string {
	return nil
}
